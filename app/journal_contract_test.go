package app

import (
	"path/filepath"
	"testing"
	"time"
)

func TestJournalContract(t *testing.T) {
	type factory struct {
		name string
		new  func(t *testing.T) (Journal, func())
	}
	factories := []factory{
		{
			name: "memory",
			new: func(t *testing.T) (Journal, func()) {
				return NewInMemoryJournal(10), func() {}
			},
		},
		{
			name: "jsonl",
			new: func(t *testing.T) (Journal, func()) {
				j, err := NewJSONLJournal(t.TempDir())
				if err != nil {
					t.Fatalf("NewJSONLJournal: %v", err)
				}
				return j, func() {}
			},
		},
		{
			name: "sqlite",
			new: func(t *testing.T) (Journal, func()) {
				j, err := NewSQLiteJournal(filepath.Join(t.TempDir(), "runs.db"), 10)
				if err != nil {
					t.Fatalf("NewSQLiteJournal: %v", err)
				}
				return j, func() { _ = j.Close() }
			},
		},
	}

	base := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	for _, tc := range factories {
		t.Run(tc.name, func(t *testing.T) {
			j, cleanup := tc.new(t)
			defer cleanup()
			if err := j.Start(RunRecord{ID: "run", SessionID: "session", Source: "test", StartedAt: base}); err != nil {
				t.Fatalf("Start: %v", err)
			}
			if err := j.Append(RunEvent{RunID: "run", At: base.Add(time.Second), Kind: EvLLMReponse,
				Data: map[string]any{"in_tokens": 3, "out_tokens": 2}}); err != nil {
				t.Fatalf("Append response: %v", err)
			}
			if err := j.Append(RunEvent{RunID: "run", At: base.Add(2 * time.Second), Kind: EvToolResult,
				Data: map[string]any{"is_error": true}}); err != nil {
				t.Fatalf("Append tool result: %v", err)
			}
			// Blocked and Masked have been wrong twice, both times because a
			// producer and a counter disagreed on the word. They are only
			// compared across adapters if the fixture actually contains them.
			if err := j.Append(RunEvent{RunID: "run", At: base.Add(3 * time.Second), Kind: EvGuardrail,
				Data: map[string]any{"tool": "bash", "action": "deny"}}); err != nil {
				t.Fatalf("Append guardrail deny: %v", err)
			}
			if err := j.Append(RunEvent{RunID: "run", At: base.Add(4 * time.Second), Kind: EvGuardrail,
				Data: map[string]any{"tool": "read", "action": "mask"}}); err != nil {
				t.Fatalf("Append guardrail mask: %v", err)
			}
			if err := j.Finish("run", "error"); err != nil {
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

			metas, err := j.List(ListFilter{SessionID: "session", Status: "error", Since: base.Add(-time.Second), Limit: 1})
			if err != nil || len(metas) != 1 || metas[0].ID != "run" || metas[0].Summary != rec.Summary {
				t.Fatalf("List = %+v, err=%v", metas, err)
			}
		})
	}
}
