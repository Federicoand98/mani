package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Federicoand98/mani/config"
)

func TestUnavailableClient_ReturnsOriginalError(t *testing.T) {
	want := errors.New("no creds")
	c := unavailableClient{err: want}
	_, err := c.Send(context.Background(), nil, nil, nil)
	if !errors.Is(err, want) {
		t.Fatalf("got %v, want %v", err, want)
	}
}

// A custom provider speaks the API its type names. An OpenAI-compatible one
// without a key sends no Authorization header (LM Studio, vLLM, llama.cpp).
func TestProviderModels_CustomEndpoint(t *testing.T) {
	var gotAuth atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth.Store(r.Header.Get("Authorization"))
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"local-7b"},{"id":"local-70b"}]}`))
	}))
	defer srv.Close()

	cfg := config.Config{Providers: map[string]config.ProviderConfig{
		"lmstudio": {BaseURL: srv.URL + "/v1", Type: "openai", Enabled: true},
	}}
	auth := config.Auth{Providers: map[string]config.Credential{}}

	models, err := ProviderModels(context.Background(), cfg, auth, "lmstudio")
	if err != nil || len(models) != 2 || !slices.Contains(models, "local-7b") {
		t.Fatalf("models = %v, %v", models, err)
	}
	if h, _ := gotAuth.Load().(string); h != "" {
		t.Errorf("Authorization sent without a key: %q", h)
	}

	auth.Set("lmstudio", config.Credential{Type: config.CredAPI, Key: "sk-local"})
	if _, err := ProviderModels(context.Background(), cfg, auth, "lmstudio"); err != nil {
		t.Fatal(err)
	}
	if h, _ := gotAuth.Load().(string); h != "Bearer sk-local" {
		t.Errorf("Authorization with a key = %q", h)
	}

	// Declared models win: the endpoint is not even asked.
	pc := cfg.Providers["lmstudio"]
	pc.Models = []string{"only-this"}
	pc.BaseURL = "http://127.0.0.1:1"
	cfg.Providers["lmstudio"] = pc
	if models, err := ProviderModels(context.Background(), cfg, auth, "lmstudio"); err != nil || len(models) != 1 || models[0] != "only-this" {
		t.Errorf("declared models = %v, %v", models, err)
	}

	cfg.Providers["weird"] = config.ProviderConfig{BaseURL: srv.URL, Type: "grpc"}
	if _, err := ProviderModels(context.Background(), cfg, auth, "weird"); err == nil || !strings.Contains(err.Error(), "unknown API type") {
		t.Errorf("unknown type: %v", err)
	}
	cfg.Providers["claude-proxy"] = config.ProviderConfig{BaseURL: srv.URL, Type: "anthropic"}
	if _, err := ProviderModels(context.Background(), cfg, auth, "claude-proxy"); err == nil || !strings.Contains(err.Error(), "needs an API key") {
		t.Errorf("anthropic without key: %v", err)
	}
}
