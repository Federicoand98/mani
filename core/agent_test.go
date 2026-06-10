//go:build phase2

package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// mockLLMClient restituisce risposte pre-configurate in sequenza.
type mockLLMClient struct {
	responses []LLMResponse
	err       error
	callIdx   int
}

func (m *mockLLMClient) Send(_ context.Context, _ []Message, _ []ToolDefinition) (LLMResponse, error) {
	if m.err != nil {
		return LLMResponse{}, m.err
	}
	if m.callIdx >= len(m.responses) {
		return LLMResponse{}, fmt.Errorf("mock: più chiamate del previsto (call %d)", m.callIdx)
	}
	resp := m.responses[m.callIdx]
	m.callIdx++
	return resp, nil
}

// mockCaptureLLMClient cattura gli argomenti passati a Send.
type mockCaptureLLMClient struct {
	onSend func(context.Context, []Message, []ToolDefinition) (LLMResponse, error)
}

func (m *mockCaptureLLMClient) Send(ctx context.Context, msgs []Message, tools []ToolDefinition) (LLMResponse, error) {
	return m.onSend(ctx, msgs, tools)
}

// mockToolExecutor implementa ToolExecutor con risultato pre-configurato.
type mockToolExecutor struct {
	name   string
	result string
	err    error
}

func (m *mockToolExecutor) Name() string { return m.name }
func (m *mockToolExecutor) Execute(_ context.Context, _ map[string]any) (string, error) {
	return m.result, m.err
}

// helper: risposta testuale end_turn
func textResp(text string) LLMResponse {
	return LLMResponse{
		Content:    []ContentBlock{TextBlock{Text: text}},
		StopReason: StopReasonEndTurn,
	}
}

// helper: risposta con tool_use
func toolUseResp(id, name string, input map[string]any) LLMResponse {
	return LLMResponse{
		Content:    []ContentBlock{ToolUseBlock{ID: id, Name: name, Input: input}},
		StopReason: StopReasonToolUse,
	}
}

// --- test successo ---

func TestAgent_Run_SimpleText_TwoMessages(t *testing.T) {
	client := &mockLLMClient{responses: []LLMResponse{textResp("risposta")}}
	agent := NewAgent(client)
	memory := NewInMemory()

	if err := agent.Run(context.Background(), memory, "ciao"); err != nil {
		t.Fatalf("errore inatteso: %v", err)
	}

	msgs := memory.Messages()
	if len(msgs) != 2 {
		t.Fatalf("attesi 2 messaggi (user + assistant), ottenuti %d", len(msgs))
	}
	if msgs[0].Role != RoleUser {
		t.Errorf("msgs[0]: role atteso 'user', ottenuto %q", msgs[0].Role)
	}
	if msgs[1].Role != RoleAssistant {
		t.Errorf("msgs[1]: role atteso 'assistant', ottenuto %q", msgs[1].Role)
	}
	if TextFrom(msgs[1].Content) != "risposta" {
		t.Errorf("testo risposta atteso 'risposta', ottenuto %q", TextFrom(msgs[1].Content))
	}
}

func TestAgent_Run_SingleToolLoop_FourMessages(t *testing.T) {
	// il loop deve produrre: user → assistant(tool_use) → tool(result) → assistant(text)
	client := &mockLLMClient{
		responses: []LLMResponse{
			toolUseResp("call_0", "read_file", map[string]any{"path": "main.go"}),
			textResp("il file contiene: package main"),
		},
	}
	agent := NewAgent(client)
	agent.AddTool(
		ToolDefinition{Name: "read_file"},
		&mockToolExecutor{name: "read_file", result: "package main"},
	)
	memory := NewInMemory()

	if err := agent.Run(context.Background(), memory, "leggi main.go"); err != nil {
		t.Fatalf("errore inatteso: %v", err)
	}

	msgs := memory.Messages()
	if len(msgs) != 4 {
		t.Fatalf("attesi 4 messaggi nel loop, ottenuti %d", len(msgs))
	}
	if msgs[0].Role != RoleUser {
		t.Errorf("msgs[0]: atteso user, ottenuto %q", msgs[0].Role)
	}
	if msgs[1].Role != RoleAssistant {
		t.Errorf("msgs[1]: atteso assistant, ottenuto %q", msgs[1].Role)
	}
	if msgs[2].Role != RoleTool {
		t.Errorf("msgs[2]: atteso tool, ottenuto %q", msgs[2].Role)
	}
	if msgs[3].Role != RoleAssistant {
		t.Errorf("msgs[3]: atteso assistant, ottenuto %q", msgs[3].Role)
	}
}

