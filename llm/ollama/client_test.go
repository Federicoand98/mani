package ollama

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

// --- mapMessagesToOllama ---

func TestMapMessagesToOllama_UserText(t *testing.T) {
	messages := []core.Message{
		{Role: core.RoleUser, Content: []core.ContentBlock{core.TextBlock{Text: "ciao"}}},
	}
	got := mapMessagesToOllama(messages)

	if len(got) != 1 {
		t.Fatalf("atteso 1 messaggio, ottenuto %d", len(got))
	}
	if got[0].Role != "user" {
		t.Errorf("role atteso 'user', ottenuto %q", got[0].Role)
	}
	if got[0].Content != "ciao" {
		t.Errorf("content atteso 'ciao', ottenuto %q", got[0].Content)
	}
	if len(got[0].ToolCalls) != 0 {
		t.Errorf("attesi 0 tool_calls, ottenuti %d", len(got[0].ToolCalls))
	}
}

func TestMapMessagesToOllama_SystemRole(t *testing.T) {
	messages := []core.Message{
		{Role: core.RoleSystem, Content: []core.ContentBlock{core.TextBlock{Text: "sei un assistente"}}},
	}
	got := mapMessagesToOllama(messages)

	if got[0].Role != "system" {
		t.Errorf("role atteso 'system', ottenuto %q", got[0].Role)
	}
}

func TestMapMessagesToOllama_AssistantWithToolUse(t *testing.T) {
	messages := []core.Message{
		{
			Role: core.RoleAssistant,
			Content: []core.ContentBlock{
				core.TextBlock{Text: "leggo il file"},
				core.ToolUseBlock{ID: "call_0", Name: "read_file", Input: map[string]any{"path": "main.go"}},
			},
		},
	}
	got := mapMessagesToOllama(messages)

	msg := got[0]
	if msg.Role != "assistant" {
		t.Errorf("role atteso 'assistant', ottenuto %q", msg.Role)
	}
	if msg.Content != "leggo il file" {
		t.Errorf("content atteso 'leggo il file', ottenuto %q", msg.Content)
	}
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("atteso 1 tool_call, ottenuti %d", len(msg.ToolCalls))
	}
	if msg.ToolCalls[0].Function.Name != "read_file" {
		t.Errorf("nome tool atteso 'read_file', ottenuto %q", msg.ToolCalls[0].Function.Name)
	}
	if msg.ToolCalls[0].Function.Arguments["path"] != "main.go" {
		t.Errorf("argomento path atteso 'main.go', ottenuto %v", msg.ToolCalls[0].Function.Arguments["path"])
	}
}

func TestMapMessagesToOllama_ToolResult(t *testing.T) {
	messages := []core.Message{
		{
			Role: core.RoleTool,
			Content: []core.ContentBlock{
				core.ToolResultBlock{ToolUseID: "call_0", Content: "package main", IsError: false},
			},
		},
	}
	got := mapMessagesToOllama(messages)

	if got[0].Role != "tool" {
		t.Errorf("role atteso 'tool', ottenuto %q", got[0].Role)
	}
	if got[0].Content != "package main" {
		t.Errorf("content atteso 'package main', ottenuto %q", got[0].Content)
	}
}

func TestMapMessagesToOllama_ToolResultError(t *testing.T) {
	messages := []core.Message{
		{
			Role: core.RoleTool,
			Content: []core.ContentBlock{
				core.ToolResultBlock{ToolUseID: "call_0", Content: "file not found", IsError: true},
			},
		},
	}
	got := mapMessagesToOllama(messages)

	expected := "[ERROR] file not found"
	if got[0].Content != expected {
		t.Errorf("content atteso %q, ottenuto %q", expected, got[0].Content)
	}
}

func TestMapMessagesToOllama_MultipleTextBlocksConcatenated(t *testing.T) {
	messages := []core.Message{
		{
			Role: core.RoleUser,
			Content: []core.ContentBlock{
				core.TextBlock{Text: "prima "},
				core.TextBlock{Text: "seconda"},
			},
		},
	}
	got := mapMessagesToOllama(messages)

	if got[0].Content != "prima seconda" {
		t.Errorf("content atteso 'prima seconda', ottenuto %q", got[0].Content)
	}
}

// --- mapToolsToOllama ---

