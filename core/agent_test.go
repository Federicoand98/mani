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

func (m *mockLLMClient) Send(_ context.Context, _ []Message, _ []ToolDefinition, _ TokenHandler) (LLMResponse, error) {
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
	onSend func(context.Context, []Message, []ToolDefinition, TokenHandler) (LLMResponse, error)
}

func (m *mockCaptureLLMClient) Send(ctx context.Context, msgs []Message, tools []ToolDefinition, h TokenHandler) (LLMResponse, error) {
	return m.onSend(ctx, msgs, tools, h)
}

// mockToolExecutor implementa ToolExecutor con risultato pre-configurato.
type mockToolExecutor struct {
	name   string
	result string
	err    error
	calls  int
}

func (m *mockToolExecutor) Name() string { return m.name }
func (m *mockToolExecutor) Execute(_ context.Context, _ map[string]any) (string, error) {
	m.calls++
	return m.result, m.err
}

// captureEmitter cattura gli eventi emessi dall'Agent (sostituisce il vecchio ToolEventHandler).
type captureEmitter struct {
	toolCalls   int
	toolResults int
	lastName    string
	lastResult  string
	lastIsError bool
}

func (c *captureEmitter) Token(string)    {}
func (c *captureEmitter) Thinking(string) {}
func (c *captureEmitter) ToolCall(name string, _ map[string]any) {
	c.toolCalls++
	c.lastName = name
}

func (c *captureEmitter) ToolResult(name, result string, isError bool) {
	c.toolResults++
	c.lastName = name
	c.lastResult = result
	c.lastIsError = isError
}
func (c *captureEmitter) Usage(int, int) {}

func textResp(text string) LLMResponse {
	return LLMResponse{
		Content:    []ContentBlock{TextBlock{Text: text}},
		StopReason: StopReasonEndTurn,
	}
}

func toolUseResp(id, name string, input map[string]any) LLMResponse {
	return LLMResponse{
		Content:    []ContentBlock{ToolUseBlock{ID: id, Name: name, Input: input}},
		StopReason: StopReasonToolUse,
	}
}

// --- successo ---

func TestAgent_Run_SimpleText_TwoMessages(t *testing.T) {
	client := &mockLLMClient{responses: []LLMResponse{textResp("risposta")}}
	agent := NewAgent(client)
	memory := NewInMemory()

	if err := agent.Run(context.Background(), memory, "ciao"); err != nil {
		t.Fatalf("errore inatteso: %v", err)
	}

	msgs := memory.Messages()
	if len(msgs) != 2 {
		t.Fatalf("attesi 2 messaggi, ottenuti %d", len(msgs))
	}
	if msgs[0].Role != RoleUser || msgs[1].Role != RoleAssistant {
		t.Errorf("ruoli attesi user/assistant, ottenuti %q/%q", msgs[0].Role, msgs[1].Role)
	}
}

func TestAgent_Run_SingleToolLoop_FourMessages(t *testing.T) {
	client := &mockLLMClient{
		responses: []LLMResponse{
			toolUseResp("call_0", "read_file", map[string]any{"path": "main.go"}),
			textResp("ok"),
		},
	}
	agent := NewAgent(client)
	agent.AddTool(ToolDefinition{Name: "read_file"}, &mockToolExecutor{result: "package main"})

	memory := NewInMemory()
	if err := agent.Run(context.Background(), memory, "leggi"); err != nil {
		t.Fatalf("errore inatteso: %v", err)
	}

	msgs := memory.Messages()
	if len(msgs) != 4 {
		t.Fatalf("attesi 4 messaggi, ottenuti %d", len(msgs))
	}
	if msgs[2].Role != RoleTool {
		t.Errorf("msgs[2]: atteso tool, ottenuto %q", msgs[2].Role)
	}
}

func TestAgent_Run_ToolResultBlock_Populated(t *testing.T) {
	client := &mockLLMClient{
		responses: []LLMResponse{
			toolUseResp("call_0", "read_file", map[string]any{}),
			textResp("ok"),
		},
	}
	agent := NewAgent(client)
	agent.AddTool(ToolDefinition{Name: "read_file"}, &mockToolExecutor{result: "module x"})

	memory := NewInMemory()
	agent.Run(context.Background(), memory, "test")

	trb, ok := memory.Messages()[2].Content[0].(ToolResultBlock)
	if !ok {
		t.Fatalf("atteso ToolResultBlock, ottenuto %T", memory.Messages()[2].Content[0])
	}
	if trb.Content != "module x" {
		t.Errorf("content atteso 'module x', ottenuto %q", trb.Content)
	}
	if trb.IsError {
		t.Error("IsError dovrebbe essere false")
	}
	if trb.ToolUseID != "call_0" {
		t.Errorf("ToolUseID atteso 'call_0', ottenuto %q", trb.ToolUseID)
	}
}

