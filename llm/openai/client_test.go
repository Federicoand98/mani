package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Federicoand98/mani/core"
)

// sseServer replays the given SSE lines and records the request it received.
func sseServer(t *testing.T, lines []string, gotReq *map[string]any, gotAuth *string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if gotAuth != nil {
			*gotAuth = r.Header.Get("Authorization")
		}
		if gotReq != nil {
			_ = json.NewDecoder(r.Body).Decode(gotReq)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		for _, l := range lines {
			fmt.Fprintf(w, "%s\n\n", l)
			fl.Flush()
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newClient(url string) *Client {
	return New(Config{BaseURL: url, Model: "test-model", AuthFn: StaticKey("k-123")})
}

func userMsg(text string) []core.Message {
	return []core.Message{{Role: core.RoleUser, Content: []core.ContentBlock{core.TextBlock{Text: text}}}}
}

// Testo in streaming: i delta si concatenano in un solo TextBlock.
func TestSend_StreamsTextAndUsage(t *testing.T) {
	srv := sseServer(t, []string{
		`data: {"choices":[{"delta":{"content":"Ciao "}}]}`,
		`data: {"choices":[{"delta":{"content":"mondo"}}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":11,"completion_tokens":7}}`,
		`data: [DONE]`,
	}, nil, nil)

	var streamed strings.Builder
	resp, err := newClient(srv.URL).Send(context.Background(), userMsg("ciao"), nil,
		func(tok string, thinking bool) { streamed.WriteString(tok) })
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	if resp.StopReason != core.StopReasonEndTurn {
		t.Errorf("StopReason = %v, atteso end_turn", resp.StopReason)
	}
	if streamed.String() != "Ciao mondo" {
		t.Errorf("token streamati = %q", streamed.String())
	}
	if len(resp.Content) != 1 {
		t.Fatalf("attesi 1 blocco, ottenuti %d: %+v", len(resp.Content), resp.Content)
	}
	tb, ok := resp.Content[0].(core.TextBlock)
	if !ok || tb.Text != "Ciao mondo" {
		t.Errorf("TextBlock = %+v", resp.Content[0])
	}
	if resp.Usage.InputTokens != 11 || resp.Usage.OutputTokens != 7 {
		t.Errorf("usage = %+v, attesi 11/7", resp.Usage)
	}
}

// Tool call spezzata su più delta: gli argomenti vanno riassemblati e parsati.
func TestSend_AssemblesStreamedToolCall(t *testing.T) {
	srv := sseServer(t, []string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"read","arguments":"{\"path\":"}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"go.mod\"}"}}]}}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
	}, nil, nil)

	resp, err := newClient(srv.URL).Send(context.Background(), userMsg("leggi"), nil, func(string, bool) {})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	if resp.StopReason != core.StopReasonToolUse {
		t.Errorf("StopReason = %v, atteso tool_use", resp.StopReason)
	}
	var call core.ToolUseBlock
	for _, b := range resp.Content {
		if tu, ok := b.(core.ToolUseBlock); ok {
			call = tu
		}
	}
	if call.Name != "read" || call.ID != "call_1" {
		t.Fatalf("tool call = %+v", call)
	}
	if call.Input["path"] != "go.mod" {
		t.Errorf("argomenti non riassemblati: %+v", call.Input)
	}
}

// Le definizioni dei tool e il modello devono finire nella richiesta.
func TestSend_SendsToolsAndModel(t *testing.T) {
	var req map[string]any
	var auth string
	srv := sseServer(t, []string{`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`, `data: [DONE]`}, &req, &auth)

	tools := []core.ToolDefinition{{
		Name:        "read",
		Description: "read a file",
		InputSchema: core.ToolInputSchema{
			Type:       "object",
			Properties: map[string]core.ToolProperty{"path": {Type: "string"}},
			Required:   []string{"path"},
		},
	}}

	if _, err := newClient(srv.URL).Send(context.Background(), userMsg("x"), tools, func(string, bool) {}); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if auth != "Bearer k-123" {
		t.Errorf("Authorization = %q", auth)
	}
	if req["model"] != "test-model" {
		t.Errorf("model = %v", req["model"])
	}
	sent, ok := req["tools"].([]any)
	if !ok || len(sent) != 1 {
		t.Fatalf("tools non inviati: %v", req["tools"])
	}
}

// Un errore HTTP del provider deve risalire, non essere silenziato.
func TestSend_HTTPErrorPropagates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"invalid api key"}`, http.StatusUnauthorized)
	}))
	defer srv.Close()

	_, err := newClient(srv.URL).Send(context.Background(), userMsg("x"), nil, func(string, bool) {})
	if err == nil {
		t.Fatal("un 401 deve produrre un errore")
	}
}

// Un context già cancellato non deve nemmeno partire.
func TestSend_RespectsCancelledContext(t *testing.T) {
	srv := sseServer(t, []string{`data: [DONE]`}, nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := newClient(srv.URL).Send(ctx, userMsg("x"), nil, func(string, bool) {}); err == nil {
		t.Fatal("atteso errore con context cancellato")
	}
}