func TestMapToolsToOllama_SingleTool(t *testing.T) {
	tools := []core.ToolDefinition{
		{
			Name:        "read_file",
			Description: "Legge un file",
			InputSchema: core.ToolInputSchema{
				Type: "object",
				Properties: map[string]core.ToolProperty{
					"path": {Type: "string", Description: "Percorso del file"},
				},
				Required: []string{"path"},
			},
		},
	}
	got := mapToolsToOllama(tools)

	if len(got) != 1 {
		t.Fatalf("atteso 1 tool, ottenuto %d", len(got))
	}
	ot := got[0]
	if ot.Type != "function" {
		t.Errorf("type atteso 'function', ottenuto %q", ot.Type)
	}
	if ot.Function.Name != "read_file" {
		t.Errorf("name atteso 'read_file', ottenuto %q", ot.Function.Name)
	}
	if ot.Function.Parameters.Type != "object" {
		t.Errorf("parameters.type atteso 'object', ottenuto %q", ot.Function.Parameters.Type)
	}
	prop, ok := ot.Function.Parameters.Properties["path"]
	if !ok {
		t.Fatal("proprietà 'path' non trovata")
	}
	if prop.Type != "string" {
		t.Errorf("tipo proprietà 'path' atteso 'string', ottenuto %q", prop.Type)
	}
	if len(ot.Function.Parameters.Required) != 1 || ot.Function.Parameters.Required[0] != "path" {
		t.Errorf("required atteso [path], ottenuto %v", ot.Function.Parameters.Required)
	}
}

func TestMapToolsToOllama_Empty(t *testing.T) {
	got := mapToolsToOllama(nil)
	if len(got) != 0 {
		t.Errorf("atteso 0 tools, ottenuto %d", len(got))
	}
}

// --- mapOllamaResponseToLLM ---

func TestMapOllamaResponseToLLM_TextOnly(t *testing.T) {
	resp := ollamaResponse{
		Message:         ollamaMessage{Role: "assistant", Content: "risposta"},
		DoneReason:      "stop",
		PromptEvalCount: 10,
		EvalCount:       20,
	}
	got := mapOllamaResponseToLLM(resp)

	if len(got.Content) != 1 {
		t.Fatalf("atteso 1 block, ottenuto %d", len(got.Content))
	}
	tb, ok := got.Content[0].(core.TextBlock)
	if !ok {
		t.Fatalf("atteso TextBlock, ottenuto %T", got.Content[0])
	}
	if tb.Text != "risposta" {
		t.Errorf("text atteso 'risposta', ottenuto %q", tb.Text)
	}
	if got.StopReason != core.StopReasonEndTurn {
		t.Errorf("stop reason attesa %q, ottenuta %q", core.StopReasonEndTurn, got.StopReason)
	}
	if got.Usage.InputTokens != 10 || got.Usage.OutputTokens != 20 {
		t.Errorf("usage atteso {10,20}, ottenuto {%d,%d}", got.Usage.InputTokens, got.Usage.OutputTokens)
	}
}

func TestMapOllamaResponseToLLM_ToolUse(t *testing.T) {
	resp := ollamaResponse{
		Message: ollamaMessage{
			Role: "assistant",
			ToolCalls: []ollamaToolCall{
				{Function: ollamaToolCallFunction{Name: "read_file", Arguments: map[string]any{"path": "main.go"}}},
			},
		},
		DoneReason: "stop",
	}
	got := mapOllamaResponseToLLM(resp)

	if len(got.Content) != 1 {
		t.Fatalf("atteso 1 block, ottenuto %d", len(got.Content))
	}
	tub, ok := got.Content[0].(core.ToolUseBlock)
	if !ok {
		t.Fatalf("atteso ToolUseBlock, ottenuto %T", got.Content[0])
	}
	if tub.Name != "read_file" {
		t.Errorf("nome atteso 'read_file', ottenuto %q", tub.Name)
	}
	if tub.ID != "call_0" {
		t.Errorf("ID atteso 'call_0', ottenuto %q", tub.ID)
	}
	if tub.Input["path"] != "main.go" {
		t.Errorf("input path atteso 'main.go', ottenuto %v", tub.Input["path"])
	}
	if got.StopReason != core.StopReasonToolUse {
		t.Errorf("stop reason attesa %q, ottenuta %q", core.StopReasonToolUse, got.StopReason)
	}
}