func TestAgent_Run_ToolDefinitions_SentToLLM(t *testing.T) {
	var captured []ToolDefinition
	client := &mockCaptureLLMClient{
		onSend: func(_ context.Context, _ []Message, tools []ToolDefinition, _ TokenHandler) (LLMResponse, error) {
			captured = tools
			return textResp("ok"), nil
		},
	}
	agent := NewAgent(client)
	agent.AddTool(ToolDefinition{Name: "read_file"}, &mockToolExecutor{})
	agent.Run(context.Background(), NewInMemory(), "test")

	if len(captured) != 1 || captured[0].Name != "read_file" {
		t.Errorf("tool 'read_file' non passato all'LLM: %+v", captured)
	}
}

func TestAgent_Run_MultipleToolCalls_BothExecuted(t *testing.T) {
	client := &mockLLMClient{
		responses: []LLMResponse{
			{
				Content: []ContentBlock{
					ToolUseBlock{ID: "c1", Name: "tool_a"},
					ToolUseBlock{ID: "c2", Name: "tool_b"},
				},
				StopReason: StopReasonToolUse,
			},
			textResp("ok"),
		},
	}
	execA := &mockToolExecutor{result: "a"}
	execB := &mockToolExecutor{result: "b"}

	agent := NewAgent(client)
	agent.AddTool(ToolDefinition{Name: "tool_a"}, execA)
	agent.AddTool(ToolDefinition{Name: "tool_b"}, execB)

	agent.Run(context.Background(), NewInMemory(), "usa entrambi")
	if execA.calls != 1 || execB.calls != 1 {
		t.Errorf("entrambi i tool devono essere chiamati: a=%d, b=%d", execA.calls, execB.calls)
	}
}

// --- fallimenti ---

func TestAgent_Run_ToolError_NotFatal(t *testing.T) {
	client := &mockLLMClient{
		responses: []LLMResponse{
			toolUseResp("c0", "read_file", nil),
			textResp("ok"),
		},
	}
	agent := NewAgent(client)
	agent.AddTool(ToolDefinition{Name: "read_file"}, &mockToolExecutor{err: errors.New("nope")})

	memory := NewInMemory()
	if err := agent.Run(context.Background(), memory, "leggi"); err != nil {
		t.Fatalf("errore tool non deve propagarsi: %v", err)
	}

	trb := memory.Messages()[2].Content[0].(ToolResultBlock)
	if !trb.IsError || trb.Content != "nope" {
		t.Errorf("IsError/Content non corrispondono: %+v", trb)
	}
}

func TestAgent_Run_UnknownTool_ReturnsError(t *testing.T) {
	client := &mockLLMClient{
		responses: []LLMResponse{toolUseResp("c0", "sconosciuto", nil)},
	}
	agent := NewAgent(client)
	if err := agent.Run(context.Background(), NewInMemory(), "x"); err == nil {
		t.Fatal("atteso errore per tool sconosciuto")
	}
}

func TestAgent_Run_MaxIterations_ReturnsError(t *testing.T) {
	responses := make([]LLMResponse, defaultMaxIterations+5)
	for i := range responses {
		responses[i] = toolUseResp("c0", "t", nil)
	}
	client := &mockLLMClient{responses: responses}
	agent := NewAgent(client)
	agent.AddTool(ToolDefinition{Name: "t"}, &mockToolExecutor{result: "ok"})

	// dopo maxIterations senza completare, l'agent ritorna errore (non più nil)
	if err := agent.Run(context.Background(), NewInMemory(), "loop"); err == nil {
		t.Error("atteso errore al raggiungimento del limite iterazioni")
	}
}

func TestAgent_Run_LLMError_Wrapped(t *testing.T) {
	client := &mockLLMClient{err: errors.New("network")}
	agent := NewAgent(client)
	err := agent.Run(context.Background(), NewInMemory(), "x")
	if err == nil || !strings.Contains(err.Error(), "agent:") {
		t.Errorf("errore deve essere wrappato con 'agent:', ottenuto %v", err)
	}
}

func TestAgent_Run_MaxTokens_ReturnsError(t *testing.T) {
	client := &mockLLMClient{
		responses: []LLMResponse{
			{Content: []ContentBlock{TextBlock{Text: "..."}}, StopReason: StopReasonMaxTokens},
		},
	}
	agent := NewAgent(client)
	if err := agent.Run(context.Background(), NewInMemory(), "x"); err == nil {
		t.Fatal("atteso errore per max_tokens")
	}
}

func TestAgent_Run_ContextCancelled_ReturnsError(t *testing.T) {
	client := &mockLLMClient{err: context.Canceled}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	agent := NewAgent(client)
	if err := agent.Run(ctx, NewInMemory(), "x"); err == nil {
		t.Fatal("atteso errore per context cancellato")
	}
}

// --- hook chain ---

