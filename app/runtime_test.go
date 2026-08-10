package app

import (
	"context"
	"testing"

	"github.com/Federicoand98/mani/core"
)

// Regressione della condizione invertita: LastResponse ritorna il testo dell'ASSISTANT,
// non l'eco del prompt utente.
func TestLastResponse_ReturnsAssistantText(t *testing.T) {
	client := core.NewMock(core.RespText("la risposta finale"))
	rt := testRuntime(t, client)

	for range rt.Execute(context.Background(), "domanda utente") {
	}

	if got := rt.LastResponse(); got != "la risposta finale" {
		t.Errorf("LastResponse = %q, atteso 'la risposta finale'", got)
	}
}

// Senza output_schema, EventDone porta Result nil e il testo in Text.
// (il fallback {"response": ...} vive ora nei consumatori, es. server/rest.go)
func TestExecute_DonePayload_NoSchema(t *testing.T) {
	client := core.NewMock(core.RespText("ciao"))
	rt := testRuntime(t, client)

	var done DonePayload
	var seen bool
	for ev := range rt.Execute(context.Background(), "x") {
		if ev.Type == EventDone {
			done, seen = ev.Payload.(DonePayload), true
		}
	}

	if !seen {
		t.Fatal("EventDone non ricevuto")
	}
	if done.Result != nil {
		t.Errorf("senza schema Result deve essere nil, ottenuto %v", done.Result)
	}
	if done.Text != "ciao" {
		t.Errorf("Text = %q, atteso 'ciao'", done.Text)
	}
}