func TestMapOllamaResponseToLLM_Mixed(t *testing.T) {
	resp := ollamaResponse{
		Message: ollamaMessage{
			Role:    "assistant",
			Content: "leggo il file",
			ToolCalls: []ollamaToolCall{
				{Function: ollamaToolCallFunction{Name: "read_file", Arguments: map[string]any{"path": "main.go"}}},
			},
		},
		DoneReason: "stop",
	}
	got := mapOllamaResponseToLLM(resp)

	if len(got.Content) != 2 {
		t.Fatalf("attesi 2 blocks, ottenuti %d", len(got.Content))
	}
	if _, ok := got.Content[0].(core.TextBlock); !ok {
		t.Errorf("primo block atteso TextBlock, ottenuto %T", got.Content[0])
	}
	if _, ok := got.Content[1].(core.ToolUseBlock); !ok {
		t.Errorf("secondo block atteso ToolUseBlock, ottenuto %T", got.Content[1])
	}
	if got.StopReason != core.StopReasonToolUse {
		t.Errorf("stop reason attesa %q con tool calls, ottenuta %q", core.StopReasonToolUse, got.StopReason)
	}
}

func TestMapOllamaResponseToLLM_EmptyContent(t *testing.T) {
	resp := ollamaResponse{
		Message:    ollamaMessage{Role: "assistant", Content: ""},
		DoneReason: "stop",
	}
	got := mapOllamaResponseToLLM(resp)

	if len(got.Content) != 0 {
		t.Errorf("attesi 0 blocks per content vuoto, ottenuti %d", len(got.Content))
	}
}

// --- Send (integration con httptest) ---

func TestSend_TextResponse(t *testing.T) {
	serverResp := ollamaResponse{
		Message:         ollamaMessage{Role: "assistant", Content: "ciao!"},
		DoneReason:      "stop",
		PromptEvalCount: 5,
		EvalCount:       3,
	}
	body, _ := json.Marshal(serverResp)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("metodo atteso POST, ottenuto %s", r.Method)
		}
		if r.URL.Path != "/api/chat" {
			t.Errorf("path atteso /api/chat, ottenuto %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer server.Close()

	client := NewOllamaClient(server.URL, "test-model")
	messages := []core.Message{
		{Role: core.RoleUser, Content: []core.ContentBlock{core.TextBlock{Text: "ciao"}}},
	}

	got, err := client.Send(context.Background(), messages, nil, nil)
	if err != nil {
		t.Fatalf("errore inatteso: %v", err)
	}
	if len(got.Content) != 1 {
		t.Fatalf("atteso 1 block, ottenuto %d", len(got.Content))
	}
	tb, ok := got.Content[0].(core.TextBlock)
	if !ok {
		t.Fatalf("atteso TextBlock, ottenuto %T", got.Content[0])
	}
	if tb.Text != "ciao!" {
		t.Errorf("text atteso 'ciao!', ottenuto %q", tb.Text)
	}
}

func TestSend_VerificaRequestBody(t *testing.T) {
	var receivedReq ollamaRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedReq)
		w.Write([]byte(`{"message":{"role":"assistant","content":"ok"},"done_reason":"stop"}`))
	}))
	defer server.Close()

	client := NewOllamaClient(server.URL, "qwen2.5-coder")
	client.Send(context.Background(), nil, nil, nil)

	if receivedReq.Model != "qwen2.5-coder" {
		t.Errorf("model atteso 'qwen2.5-coder', ottenuto %q", receivedReq.Model)
	}
	if receivedReq.Stream != false {
		t.Error("stream dovrebbe essere false")
	}
}

func TestSend_HTTP500(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewOllamaClient(server.URL, "test-model")
	_, err := client.Send(context.Background(), nil, nil, nil)
	if err == nil {
		t.Fatal("atteso errore per HTTP 500, ottenuto nil")
	}
}

func TestSend_ContextCancelled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// non risponde mai — il context si cancella prima
	}))
	defer server.Close()

	client := NewOllamaClient(server.URL, "test-model")
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancella subito

	_, err := client.Send(ctx, nil, nil, nil)
	if err == nil {
		t.Fatal("atteso errore per context cancellato, ottenuto nil")
	}
}

// --- Send streaming ---

