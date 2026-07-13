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

// StructuredResult senza schema avvolge il testo in {"response": ...}.
func TestStructuredResult_FallbackWrapsText(t *testing.T) {
	client := core.NewMock(core.RespText("ciao"))
	rt := testRuntime(t, client)

	for range rt.Execute(context.Background(), "x") {
	}

	res := rt.StructuredResult()
	if res["response"] != "ciao" {
		t.Errorf("StructuredResult fallback atteso {response:ciao}, ottenuto %v", res)
	}
}
