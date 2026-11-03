package sqlite

import (
	"database/sql"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Federicoand98/mani/journal"
	"github.com/Federicoand98/mani/journal/journaltest"
)

func TestSQLiteJournal_RoundTripAndRetention(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "runs.db")
	j, err := New(dbPath, 2)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	if err := j.Start(journal.RunRecord{ID: "run-a", SessionID: "session-a", Source: "cli", StartedAt: base}); err != nil {
		t.Fatalf("Start(run-a): %v", err)
	}
	if err := j.Append(journal.RunEvent{
		RunID: "run-a", At: base.Add(time.Second), Kind: journal.EvLLMReponse,
		Data: map[string]any{"in_tokens": 12, "out_tokens": 3},
	}); err != nil {
		t.Fatalf("Append(llm): %v", err)
	}
	if err := j.Append(journal.RunEvent{
		RunID: "run-a", At: base.Add(2 * time.Second), Kind: journal.EvToolResult,
		Data: map[string]any{"tool": "read", "is_error": true},
	}); err != nil {
		t.Fatalf("Append(tool): %v", err)
	}
	if err := j.Append(journal.RunEvent{
		RunID: "run-a", At: base.Add(3 * time.Second), Kind: journal.EvGuardrail,
		Data: map[string]any{"action": "deny"},
	}); err != nil {
		t.Fatalf("Append(guardrail): %v", err)
	}
	if err := j.Finish("run-a", journal.RunOutcome{Status: "error"}); err != nil {
		t.Fatalf("Finish(run-a): %v", err)
	}

	got, err := j.Get("run-a")
	if err != nil {
		t.Fatalf("Get(run-a): %v", err)
	}
	if got.ID != "run-a" || got.SessionID != "session-a" || got.Source != "cli" || got.Status != "error" {
		t.Fatalf("header mismatch: %+v", got)
	}
	if !got.StartedAt.Equal(base) || len(got.Events) != 5 {
		t.Fatalf("timeline mismatch: started=%v events=%d", got.StartedAt, len(got.Events))
	}
	for seq, ev := range got.Events {
		if ev.Seq != seq {
			t.Errorf("event %d has sequence %d", seq, ev.Seq)
		}
	}
	if got.Summary.LLMCalls != 1 || got.Summary.InTokens != 12 || got.Summary.OutTokens != 3 ||
		got.Summary.ToolCalls != 1 || got.Summary.Errors != 1 || got.Summary.Blocked != 1 {
		t.Fatalf("summary mismatch: %+v", got.Summary)
	}

	if err := j.Start(journal.RunRecord{ID: "run-b", SessionID: "session-b", Source: "trigger", StartedAt: base.Add(time.Minute)}); err != nil {
		t.Fatalf("Start(run-b): %v", err)
	}
	if err := j.Finish("run-b", journal.RunOutcome{Status: "ok"}); err != nil {
		t.Fatalf("Finish(run-b): %v", err)
	}

	metas, err := j.List(journal.ListFilter{SessionID: "session-b", Status: "ok", Since: base.Add(30 * time.Second), Limit: 1})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(metas) != 1 || metas[0].ID != "run-b" || metas[0].Status != "ok" {
		t.Fatalf("filtered list mismatch: %+v", metas)
	}

	if err := j.Start(journal.RunRecord{ID: "run-c", SessionID: "session-c", Source: "trigger", StartedAt: base.Add(2 * time.Minute)}); err != nil {
		t.Fatalf("Start(run-c): %v", err)
	}
	if _, err := j.Get("run-a"); err == nil {
		t.Fatal("retention should remove the oldest run")
	}

	if err := j.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	reopened, err := New(dbPath, 2)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()
	if got, err := reopened.Get("run-b"); err != nil || got.Status != "ok" {
		t.Fatalf("persisted run-b: got=%+v err=%v", got, err)
	}
}

func TestSQLiteJournal_ConfiguresWALAndNormalSync(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runs.db")
	j, err := New(path, 10)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer j.Close()

	var mode string
	if err := j.db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Fatalf("journal_mode = %q, want wal", mode)
	}
	var synchronous int
	if err := j.db.QueryRow("PRAGMA synchronous").Scan(&synchronous); err != nil {
		t.Fatalf("synchronous: %v", err)
	}
	if synchronous != 1 { // NORMAL
		t.Fatalf("synchronous = %d, want NORMAL (1)", synchronous)
	}
}

func TestSQLiteJournal_RetainsEventsBeforeRunStart(t *testing.T) {
	j, err := New(filepath.Join(t.TempDir(), "runs.db"), 10)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer j.Close()

	if err := j.Append(journal.RunEvent{RunID: "run", Kind: journal.EvToolCall, Data: map[string]any{"tool": "read"}}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	got, err := j.Get("run")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got.Events) != 1 || got.Events[0].Kind != journal.EvToolCall {
		t.Fatalf("events = %+v, want the pre-start event", got.Events)
	}
	if metas, err := j.List(journal.ListFilter{}); err != nil || len(metas) != 1 || metas[0].ID != "run" {
		t.Fatalf("List = %+v, err=%v", metas, err)
	}
}

func TestSQLiteJournal_BackfillsExistingEvents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runs.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	_, err = db.Exec(`
CREATE TABLE journal_events (run_id TEXT NOT NULL, seq INTEGER NOT NULL, at INTEGER NOT NULL, depth INTEGER NOT NULL, kind TEXT NOT NULL, data TEXT NOT NULL, PRIMARY KEY (run_id, seq));
CREATE TABLE journal_sequences (run_id TEXT PRIMARY KEY, next_seq INTEGER NOT NULL);
INSERT INTO journal_events VALUES ('legacy', 0, 1000000000, 0, 'run_start', '{"source":"legacy","session_id":"s"}');
INSERT INTO journal_events VALUES ('legacy', 1, 2000000000, 0, 'llm_response', '{"in_tokens":4,"out_tokens":2}');
`)
	if err != nil {
		db.Close()
		t.Fatalf("seed legacy database: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close legacy database: %v", err)
	}

	j, err := New(path, 10)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer j.Close()
	metas, err := j.List(journal.ListFilter{SessionID: "s", Status: "running"})
	if err != nil || len(metas) != 1 {
		t.Fatalf("List after backfill = %+v, err=%v", metas, err)
	}
	if metas[0].Summary.LLMCalls != 1 || metas[0].Summary.InTokens != 4 || metas[0].Summary.OutTokens != 2 {
		t.Fatalf("backfilled summary = %+v", metas[0].Summary)
	}
}

func TestSQLiteJournal_ConcurrentAppendsHaveUniqueSequences(t *testing.T) {
	j, err := New(filepath.Join(t.TempDir(), "runs.db"), 10)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer j.Close()

	if err := j.Start(journal.RunRecord{ID: "run", StartedAt: time.Now()}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	const appendCount = 32
	var wg sync.WaitGroup
	errs := make(chan error, appendCount)
	for i := 0; i < appendCount; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			errs <- j.Append(journal.RunEvent{RunID: "run", Kind: journal.EvToolCall, Data: map[string]any{"index": index}})
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Append: %v", err)
		}
	}

	got, err := j.Get("run")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got.Events) != appendCount+1 {
		t.Fatalf("got %d events, want %d", len(got.Events), appendCount+1)
	}
	for seq, ev := range got.Events {
		if ev.Seq != seq {
			t.Errorf("event %d has sequence %d", seq, ev.Seq)
		}
	}
}

func TestContract(t *testing.T) {
	j, err := New(filepath.Join(t.TempDir(), "runs.db"), 10)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	journaltest.Run(t, j)
}
