package fetch

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Federicoand98/mani/core"
	"github.com/Federicoand98/mani/tool"
)

const (
	maxBodyBytes   = 5 << 20
	maxResultChars = 40000
	requestTimeout = 30 * time.Second
	maxRedirects   = 5
)

type FetchTool struct {
	client      *http.Client
	hostAllowed func(host string) bool
	ipBlocked   func(net.IP) bool
}

type Option func(*FetchTool)

func WithHostAllowed(fn func(host string) bool) Option {
	return func(ft *FetchTool) { ft.hostAllowed = fn }
}

func WithIPGuard(blocked func(net.IP) bool) Option {
	return func(ft *FetchTool) { ft.ipBlocked = blocked }
}

func New(opts ...Option) *FetchTool {
	t := &FetchTool{ipBlocked: blockedIP}
	for _, o := range opts {
		o(t)
	}
	t.client = newSafeClient(t.ipBlocked, t.hostAllowed)
	return t
}

func (t *FetchTool) Name() string              { return "fetch" }
func (t *FetchTool) RiskLevel() core.RiskLevel { return core.RiskNetwork }
func (t *FetchTool) Description() string {
	return "Fetches a URL with an HTTP GET and returns its content as text. " +
		"HTML pages are stripped down to a readable text. Only http and https are supported."
}

func (t *FetchTool) Schema() tool.ToolSchema {
	return tool.ToolSchema{
		Name:        t.Name(),
		Description: t.Description(),
		InputSchema: tool.InputSchema{
			Type: "object",
			Properties: map[string]tool.PropertySchema{
				"url": {
					Type:        "string",
					Description: "The absolute http or https to fetch",
				},
			},
			Required: []string{"url"},
		},
	}
}

func (t *FetchTool) Execute(ctx context.Context, input map[string]any) (string, error) {
	raw, ok := input["url"].(string)
	if !ok || raw == "" {
		return "", fmt.Errorf("fetch: missing required input 'url'")
	}

	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("fetch: invalid url: %w", err)
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("fetch: unsupported scheme: %s", u.Scheme)
	}

	if t.hostAllowed != nil && !t.hostAllowed(u.Hostname()) {
		return "", fmt.Errorf("fetch: host not allowed: %s", u.Hostname())
	}

	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", fmt.Errorf("fetch: failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "mani (+https://github.com/Federicoand98/mani)")
	req.Header.Set("Accept", "text/html,text/plain,application/json;q=0.9,*/*;q=0.5")

	resp, err := t.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch: failed to fetch: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return "", fmt.Errorf("fetch: failed to read body: %w", err)
	}

	text := string(body)
	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/html") {
		text = htmlToText(text)
	}

	text = strings.TrimSpace(text)

	var out strings.Builder

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		fmt.Fprintf(&out, "HTTP %d %s\n\n", resp.StatusCode, http.StatusText(resp.StatusCode))
	}

	if len(text) > maxResultChars {
		out.WriteString(truncateUTF8(text, maxResultChars))
		fmt.Fprintf(&out, "\n\n... (truncated: the page is %d characters, showing the first %d)", len(text), maxResultChars)
	} else {
		out.WriteString(text)
	}

	if out.Len() == 0 {
		return "", fmt.Errorf("fetch: no content")
	}

	return out.String(), nil
}

func truncateUTF8(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}
