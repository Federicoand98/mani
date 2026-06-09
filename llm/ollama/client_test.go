package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Federicoand98/mani/core"
	"github.com/Federicoand98/mani/llm"
	"github.com/Federicoand98/mani/tool"
)

// mockTool implementa tool.Tool per i test — nessun comportamento reale.
type mockTool struct {
	name   string
	schema tool.ToolSchema
}

func (m mockTool) Name() string                                                { return m.name }
func (m mockTool) Description() string                                         { return "mock" }
func (m mockTool) Schema() tool.ToolSchema                                     { return m.schema }
func (m mockTool) Execute(_ context.Context, _ map[string]interface{}) (string, error) {
	return "", nil
}

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
			Role: core.RoleUser,
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
			Role: core.RoleUser,
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
	tools := []tool.Tool{
		mockTool{
			name: "read_file",
			schema: tool.ToolSchema{
				Name:        "read_file",
				Description: "Legge un file",
				InputSchema: tool.InputSchema{
					Type: "object",
					Properties: map[string]tool.PropertySchema{
						"path": {Type: "string", Description: "Percorso del file"},
					},
					Required: []string{"path"},
				},
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
	if got.StopReason != llm.StopReasonEndTurn {
		t.Errorf("stop reason attesa %q, ottenuta %q", llm.StopReasonEndTurn, got.StopReason)
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
	if got.StopReason != llm.StopReasonToolUse {
		t.Errorf("stop reason attesa %q, ottenuta %q", llm.StopReasonToolUse, got.StopReason)
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
	if got.StopReason != llm.StopReasonToolUse {
		t.Errorf("stop reason attesa %q con tool calls, ottenuta %q", llm.StopReasonToolUse, got.StopReason)
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

	got, err := client.Send(context.Background(), messages, nil)
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
	client.Send(context.Background(), nil, nil)

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
	_, err := client.Send(context.Background(), nil, nil)
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

	_, err := client.Send(ctx, nil, nil)
	if err == nil {
		t.Fatal("atteso errore per context cancellato, ottenuto nil")
	}
}