// helper: costruisce un server httptest che risponde con NDJSON.
func ndjsonServer(t *testing.T, chunks []string) *httptest.Server {
	t.Helper()
	body := strings.Join(chunks, "\n")
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		fmt.Fprint(w, body)
	}))
}

func TestSend_Streaming_SetsStreamTrue(t *testing.T) {
	var receivedReq ollamaRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedReq)
		fmt.Fprint(w, `{"message":{"role":"assistant","content":""},"done":true,"done_reason":"stop"}`)
	}))
	defer server.Close()

	client := NewOllamaClient(server.URL, "test-model")
	client.Send(context.Background(), nil, nil, func(string, bool) {})

	if !receivedReq.Stream {
		t.Error("stream dovrebbe essere true quando tokenHandler != nil")
	}
}

func TestSend_Streaming_AccumulatesTokens(t *testing.T) {
	chunks := []string{
		`{"message":{"role":"assistant","content":"ciao"},"done":false}`,
		`{"message":{"role":"assistant","content":" mondo"},"done":false}`,
		`{"message":{"role":"assistant","content":""},"done":true,"done_reason":"stop","prompt_eval_count":3,"eval_count":2}`,
	}
	server := ndjsonServer(t, chunks)
	defer server.Close()

	client := NewOllamaClient(server.URL, "test-model")

	var received []string
	handler := func(token string, isThinking bool) {
		received = append(received, token)
	}

	got, err := client.Send(context.Background(), nil, nil, handler)
	if err != nil {
		t.Fatalf("errore inatteso: %v", err)
	}

	// token ricevuti nell'ordine corretto
	if len(received) != 2 {
		t.Fatalf("attesi 2 token, ottenuti %d: %v", len(received), received)
	}
	if received[0] != "ciao" || received[1] != " mondo" {
		t.Errorf("token attesi ['ciao', ' mondo'], ottenuti %v", received)
	}

	// contenuto accumulato correttamente nella risposta
	tb, ok := got.Content[0].(core.TextBlock)
	if !ok {
		t.Fatalf("atteso TextBlock, ottenuto %T", got.Content[0])
	}
	if tb.Text != "ciao mondo" {
		t.Errorf("contenuto accumulato atteso 'ciao mondo', ottenuto %q", tb.Text)
	}
}

func TestSend_Streaming_HandlerCalledInOrder(t *testing.T) {
	words := []string{"uno", " due", " tre", " quattro"}
	var chunks []string
	for _, w := range words {
		chunks = append(chunks, fmt.Sprintf(`{"message":{"role":"assistant","content":%q},"done":false}`, w))
	}
	chunks = append(chunks, `{"message":{"role":"assistant","content":""},"done":true,"done_reason":"stop"}`)

	server := ndjsonServer(t, chunks)
	defer server.Close()

	client := NewOllamaClient(server.URL, "test-model")

	var received []string
	client.Send(context.Background(), nil, nil, func(token string, _ bool) {
		received = append(received, token)
	})

	for i, w := range words {
		if i >= len(received) || received[i] != w {
			t.Errorf("token[%d]: atteso %q, ottenuto %q", i, w, received[i])
		}
	}
}

func TestSend_Streaming_ThinkingTokens_IsThinkingTrue(t *testing.T) {
	chunks := []string{
		`{"message":{"role":"assistant","content":"","thinking":"sto ragionando"},"done":false}`,
		`{"message":{"role":"assistant","content":"risposta"},"done":false}`,
		`{"message":{"role":"assistant","content":""},"done":true,"done_reason":"stop"}`,
	}
	server := ndjsonServer(t, chunks)
	defer server.Close()

	client := NewOllamaClient(server.URL, "test-model")

	type call struct {
		token      string
		isThinking bool
	}
	var calls []call
	client.Send(context.Background(), nil, nil, func(token string, isThinking bool) {
		calls = append(calls, call{token, isThinking})
	})

	if len(calls) != 2 {
		t.Fatalf("attese 2 chiamate al handler, ottenute %d", len(calls))
	}
	if calls[0].token != "sto ragionando" || !calls[0].isThinking {
		t.Errorf("prima call: atteso thinking='sto ragionando' isThinking=true, ottenuto %+v", calls[0])
	}
	if calls[1].token != "risposta" || calls[1].isThinking {
		t.Errorf("seconda call: atteso token='risposta' isThinking=false, ottenuto %+v", calls[1])
	}
}

