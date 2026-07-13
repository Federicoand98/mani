package core

import (
	"context"
	"testing"
)

func respondSchema() ToolInputSchema {
	return ToolInputSchema{
		Type: "object",
		Properties: map[string]ToolProperty{
			"sentiment": {Type: "string", Enum: []string{"positive", "negative", "neutral"}},
			"score":     {Type: "number"},
		},
		Required: []string{"sentiment", "score"},
	}
}

func newStructuredAgent(client LLMClient, exec ToolExecutor) *Agent {
	a := NewAgent(client)
	a.AddTool(ToolDefinition{Name: "respond", InputSchema: respondSchema()}, exec)
	a.SetFinalTool("respond")
	return a
}

// Regressione del loop infinito: un respond valido deve CATTURARE il risultato e terminare.
func TestAgent_FinalTool_TerminatesOnValidRespond(t *testing.T) {
	client := NewMock(RespToolCall("1", "respond", map[string]any{"sentiment": "positive", "score": 1.0}))
	a := newStructuredAgent(client, &mockToolExecutor{name: "respond"})

	if err := a.Run(context.Background(), NewInMemory(), "adoro"); err != nil {
		t.Fatalf("errore inatteso: %v", err)
	}
	got := a.FinalResult()
	if got == nil || got["sentiment"] != "positive" {
		t.Fatalf("finalResult atteso {sentiment:positive}, ottenuto %v", got)
	}
}

// Payload invalido → feedback di errore → il modello ritenta col payload corretto.
func TestAgent_FinalTool_RetriesOnInvalid(t *testing.T) {
	client := NewMock(
		RespToolCall("1", "respond", map[string]any{"sentiment": "positive"}), // manca 'score'
		RespToolCall("2", "respond", map[string]any{"sentiment": "positive", "score": 0.9}),
	)
	a := newStructuredAgent(client, &mockToolExecutor{name: "respond"})

	if err := a.Run(context.Background(), NewInMemory(), "x"); err != nil {
		t.Fatalf("errore inatteso: %v", err)
	}
	if a.FinalResult()["score"] != 0.9 {
		t.Fatalf("atteso retry con score valido, ottenuto %v", a.FinalResult())
	}
}

// end_turn con schema attivo ma senza respond → il guard re-prompta, poi respond.
func TestAgent_FinalTool_GuardRepromptsOnText(t *testing.T) {
	client := NewMock(
		RespText("il sentiment è positivo"), // end_turn: il guard deve forzare
		RespToolCall("1", "respond", map[string]any{"sentiment": "positive", "score": 1.0}),
	)
	a := newStructuredAgent(client, &mockToolExecutor{name: "respond"})

	if err := a.Run(context.Background(), NewInMemory(), "x"); err != nil {
		t.Fatalf("errore inatteso: %v", err)
	}
	if a.FinalResult() == nil {
		t.Fatal("il guard doveva forzare respond, finalResult nil")
	}
}

// Il tool terminale NON deve passare dall'executor (è intercettato prima).
func TestAgent_FinalTool_NotExecuted(t *testing.T) {
	exec := &mockToolExecutor{name: "respond", result: "SHOULD NOT RUN"}
	client := NewMock(RespToolCall("1", "respond", map[string]any{"sentiment": "neutral", "score": 0.5}))
	a := newStructuredAgent(client, exec)

	a.Run(context.Background(), NewInMemory(), "x")
	if exec.calls != 0 {
		t.Errorf("il final tool non deve essere eseguito, calls=%d", exec.calls)
	}
}
