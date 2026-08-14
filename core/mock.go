package core

/* AI generated */

import "context"

// MockClient è un LLMClient finto per i test, esportato così che anche i test di app/ e server/
// possano usarlo (core.NewMock). Ritorna risposte scriptate in coda, una per ogni Send.
// Registra ciò che l'agent ha inviato (Sent / SentTools) per le assert.
type MockClient struct {
	responses []LLMResponse
	idx       int

	// Err, se impostato, viene ritornato da ogni Send (per testare errori di rete).
	Err error
	// Fn, se impostata, ha la precedenza sulla coda: il mock reagisce alla conversazione
	// (escape hatch per i test che devono ispezionare messaggi/tool).
	Fn func(msgs []Message, tools []ToolDefinition) LLMResponse

	Sent      [][]Message        // messaggi passati a ogni Send
	SentTools [][]ToolDefinition // tool passati a ogni Send
}

// NewMock crea un mock che ritorna le risposte date, in ordine. Esaurita la coda, ritorna end_turn.
func NewMock(responses ...LLMResponse) *MockClient {
	return &MockClient{responses: responses}
}

// NewMockFunc crea un mock programmabile che decide la risposta in base alla conversazione.
func NewMockFunc(fn func(msgs []Message, tools []ToolDefinition) LLMResponse) *MockClient {
	return &MockClient{Fn: fn}
}

func (m *MockClient) Send(_ context.Context, msgs []Message, tools []ToolDefinition, _ TokenHandler) (LLMResponse, error) {
	m.Sent = append(m.Sent, msgs)
	m.SentTools = append(m.SentTools, tools)

	if m.Err != nil {
		return LLMResponse{}, m.Err
	}
	if m.Fn != nil {
		return m.Fn(msgs, tools), nil
	}
	if m.idx >= len(m.responses) {
		return LLMResponse{StopReason: StopReasonEndTurn}, nil // coda finita: chiudi il turno
	}
	r := m.responses[m.idx]
	m.idx++
	return r, nil
}

// --- builder helper (mini-DSL leggibile) ---

// RespText: risposta finale in testo (end_turn).
func RespText(s string) LLMResponse {
	return LLMResponse{StopReason: StopReasonEndTurn, Content: []ContentBlock{TextBlock{Text: s}}}
}

// RespToolCall: il modello chiama un tool (tool_use).
func RespToolCall(id, name string, input map[string]any) LLMResponse {
	return LLMResponse{StopReason: StopReasonToolUse, Content: []ContentBlock{ToolUseBlock{ID: id, Name: name, Input: input}}}
}

// WithUsage: aggiunge il conteggio token a una risposta (per i test di budget).
func WithUsage(r LLMResponse, in, out int) LLMResponse {
	r.Usage = TokenUsage{InputTokens: in, OutputTokens: out}
	return r
}