func TestSend_Streaming_EmptyChunks_HandlerNotCalled(t *testing.T) {
	// chunk con content="" e thinking="" non devono chiamare il handler
	chunks := []string{
		`{"message":{"role":"assistant","content":""},"done":false}`,
		`{"message":{"role":"assistant","content":"testo"},"done":false}`,
		`{"message":{"role":"assistant","content":""},"done":true,"done_reason":"stop"}`,
	}
	server := ndjsonServer(t, chunks)
	defer server.Close()

	client := NewOllamaClient(server.URL, "test-model")

	callCount := 0
	client.Send(context.Background(), nil, nil, func(string, bool) { callCount++ })

	if callCount != 1 {
		t.Errorf("attesa 1 chiamata (solo per 'testo'), ottenute %d", callCount)
	}
}

func TestSend_Streaming_UsageFromLastChunk(t *testing.T) {
	chunks := []string{
		`{"message":{"role":"assistant","content":"ok"},"done":false}`,
		`{"message":{"role":"assistant","content":""},"done":true,"done_reason":"stop","prompt_eval_count":42,"eval_count":7}`,
	}
	server := ndjsonServer(t, chunks)
	defer server.Close()

	client := NewOllamaClient(server.URL, "test-model")
	got, err := client.Send(context.Background(), nil, nil, func(string, bool) {})
	if err != nil {
		t.Fatalf("errore inatteso: %v", err)
	}

	if got.Usage.InputTokens != 42 {
		t.Errorf("InputTokens atteso 42, ottenuto %d", got.Usage.InputTokens)
	}
	if got.Usage.OutputTokens != 7 {
		t.Errorf("OutputTokens atteso 7, ottenuto %d", got.Usage.OutputTokens)
	}
}

func TestSend_Streaming_StopReason_EndTurn(t *testing.T) {
	chunks := []string{
		`{"message":{"role":"assistant","content":"ok"},"done":false}`,
		`{"message":{"role":"assistant","content":""},"done":true,"done_reason":"stop"}`,
	}
	server := ndjsonServer(t, chunks)
	defer server.Close()

	client := NewOllamaClient(server.URL, "test-model")
	got, err := client.Send(context.Background(), nil, nil, func(string, bool) {})
	if err != nil {
		t.Fatalf("errore inatteso: %v", err)
	}
	if got.StopReason != core.StopReasonEndTurn {
		t.Errorf("StopReason atteso end_turn, ottenuto %q", got.StopReason)
	}
}

func TestSend_Streaming_ToolCall_StopReasonToolUse(t *testing.T) {
	chunks := []string{
		`{"message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"read_file","arguments":{"path":"main.go"}}}]},"done":false}`,
		`{"message":{"role":"assistant","content":""},"done":true,"done_reason":"stop"}`,
	}
	server := ndjsonServer(t, chunks)
	defer server.Close()

	client := NewOllamaClient(server.URL, "test-model")
	got, err := client.Send(context.Background(), nil, nil, func(string, bool) {})
	if err != nil {
		t.Fatalf("errore inatteso: %v", err)
	}

	if got.StopReason != core.StopReasonToolUse {
		t.Errorf("StopReason atteso tool_use, ottenuto %q", got.StopReason)
	}

	// cerca il ToolUseBlock nella risposta
	var toolBlock *core.ToolUseBlock
	for _, b := range got.Content {
		if tub, ok := b.(core.ToolUseBlock); ok {
			toolBlock = &tub
			break
		}
	}
	if toolBlock == nil {
		t.Fatal("ToolUseBlock non trovato nella risposta")
	}
	if toolBlock.Name != "read_file" {
		t.Errorf("tool name atteso 'read_file', ottenuto %q", toolBlock.Name)
	}
	if toolBlock.Input["path"] != "main.go" {
		t.Errorf("tool input path atteso 'main.go', ottenuto %v", toolBlock.Input["path"])
	}
}

func TestSend_Streaming_HTTP500_ReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewOllamaClient(server.URL, "test-model")
	_, err := client.Send(context.Background(), nil, nil, func(string, bool) {})
	if err == nil {
		t.Fatal("atteso errore per HTTP 500 con streaming, ottenuto nil")
	}
}