func TestAgent_Run_ToolResult_ContentCorrect(t *testing.T) {
	client := &mockLLMClient{
		responses: []LLMResponse{
			toolUseResp("call_0", "read_file", map[string]any{"path": "go.mod"}),
			textResp("ok"),
		},
	}
	agent := NewAgent(client)
	agent.AddTool(
		ToolDefinition{Name: "read_file"},
		&mockToolExecutor{name: "read_file", result: "module github.com/test"},
	)
	memory := NewInMemory()
	agent.Run(context.Background(), memory, "leggi go.mod")

	trb, ok := memory.Messages()[2].Content[0].(ToolResultBlock)
	if !ok {
		t.Fatalf("msgs[2].Content[0]: atteso ToolResultBlock, ottenuto %T", memory.Messages()[2].Content[0])
	}
	if trb.Content != "module github.com/test" {
		t.Errorf("tool result: atteso 'module github.com/test', ottenuto %q", trb.Content)
	}
	if trb.IsError {
		t.Error("IsError dovrebbe essere false per esecuzione riuscita")
	}
	if trb.ToolUseID != "call_0" {
		t.Errorf("ToolUseID atteso 'call_0', ottenuto %q", trb.ToolUseID)
	}
}

func TestAgent_Run_ToolDefinitions_SentToLLM(t *testing.T) {
	var capturedTools []ToolDefinition
	client := &mockCaptureLLMClient{
		onSend: func(_ context.Context, _ []Message, tools []ToolDefinition) (LLMResponse, error) {
			capturedTools = tools
			return textResp("ok"), nil
		},
	}
	agent := NewAgent(client)
	def := ToolDefinition{Name: "read_file", Description: "Legge un file"}
	agent.AddTool(def, &mockToolExecutor{name: "read_file"})
	agent.Run(context.Background(), NewInMemory(), "test")

	if len(capturedTools) != 1 {
		t.Fatalf("atteso 1 tool inviato all'LLM, ottenuti %d", len(capturedTools))
	}
	if capturedTools[0].Name != "read_file" {
		t.Errorf("tool name atteso 'read_file', ottenuto %q", capturedTools[0].Name)
	}
}

func TestAgent_Run_MultipleToolCalls_BothExecuted(t *testing.T) {
	client := &mockLLMClient{
		responses: []LLMResponse{
			{
				Content: []ContentBlock{
					ToolUseBlock{ID: "c1", Name: "tool_a", Input: map[string]any{}},
					ToolUseBlock{ID: "c2", Name: "tool_b", Input: map[string]any{}},
				},
				StopReason: StopReasonToolUse,
			},
			textResp("fatto"),
		},
	}
	executedA, executedB := false, false
	execA := &mockCapturingExecutor{name: "tool_a", onExecute: func() { executedA = true }}
	execB := &mockCapturingExecutor{name: "tool_b", onExecute: func() { executedB = true }}

	agent := NewAgent(client)
	agent.AddTool(ToolDefinition{Name: "tool_a"}, execA)
	agent.AddTool(ToolDefinition{Name: "tool_b"}, execB)

	if err := agent.Run(context.Background(), NewInMemory(), "usa entrambi"); err != nil {
		t.Fatalf("errore inatteso: %v", err)
	}
	if !executedA {
		t.Error("tool_a non è stato eseguito")
	}
	if !executedB {
		t.Error("tool_b non è stato eseguito")
	}
}

// --- test fallimento ---

func TestAgent_Run_ToolError_IsNotFatal(t *testing.T) {
	// l'errore nel tool viene passato al modello come IsError, non causa crash
	client := &mockLLMClient{
		responses: []LLMResponse{
			toolUseResp("call_0", "read_file", map[string]any{"path": "missing.go"}),
			textResp("il file non esiste"),
		},
	}
	agent := NewAgent(client)
	agent.AddTool(
		ToolDefinition{Name: "read_file"},
		&mockToolExecutor{name: "read_file", err: errors.New("file non trovato")},
	)
	memory := NewInMemory()

	err := agent.Run(context.Background(), memory, "leggi missing.go")
	if err != nil {
		t.Fatalf("errore inatteso: l'errore del tool non deve propagarsi, ottenuto: %v", err)
	}

	trb, ok := memory.Messages()[2].Content[0].(ToolResultBlock)
	if !ok {
		t.Fatalf("msgs[2]: atteso ToolResultBlock, ottenuto %T", memory.Messages()[2].Content[0])
	}
	if !trb.IsError {
		t.Error("IsError dovrebbe essere true per errore del tool")
	}
	if trb.Content != "file non trovato" {
		t.Errorf("errore atteso 'file non trovato', ottenuto %q", trb.Content)
	}
}

func TestAgent_Run_UnknownTool_ReturnsError(t *testing.T) {
	client := &mockLLMClient{
		responses: []LLMResponse{
			toolUseResp("call_0", "tool_sconosciuto", map[string]any{}),
		},
	}
	agent := NewAgent(client)
	// nessun tool registrato

	err := agent.Run(context.Background(), NewInMemory(), "usa tool sconosciuto")
	if err == nil {
		t.Fatal("atteso errore per tool non registrato, ottenuto nil")
	}
}

