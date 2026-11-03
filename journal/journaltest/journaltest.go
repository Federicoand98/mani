// Package journaltest is the contract every journal.Journal must honour. It is
// exported so an adapter written outside this repository can run the same
// checks as the ones in it: a port with several implementations is only a
// port while they agree, and per-adapter tests are how they stop agreeing.
package journaltest

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/Federicoand98/mani/journal"
)

// Run checks j against the contract. j must be empty; Run writes two runs to it.
func Run(t *testing.T, j journal.Journal) {
	t.Helper()
	base := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := j.Start(journal.RunRecord{ID: "run", SessionID: "session", Source: "test", StartedAt: base}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := j.Append(journal.RunEvent{RunID: "run", At: base.Add(time.Second), Kind: journal.EvLLMReponse,
		Data: map[string]any{"in_tokens": 3, "out_tokens": 2}}); err != nil {
		t.Fatalf("Append response: %v", err)
	}
	if err := j.Append(journal.RunEvent{RunID: "run", At: base.Add(2 * time.Second), Kind: journal.EvToolResult,
		Data: map[string]any{"is_error": true}}); err != nil {
		t.Fatalf("Append tool result: %v", err)
	}
	// Blocked and Masked have been wrong twice, both times because a
	// producer and a counter disagreed on the word. They are only
	// compared across adapters if the fixture actually contains them.
	if err := j.Append(journal.RunEvent{RunID: "run", At: base.Add(3 * time.Second), Kind: journal.EvGuardrail,
		Data: map[string]any{"tool": "bash", "action": "deny"}}); err != nil {
		t.Fatalf("Append guardrail deny: %v", err)
	}
	if err := j.Append(journal.RunEvent{RunID: "run", At: base.Add(4 * time.Second), Kind: journal.EvGuardrail,
		Data: map[string]any{"tool": "read", "action": "mask"}}); err != nil {
		t.Fatalf("Append guardrail mask: %v", err)
	}
	if err := j.Finish("run", journal.RunOutcome{Status: "error", Result: map[string]any{"label": "positive"}}); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	rec, err := j.Get("run")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if rec.SessionID != "session" || rec.Source != "test" || rec.Status != "error" || !rec.StartedAt.Equal(base) {
		t.Fatalf("metadata = %+v", rec.Meta())
	}
	if rec.Summary.LLMCalls != 1 || rec.Summary.ToolCalls != 1 || rec.Summary.InTokens != 3 || rec.Summary.OutTokens != 2 || rec.Summary.Errors != 1 {
		t.Fatalf("summary = %+v", rec.Summary)
	}
	if rec.Summary.Blocked != 1 || rec.Summary.Masked != 1 {
		t.Fatalf("guardrail counters = blocked %d, masked %d, want 1 and 1", rec.Summary.Blocked, rec.Summary.Masked)
	}

	// The audit trail has to answer "what did the agent reply", not only
	// "did it work". The result rides in the run_end event, so every
	// adapter that folds events gets it: this is what stops one of them
	// from quietly dropping it.
	if rec.Results["label"] != "positive" {
		t.Fatalf("result = %+v, want the structured result of the run", rec.Results)
	}

	metas, err := j.List(journal.ListFilter{SessionID: "session", Status: "error", Since: base.Add(-time.Second), Limit: 1})
	if err != nil || len(metas) != 1 || metas[0].ID != "run" || metas[0].Summary != rec.Summary {
		t.Fatalf("List = %+v, err=%v", metas, err)
	}

	// A run that answered in text has no structured result: the field is
	// absent, and the JSON of the record must not carry a null for it.
	if err := j.Start(journal.RunRecord{ID: "plain", SessionID: "session", Source: "test", StartedAt: base}); err != nil {
		t.Fatalf("Start(plain): %v", err)
	}
	if err := j.Finish("plain", journal.RunOutcome{Status: "ok"}); err != nil {
		t.Fatalf("Finish(plain): %v", err)
	}
	plain, err := j.Get("plain")
	if err != nil {
		t.Fatalf("Get(plain): %v", err)
	}
	if plain.Results != nil {
		t.Errorf("result = %+v, want none", plain.Results)
	}
	if b, _ := json.Marshal(plain); bytes.Contains(b, []byte(`"results":null`)) {
		t.Errorf("record JSON carries a null result: %s", b)
	}
}
