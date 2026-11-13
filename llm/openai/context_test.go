package openai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Federicoand98/mani/core"
)

func TestModelInfoMetadataAndUnsupported(t *testing.T) {
	for _, body := range []string{
		`{"data":[{"id":"test-model","max_model_len":131072,"max_output_tokens":11000}]}`,
		`{"data":[{"id":"test-model","context_length":200000,"top_provider":{"context_length":131072,"max_completion_tokens":11000}}]}`,
		`{"data":[{"id":"test-model","capabilities":{"limits":{"max_prompt_tokens":245000,"max_output_tokens":11000}}}]}`,
		`{"data":[{"id":"test-model"}]}`,
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) }))
		limits, err := newClient(srv.URL).ModelInfo(context.Background())
		srv.Close()
		if err != nil {
			t.Fatal(err)
		}
		if limits.ContextWindow != 0 && limits.ContextWindow != 131072 {
			t.Fatalf("window=%d", limits.ContextWindow)
		}
		if limits.MaxInputTokens != 0 && limits.MaxInputTokens != 245000 {
			t.Fatalf("input=%d", limits.MaxInputTokens)
		}
	}
	for _, code := range []int{404, 405, 401, 500} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(code); w.Write([]byte("failure")) }))
		_, err := newClient(srv.URL).ModelInfo(context.Background())
		srv.Close()
		if (err != nil) != (code != 404 && code != 405) {
			t.Fatalf("status=%d err=%v", code, err)
		}
	}
}

func TestSendOutputLimitAndOverflowClassification(t *testing.T) {
	var request map[string]any
	srv := sseServer(t, []string{`data: {"choices":[{"delta":{"content":"done"},"finish_reason":"stop"}]}`, `data: [DONE]`}, &request, nil)
	ctx := core.WithTokenLimits(context.Background(), core.TokenLimits{OutputTokens: 4096})
	if _, err := newClient(srv.URL).Send(ctx, userMsg("hi"), nil, nil); err != nil {
		t.Fatal(err)
	}
	if request["max_tokens"] != float64(4096) {
		t.Fatalf("request=%v", request)
	}
	for _, tt := range []struct {
		body     string
		overflow bool
	}{
		{`{"error":{"code":"context_length_exceeded"}}`, true},
		{`{"error":{"type":"ContextWindowExceededError"}}`, true},
		{`{"error":{"code":400,"type":"ContextWindowExceededError"}}`, true},
		{`{"error":{"message":"bad input mentions context_length_exceeded"}}`, false},
		{`not JSON`, false},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(400); w.Write([]byte(tt.body)) }))
		_, err := newClient(srv.URL).Send(ctx, userMsg("hi"), nil, nil)
		srv.Close()
		var pe *core.ProviderError
		if !errors.As(err, &pe) || pe.ContextOverflow != tt.overflow {
			t.Fatalf("body=%s err=%v", tt.body, err)
		}
	}
}

func TestModelInfoInvalidMetadata(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"id": "test-model", "max_model_len": -1}}})
	}))
	defer srv.Close()
	if _, err := newClient(srv.URL).ModelInfo(context.Background()); err == nil {
		t.Fatal("negative metadata accepted")
	}
}
