package copilot

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/Federicoand98/mani/config"
	"github.com/Federicoand98/mani/llm/openai"
)

const (
	clientID      = "Iv1.b507a08c87ecfe98"
	tokenExchange = "https://api.github.com/copilot_internal/v2/token"
	integrationID = "vscode-chat"
	editorVersion = "mani/0.1"
)

func New(baseURL, model string, cred config.Credential) *openai.Client {
	tp := &tokenProvider{cred: cred}
	return openai.New(openai.Config{
		BaseURL: baseURL,
		Model:   model,
		AuthFn:  tp.token,
		Headers: map[string]string{
			"Editor-Version":         editorVersion,
			"Copilot-Integration-Id": integrationID,
		},
	})
}

type tokenProvider struct {
	mu   sync.Mutex
	cred config.Credential
	http *http.Client
}

type copilotTokenResp struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at"`
}

func (tp *tokenProvider) token(ctx context.Context) (string, error) {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	// token corto ancora valido (margine 60s)
	if tp.cred.Access != "" && time.Now().Add(60*time.Second).Before(tp.cred.ExpiresAt) {
		return tp.cred.Access, nil
	}

	if tp.cred.Refresh == "" {
		return "", fmt.Errorf("copilot: no GitHub token, do /login copilot before")
	}

	req, _ := http.NewRequestWithContext(ctx, "GET", tokenExchange, nil)
	req.Header.Set("Authorization", "token "+tp.cred.Refresh)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Editor-Version", editorVersion)

	resp, err := tp.client().Do(req)
	if err != nil {
		return "", fmt.Errorf("copilot: token exchange: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("copilot: token exchange HTTP %d", resp.StatusCode)
	}

	var tr copilotTokenResp
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return "", fmt.Errorf("copilot: decode token: %w", err)
	}

	tp.cred.Access = tr.Token
	tp.cred.ExpiresAt = time.Unix(tr.ExpiresAt, 0)

	// ripersisti in auth.json (best effort)
	if auth, err := config.LoadAuth(); err == nil {
		auth.Set("copilot", tp.cred)
		_ = config.SaveAuth(auth)
	}

	return tp.cred.Access, nil
}

func (tp *tokenProvider) client() *http.Client {
	if tp.http == nil {
		tp.http = &http.Client{Timeout: 30 * time.Second}
	}
	return tp.http
}
