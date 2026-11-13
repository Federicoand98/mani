package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Federicoand98/mani/core"
)

func TestModelInfoAndConfiguredContext(t *testing.T) {
	var got *ollamaOptions
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/show" {
			w.Write([]byte(`{"model_info":{"example.context_length":131072},"parameters":"num_ctx 32768\n"}`))
			return
		}
		var req ollamaRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		got = req.Options
		w.Write([]byte(`{"message":{"role":"assistant","content":"done"},"done":true,"done_reason":"stop","prompt_eval_count":10,"eval_count":1}` + "\n"))
	}))
	defer srv.Close()
	c := NewOllamaClient(srv.URL, "example")
	info, err := c.ModelInfo(context.Background())
	if err != nil || info.ContextWindow != 32768 {
		t.Fatalf("info=%v err=%v", info, err)
	}
	ctx := core.WithTokenLimits(context.Background(), core.TokenLimits{ContextWindow: info.ContextWindow, OutputTokens: 4096})
	if _, err := c.Send(ctx, []core.Message{{Role: core.RoleUser, Content: []core.ContentBlock{core.TextBlock{Text: "hi"}}}}, nil, nil); err != nil {
		t.Fatal(err)
	}
	if got == nil || got.NumCtx != 32768 || got.NumPredict != 4096 {
		t.Fatalf("options=%v", got)
	}
}

func TestModelInfoUnknownDoesNotInventDefault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"model_info":{}}`)) }))
	defer srv.Close()
	info, err := NewOllamaClient(srv.URL, "example").ModelInfo(context.Background())
	if err != nil || info.ContextWindow != 0 {
		t.Fatalf("info=%v err=%v", info, err)
	}
}