func TestAgent_Run_MaxIterations_ReturnsError(t *testing.T) {
	// l'LLM risponde sempre con tool_use → il loop deve terminare con errore
	responses := make([]LLMResponse, maxIterations+5)
	for i := range responses {
		responses[i] = toolUseResp("call_0", "read_file", map[string]any{"path": "f.go"})
	}

	client := &mockLLMClient{responses: responses}
	agent := NewAgent(client)
	agent.AddTool(
		ToolDefinition{Name: "read_file"},
		&mockToolExecutor{name: "read_file", result: "ok"},
	)

	err := agent.Run(context.Background(), NewInMemory(), "loop infinito")
	if err == nil {
		t.Fatal("atteso errore per limite iterazioni raggiunto, ottenuto nil")
	}
}

func TestAgent_Run_LLMError_Propagates(t *testing.T) {
	client := &mockLLMClient{err: errors.New("network error")}
	agent := NewAgent(client)

	err := agent.Run(context.Background(), NewInMemory(), "ciao")
	if err == nil {
		t.Fatal("atteso errore da LLM, ottenuto nil")
	}
	if !strings.Contains(err.Error(), "agent:") {
		t.Errorf("errore deve essere wrappato con 'agent:', ottenuto %q", err.Error())
	}
}

func TestAgent_Run_MaxTokens_ReturnsError(t *testing.T) {
	client := &mockLLMClient{
		responses: []LLMResponse{
			{
				Content:    []ContentBlock{TextBlock{Text: "troncato"}},
				StopReason: StopReasonMaxTokens,
			},
		},
	}
	agent := NewAgent(client)

	err := agent.Run(context.Background(), NewInMemory(), "input lungo")
	if err == nil {
		t.Fatal("atteso errore per max_tokens, ottenuto nil")
	}
}

func TestAgent_Run_ContextCancelled_ReturnsError(t *testing.T) {
	client := &mockLLMClient{err: context.Canceled}
	agent := NewAgent(client)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := agent.Run(ctx, NewInMemory(), "ciao")
	if err == nil {
		t.Fatal("atteso errore per context cancellato, ottenuto nil")
	}
}

// --- edge cases ---

func TestAgent_Run_EmptyInput_StillWorks(t *testing.T) {
	client := &mockLLMClient{responses: []LLMResponse{textResp("ok")}}
	agent := NewAgent(client)

	if err := agent.Run(context.Background(), NewInMemory(), ""); err != nil {
		t.Fatalf("errore inatteso con input vuoto: %v", err)
	}
}

func TestAgent_Run_NoTools_NilSentToLLM(t *testing.T) {
	// senza tool registrati, la slice passata all'LLM deve essere nil o vuota
	var capturedTools []ToolDefinition
	client := &mockCaptureLLMClient{
		onSend: func(_ context.Context, _ []Message, tools []ToolDefinition) (LLMResponse, error) {
			capturedTools = tools
			return textResp("ok"), nil
		},
	}
	agent := NewAgent(client)
	agent.Run(context.Background(), NewInMemory(), "senza tool")

	if len(capturedTools) != 0 {
		t.Errorf("attesi 0 tool senza AddTool, ottenuti %d", len(capturedTools))
	}
}

func TestAgent_Run_MemoryGrowsCorrectly(t *testing.T) {
	// due turni di tool_use: ogni turno aggiunge user+assistant+tool+assistant
	// ma il secondo turno non aggiunge "user" — l'input utente è solo nel primo Add
	client := &mockLLMClient{
		responses: []LLMResponse{
			toolUseResp("c0", "read_file", map[string]any{"path": "a.go"}),
			toolUseResp("c1", "read_file", map[string]any{"path": "b.go"}),
			textResp("fatto"),
		},
	}
	agent := NewAgent(client)
	agent.AddTool(
		ToolDefinition{Name: "read_file"},
		&mockToolExecutor{name: "read_file", result: "content"},
	)
	memory := NewInMemory()

	if err := agent.Run(context.Background(), memory, "leggi due file"); err != nil {
		t.Fatalf("errore inatteso: %v", err)
	}

	// user + assistant(c0) + tool(c0) + assistant(c1) + tool(c1) + assistant(text) = 6
	msgs := memory.Messages()
	if len(msgs) != 6 {
		t.Errorf("attesi 6 messaggi (2 loop + finale), ottenuti %d", len(msgs))
	}
}

// mockCapturingExecutor permette di sapere se un tool è stato eseguito.
type mockCapturingExecutor struct {
	name      string
	onExecute func()
}

func (m *mockCapturingExecutor) Name() string { return m.name }
func (m *mockCapturingExecutor) Execute(_ context.Context, _ map[string]any) (string, error) {
	m.onExecute()
	return "ok", nil
}