func TestAgent_Hook_Blocks_ToolNotExecuted(t *testing.T) {
	client := &mockLLMClient{
		responses: []LLMResponse{
			toolUseResp("c0", "bash", map[string]any{"command": "rm -rf /"}),
			textResp("ok"),
		},
	}
	exec := &mockToolExecutor{result: "should not run"}
	agent := NewAgent(client)
	agent.AddTool(ToolDefinition{Name: "bash", RiskLevel: RiskExecute}, exec)
	agent.AddPreToolUseHook(func(_ string, _ RiskLevel, _ map[string]any) error {
		return errors.New("denied")
	})

	memory := NewInMemory()
	if err := agent.Run(context.Background(), memory, "x"); err != nil {
		t.Fatalf("hook block non deve propagare errore: %v", err)
	}
	if exec.calls != 0 {
		t.Errorf("executor non doveva essere chiamato, calls=%d", exec.calls)
	}

	trb := memory.Messages()[2].Content[0].(ToolResultBlock)
	if !trb.IsError {
		t.Error("IsError deve essere true su block")
	}
	if !strings.Contains(trb.Content, "blocked") || !strings.Contains(trb.Content, "denied") {
		t.Errorf("Content non contiene il marker di block: %q", trb.Content)
	}
}

func TestAgent_Hook_ChainStopsAtFirstError(t *testing.T) {
	client := &mockLLMClient{
		responses: []LLMResponse{
			toolUseResp("c0", "t", nil),
			textResp("ok"),
		},
	}
	exec := &mockToolExecutor{}
	agent := NewAgent(client)
	agent.AddTool(ToolDefinition{Name: "t", RiskLevel: RiskWrite}, exec)

	firstCalled, secondCalled, thirdCalled := 0, 0, 0
	agent.AddPreToolUseHook(func(_ string, _ RiskLevel, _ map[string]any) error {
		firstCalled++
		return nil
	})
	agent.AddPreToolUseHook(func(_ string, _ RiskLevel, _ map[string]any) error {
		secondCalled++
		return errors.New("nope")
	})
	agent.AddPreToolUseHook(func(_ string, _ RiskLevel, _ map[string]any) error {
		thirdCalled++
		return nil
	})

	agent.Run(context.Background(), NewInMemory(), "x")

	if firstCalled != 1 || secondCalled != 1 {
		t.Errorf("primi due hook attesi 1/1, ottenuti %d/%d", firstCalled, secondCalled)
	}
	if thirdCalled != 0 {
		t.Errorf("terzo hook non doveva eseguire, calls=%d", thirdCalled)
	}
	if exec.calls != 0 {
		t.Error("executor non doveva eseguire dopo block")
	}
}

func TestAgent_Hook_ReceivesCorrectRiskLevel(t *testing.T) {
	client := &mockLLMClient{
		responses: []LLMResponse{
			toolUseResp("c0", "bash", nil),
			textResp("ok"),
		},
	}
	agent := NewAgent(client)
	agent.AddTool(ToolDefinition{Name: "bash", RiskLevel: RiskExecute}, &mockToolExecutor{})

	var seen RiskLevel
	agent.AddPreToolUseHook(func(_ string, level RiskLevel, _ map[string]any) error {
		seen = level
		return nil
	})

	agent.Run(context.Background(), NewInMemory(), "x")
	if seen != RiskExecute {
		t.Errorf("livello atteso RiskExecute, ottenuto %v", seen)
	}
}

func TestAgent_ToolEvent_NotEmittedOnBlock(t *testing.T) {
	client := &mockLLMClient{
		responses: []LLMResponse{
			toolUseResp("c0", "t", nil),
			textResp("ok"),
		},
	}
	agent := NewAgent(client)
	agent.AddTool(ToolDefinition{Name: "t", RiskLevel: RiskWrite}, &mockToolExecutor{})
	agent.AddPreToolUseHook(func(_ string, _ RiskLevel, _ map[string]any) error {
		return errors.New("denied")
	})

	cap := &captureEmitter{}

	agent.Run(context.Background(), NewInMemory(), "x")
	if cap.toolCalls != 0 || cap.toolResults != 0 {
		t.Errorf("emitter non doveva emettere tool events su block: calls=%d results=%d", cap.toolCalls, cap.toolResults)
	}
}

func TestAgent_ToolEvent_EmittedOnSuccess(t *testing.T) {
	client := &mockLLMClient{
		responses: []LLMResponse{
			toolUseResp("c0", "t", nil),
			textResp("ok"),
		},
	}
	agent := NewAgent(client)
	agent.AddTool(ToolDefinition{Name: "t"}, &mockToolExecutor{result: "done"})

	cap := &captureEmitter{}

	agent.Run(context.Background(), NewInMemory(), "x")
	if cap.lastName != "t" || cap.lastResult != "done" || cap.lastIsError {
		t.Errorf("emitter args inattesi: name=%q result=%q isError=%v", cap.lastName, cap.lastResult, cap.lastIsError)
	}
	if cap.toolResults != 1 {
		t.Errorf("atteso 1 ToolResult emesso, ottenuto %d", cap.toolResults)
	}
}
