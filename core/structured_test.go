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

	res, err := a.Run(context.Background(), NewInMemory(), "adoro", nil)
	if err != nil {
		t.Fatalf("errore inatteso: %v", err)
	}
	if res.FinalResult == nil || res.FinalResult["sentiment"] != "positive" {
		t.Fatalf("FinalResult atteso {sentiment:positive}, ottenuto %v", res.FinalResult)
	}
}

// Payload invalido → feedback di errore → il modello ritenta col payload corretto.
func TestAgent_FinalTool_RetriesOnInvalid(t *testing.T) {
	client := NewMock(
		RespToolCall("1", "respond", map[string]any{"sentiment": "positive"}), // manca 'score'
		RespToolCall("2", "respond", map[string]any{"sentiment": "positive", "score": 0.9}),
	)
	a := newStructuredAgent(client, &mockToolExecutor{name: "respond"})

	res, err := a.Run(context.Background(), NewInMemory(), "x", nil)
	if err != nil {
		t.Fatalf("errore inatteso: %v", err)
	}
	if res.FinalResult["score"] != 0.9 {
		t.Fatalf("atteso retry con score valido, ottenuto %v", res.FinalResult)
	}
}

// end_turn con schema attivo ma senza respond → il guard re-prompta, poi respond.
func TestAgent_FinalTool_GuardRepromptsOnText(t *testing.T) {
	client := NewMock(
		RespText("il sentiment è positivo"), // end_turn: il guard deve forzare
		RespToolCall("1", "respond", map[string]any{"sentiment": "positive", "score": 1.0}),
	)
	a := newStructuredAgent(client, &mockToolExecutor{name: "respond"})

	res, err := a.Run(context.Background(), NewInMemory(), "x", nil)
	if err != nil {
		t.Fatalf("errore inatteso: %v", err)
	}
	if res.FinalResult == nil {
		t.Fatal("il guard doveva forzare respond, FinalResult nil")
	}
}

// Il tool terminale NON deve passare dall'executor (è intercettato prima).
func TestAgent_FinalTool_NotExecuted(t *testing.T) {
	exec := &mockToolExecutor{name: "respond", result: "SHOULD NOT RUN"}
	client := NewMock(RespToolCall("1", "respond", map[string]any{"sentiment": "neutral", "score": 0.5}))
	a := newStructuredAgent(client, exec)

	a.Run(context.Background(), NewInMemory(), "x", nil)
	if exec.calls != 0 {
		t.Errorf("il final tool non deve essere eseguito, calls=%d", exec.calls)
	}
}

// The point of the deep validation: a nested violation must stop the run just
// like a missing top-level field, or a declared shape is a suggestion.
func TestAgent_FinalTool_RetriesOnNestedViolation(t *testing.T) {
	schema := ToolInputSchema{
		Type: "object",
		Properties: map[string]ToolProperty{
			"letters": {Type: "array", Items: &ToolProperty{
				Type: "object",
				Properties: map[string]ToolProperty{
					"place": {Type: "string", Enum: []string{"Mantova", "Ferrara"}},
					"year":  {Type: "integer"},
				},
				Required: []string{"place"},
			}},
		},
		Required: []string{"letters"},
	}

	cases := []struct {
		name string
		bad  map[string]any
	}{
		{"item of the wrong type", map[string]any{"letters": []any{"Mantova"}}},
		{"nested required missing", map[string]any{"letters": []any{map[string]any{"year": 1495.0}}}},
		{"nested value outside the enum", map[string]any{"letters": []any{map[string]any{"place": "Milano"}}}},
		{"nested integer not integer", map[string]any{"letters": []any{map[string]any{"place": "Mantova", "year": 1495.5}}}},
	}
	good := map[string]any{"letters": []any{map[string]any{"place": "Mantova", "year": 1495.0}}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := NewMock(
				RespToolCall("1", "respond", tc.bad),
				RespToolCall("2", "respond", good),
			)
			a := NewAgent(client)
			a.AddTool(ToolDefinition{Name: "respond", InputSchema: schema}, &mockToolExecutor{name: "respond"})
			a.SetFinalTool("respond")

			res, err := a.Run(context.Background(), NewInMemory(), "x", nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			letters, _ := res.FinalResult["letters"].([]any)
			if len(letters) != 1 {
				t.Fatalf("FinalResult = %v, want the retried answer", res.FinalResult)
			}
			if first, _ := letters[0].(map[string]any); first["place"] != "Mantova" {
				t.Errorf("FinalResult = %v, want the invalid answer refused and the retry kept", res.FinalResult)
			}
		})
	}
}
