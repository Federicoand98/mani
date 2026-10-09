package app

import (
	"context"
	"errors"
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

// --- provenance (fase 36, traccia A) ---

// journaledRuntime is a Runtime with an in-memory journal, so a test can ask
// what the audit trail recorded for the run it just executed.
func journaledRuntime(t *testing.T, client core.LLMClient) (*Runtime, *InMemoryJournal) {
	t.Helper()
	rt := testRuntime(t, client)
	j := NewInMemoryJournal(10)
	RegisterJournal(rt, j)
	return rt, j
}

func onlyRun(t *testing.T, j *InMemoryJournal) RunRecord {
	t.Helper()
	metas, err := j.List(ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(metas) != 1 {
		t.Fatalf("journal has %d runs, want 1", len(metas))
	}
	rec, err := j.Get(metas[0].ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	return rec
}

// Without the id in the payload, a caller holding a result has no way back to
// the run that produced it: correlating by timestamp is not correlating.
func TestExecuteIn_DonePayloadCarriesTheRunID(t *testing.T) {
	rt, j := journaledRuntime(t, core.NewMock(core.RespText("fatto")))

	var done DonePayload
	ch, cancel := rt.ExecuteIn(context.Background(), rt.CurrentSession(), "x")
	defer cancel()
	for ev := range ch {
		if ev.Type == EventDone {
			done = ev.Payload.(DonePayload)
		}
	}

	if done.RunID == "" {
		t.Fatal("DonePayload.RunID is empty")
	}
	if got := onlyRun(t, j); got.ID != done.RunID {
		t.Errorf("journal run %q, payload %q", got.ID, done.RunID)
	}
}

// A failed run is the one you most want to look up afterwards.
func TestExecuteIn_ErrorPayloadCarriesTheRunID(t *testing.T) {
	client := core.NewMock()
	client.Err = errors.New("provider unreachable")
	rt, j := journaledRuntime(t, client)

	var failed ErrorPayload
	ch, cancel := rt.ExecuteIn(context.Background(), rt.CurrentSession(), "x")
	defer cancel()
	for ev := range ch {
		if ev.Type == EventError {
			failed = ev.Payload.(ErrorPayload)
		}
	}

	if failed.Err == nil {
		t.Fatal("no EventError")
	}
	if failed.RunID == "" {
		t.Fatal("ErrorPayload.RunID is empty")
	}
	rec := onlyRun(t, j)
	if rec.ID != failed.RunID || rec.Status != "error" {
		t.Errorf("journal = %s/%s, payload id %q", rec.ID, rec.Status, failed.RunID)
	}
}

func TestExecuteIn_JournalsTheStructuredResult(t *testing.T) {
	client := core.NewMock(core.RespToolCall("1", "respond", map[string]any{"label": "positive", "score": 0.9}))
	rt, j := journaledRuntime(t, client)
	rt.agent.SetFinalTool("respond")

	ch, cancel := rt.ExecuteIn(context.Background(), rt.CurrentSession(), "classify")
	defer cancel()
	for range ch {
	}

	rec := onlyRun(t, j)
	if rec.Status != "ok" {
		t.Fatalf("status = %q", rec.Status)
	}
	if rec.Results["label"] != "positive" {
		t.Errorf("journalled result = %+v, want the structured answer", rec.Results)
	}
}

func TestExecuteIn_TextRunLeavesNoResultInTheJournal(t *testing.T) {
	rt, j := journaledRuntime(t, core.NewMock(core.RespText("solo testo")))

	ch, cancel := rt.ExecuteIn(context.Background(), rt.CurrentSession(), "x")
	defer cancel()
	for range ch {
	}

	if rec := onlyRun(t, j); rec.Results != nil {
		t.Errorf("result = %+v, want none for a run that answered in text", rec.Results)
	}
}
