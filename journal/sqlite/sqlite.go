package sqlite

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Federicoand98/mani/journal"
	_ "modernc.org/sqlite"
)

const sqliteJournalSchema = `
CREATE TABLE IF NOT EXISTS journal_events (
    run_id TEXT NOT NULL,
    seq INTEGER NOT NULL,
    at INTEGER NOT NULL,
    depth INTEGER NOT NULL,
    kind TEXT NOT NULL,
    data TEXT NOT NULL,
    PRIMARY KEY (run_id, seq)
);
CREATE TABLE IF NOT EXISTS journal_sequences (
    run_id TEXT PRIMARY KEY,
    next_seq INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS journal_events_kind_at
    ON journal_events (kind, at DESC);
CREATE TABLE IF NOT EXISTS journal_runs (
    run_id TEXT PRIMARY KEY,
    started_at INTEGER NOT NULL DEFAULT 0,
    ended_at INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT '',
    session_id TEXT NOT NULL DEFAULT '',
    llm_calls INTEGER NOT NULL DEFAULT 0,
    tool_calls INTEGER NOT NULL DEFAULT 0,
    in_tokens INTEGER NOT NULL DEFAULT 0,
    out_tokens INTEGER NOT NULL DEFAULT 0,
    blocked INTEGER NOT NULL DEFAULT 0,
    masked INTEGER NOT NULL DEFAULT 0,
    errors INTEGER NOT NULL DEFAULT 0,
    agent TEXT NOT NULL DEFAULT '',
    task TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS journal_runs_started_at
    ON journal_runs (started_at DESC, run_id DESC);
`

// Journal stores the same event stream as journal.JSONL in a single
// SQLite database. The modernc driver keeps the adapter cgo-free.
type Journal struct {
	mu      sync.RWMutex
	db      *sql.DB
	maxRuns int
	closed  bool
}

var _ journal.Journal = (*Journal)(nil)

// New opens a cgo-free SQLite journal. Retention defaults to 100
// runs when non-positive.
func New(path string, retention int) (*Journal, error) {
	if path == "" {
		return nil, fmt.Errorf("journal: sqlite path is required")
	}
	if !strings.HasPrefix(path, "file:") && path != ":memory:" {
		if dir := filepath.Dir(path); dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, fmt.Errorf("journal: mkdir %s: %w", dir, err)
			}
		}
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("journal: open sqlite %s: %w", path, err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	maxRuns := journal.Retention(retention)
	j := &Journal{db: db, maxRuns: maxRuns}
	if _, err := db.Exec("PRAGMA busy_timeout = 5000"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("journal: configure sqlite %s: %w", path, err)
	}
	for _, pragma := range []string{"PRAGMA journal_mode = WAL", "PRAGMA synchronous = NORMAL"} {
		if _, err := db.Exec(pragma); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("journal: configure sqlite %s: %w", path, err)
		}
	}
	if _, err := db.Exec(sqliteJournalSchema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("journal: initialize sqlite %s: %w", path, err)
	}
	if err := addColumns(db, "journal_runs", "agent", "task"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("journal: migrate sqlite %s: %w", path, err)
	}
	if err := j.backfillRuns(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := j.prune(); err != nil {
		_ = db.Close()
		return nil, err
	}

	return j, nil
}

// addColumns adds the TEXT columns a database created by an older mani lacks:
// CREATE TABLE IF NOT EXISTS leaves an existing table as it was. Its rows keep
// ” there, as runs recorded before the column existed.
func addColumns(db *sql.DB, table string, cols ...string) error {
	rows, err := db.Query("SELECT name FROM pragma_table_info(?)", table)
	if err != nil {
		return err
	}
	have := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			_ = rows.Close()
			return err
		}
		have[name] = true
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, c := range cols {
		if !have[c] {
			if _, err := db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s TEXT NOT NULL DEFAULT ''", table, c)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (j *Journal) Close() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return nil
	}
	j.closed = true
	return j.db.Close()
}

func (j *Journal) Start(rec journal.RunRecord) error {
	return j.writeEvent(journal.RunEvent{
		RunID: rec.ID, At: rec.StartedAt, Kind: journal.EvRunStart, Data: journal.StartData(rec),
	})
}

func (j *Journal) Append(ev journal.RunEvent) error {
	return j.writeEvent(ev)
}

func (j *Journal) Finish(runID string, out journal.RunOutcome) error {
	return j.writeEvent(journal.RunEvent{RunID: runID, At: time.Now(), Kind: journal.EvRunEnd, Data: journal.EndData(out)})
}

func (j *Journal) writeEvent(ev journal.RunEvent) error {
	j.mu.RLock()
	defer j.mu.RUnlock()
	if j.closed {
		return fmt.Errorf("journal: sqlite is closed")
	}
	if ev.At.IsZero() {
		ev.At = time.Now()
	}

	data, err := json.Marshal(ev.Data)
	if err != nil {
		return fmt.Errorf("journal: sqlite encode event: %w", err)
	}

	tx, err := j.db.Begin()
	if err != nil {
		return fmt.Errorf("journal: sqlite begin: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if _, err := tx.Exec(`
INSERT INTO journal_sequences (run_id, next_seq) VALUES (?, 1)
ON CONFLICT(run_id) DO UPDATE SET next_seq = next_seq + 1
`, ev.RunID); err != nil {
		return fmt.Errorf("journal: sqlite sequence: %w", err)
	}

	var seq int
	if err := tx.QueryRow(`SELECT next_seq - 1 FROM journal_sequences WHERE run_id = ?`, ev.RunID).Scan(&seq); err != nil {
		return fmt.Errorf("journal: sqlite read sequence: %w", err)
	}

	if _, err := tx.Exec(`
INSERT INTO journal_events (run_id, seq, at, depth, kind, data)
VALUES (?, ?, ?, ?, ?, ?)
`, ev.RunID, seq, ev.At.UTC().UnixNano(), ev.Depth, string(ev.Kind), string(data)); err != nil {
		return fmt.Errorf("journal: sqlite append: %w", err)
	}
	if err := j.updateRunMeta(tx, ev); err != nil {
		return err
	}

	if ev.Kind == journal.EvRunStart {
		if err := j.trim(tx); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("journal: sqlite commit: %w", err)
	}
	committed = true
	return nil
}

func (j *Journal) updateRunMeta(tx *sql.Tx, ev journal.RunEvent) error {
	if _, err := tx.Exec(`INSERT OR IGNORE INTO journal_runs (run_id) VALUES (?)`, ev.RunID); err != nil {
		return fmt.Errorf("journal: sqlite run metadata: %w", err)
	}

	const update = `
UPDATE journal_runs SET
    started_at = CASE WHEN ? = ? AND started_at = 0 THEN ? ELSE started_at END,
    ended_at = CASE WHEN ? = ? THEN ? ELSE ended_at END,
    status = CASE
        WHEN ? = ? THEN CASE WHEN status = '' THEN 'running' ELSE status END
        WHEN ? = ? THEN ?
        ELSE status
    END,
    source = CASE WHEN ? = ? AND source = '' THEN ? ELSE source END,
    session_id = CASE WHEN ? = ? AND session_id = '' THEN ? ELSE session_id END,
    agent = CASE WHEN ? = ? AND agent = '' THEN ? ELSE agent END,
    task = CASE WHEN ? = ? AND task = '' THEN ? ELSE task END,
    llm_calls = llm_calls + CASE WHEN ? = ? THEN 1 ELSE 0 END,
    tool_calls = tool_calls + CASE WHEN ? = ? THEN 1 ELSE 0 END,
    in_tokens = in_tokens + CASE WHEN ? = ? THEN ? ELSE 0 END,
    out_tokens = out_tokens + CASE WHEN ? = ? THEN ? ELSE 0 END,
    blocked = blocked + CASE WHEN ? = ? AND ? = 'deny' THEN 1 ELSE 0 END
                      + CASE WHEN ? = ? AND ? THEN 1 ELSE 0 END,
    masked = masked + CASE WHEN ? = ? AND ? = 'mask' THEN 1 ELSE 0 END,
    errors = errors + CASE WHEN ? = ? AND ? THEN 1 ELSE 0 END
WHERE run_id = ?`
	data := ev.Data
	source, _ := data["source"].(string)
	sessionID, _ := data["session_id"].(string)
	action, _ := data["action"].(string)
	isError, _ := data["is_error"].(bool)
	args := []any{
		string(ev.Kind), string(journal.EvRunStart), ev.At.UTC().UnixNano(),
		string(ev.Kind), string(journal.EvRunEnd), ev.At.UTC().UnixNano(),
		string(ev.Kind), string(journal.EvRunStart), string(ev.Kind), string(journal.EvRunEnd), dataString(data, "status"),
		string(ev.Kind), string(journal.EvRunStart), source,
		string(ev.Kind), string(journal.EvRunStart), sessionID,
		string(ev.Kind), string(journal.EvRunStart), dataString(data, "agent"),
		string(ev.Kind), string(journal.EvRunStart), dataString(data, "task"),
		string(ev.Kind), string(journal.EvLLMReponse),
		string(ev.Kind), string(journal.EvToolResult),
		string(ev.Kind), string(journal.EvLLMReponse), journal.IntFrom(data, "in_tokens"),
		string(ev.Kind), string(journal.EvLLMReponse), journal.IntFrom(data, "out_tokens"),
		string(ev.Kind), string(journal.EvGuardrail), action,
		string(ev.Kind), string(journal.EvPermission), journal.Denied(dataString(data, "decision")),
		string(ev.Kind), string(journal.EvGuardrail), action,
		string(ev.Kind), string(journal.EvToolResult), isError,
		ev.RunID,
	}
	if _, err := tx.Exec(update, args...); err != nil {
		return fmt.Errorf("journal: sqlite run metadata update: %w", err)
	}
	return nil
}

func dataString(data map[string]any, key string) string {
	value, _ := data[key].(string)
	return value
}

func (j *Journal) backfillRuns() error {
	var count int
	if err := j.db.QueryRow("SELECT COUNT(*) FROM journal_runs").Scan(&count); err != nil {
		return fmt.Errorf("journal: sqlite metadata count: %w", err)
	}
	if count > 0 {
		return nil
	}
	rows, err := j.db.Query(`SELECT run_id, seq, at, depth, kind, data FROM journal_events ORDER BY run_id, seq`)
	if err != nil {
		return fmt.Errorf("journal: sqlite metadata scan: %w", err)
	}
	var events []journal.RunEvent
	for rows.Next() {
		var ev journal.RunEvent
		var at int64
		var kind, data string
		if err := rows.Scan(&ev.RunID, &ev.Seq, &at, &ev.Depth, &kind, &data); err != nil {
			_ = rows.Close()
			return fmt.Errorf("journal: sqlite metadata scan: %w", err)
		}
		ev.At = time.Unix(0, at).UTC()
		ev.Kind = journal.EventKind(kind)
		if data != "" && data != "null" {
			if err := json.Unmarshal([]byte(data), &ev.Data); err != nil {
				_ = rows.Close()
				return fmt.Errorf("journal: sqlite metadata decode: %w", err)
			}
		}
		events = append(events, ev)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("journal: sqlite metadata rows: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("journal: sqlite metadata close: %w", err)
	}

	tx, err := j.db.Begin()
	if err != nil {
		return fmt.Errorf("journal: sqlite metadata begin: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	for _, ev := range events {
		if err := j.updateRunMeta(tx, ev); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("journal: sqlite metadata commit: %w", err)
	}
	committed = true
	return nil
}

func (j *Journal) readRun(runID string) (journal.RunRecord, error) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	if j.closed {
		return journal.RunRecord{}, fmt.Errorf("journal: sqlite is closed")
	}
	rows, err := j.db.Query(`
SELECT seq, at, depth, kind, data
FROM journal_events
WHERE run_id = ?
ORDER BY seq
`, runID)
	if err != nil {
		return journal.RunRecord{}, fmt.Errorf("journal: sqlite read %s: %w", runID, err)
	}
	defer rows.Close()

	rec := journal.RunRecord{ID: runID}
	for rows.Next() {
		var (
			seq   int
			at    int64
			depth int
			kind  string
			data  string
		)
		if err := rows.Scan(&seq, &at, &depth, &kind, &data); err != nil {
			return journal.RunRecord{}, fmt.Errorf("journal: sqlite scan %s: %w", runID, err)
		}

		var eventData map[string]any
		if data != "" && data != "null" {
			if err := json.Unmarshal([]byte(data), &eventData); err != nil {
				return journal.RunRecord{}, fmt.Errorf("journal: sqlite decode %s: %w", runID, err)
			}
		}
		rec.Apply(journal.RunEvent{
			RunID: runID,
			Seq:   seq,
			At:    time.Unix(0, at).UTC(),
			Depth: depth,
			Kind:  journal.EventKind(kind),
			Data:  eventData,
		})
	}
	if err := rows.Err(); err != nil {
		return journal.RunRecord{}, fmt.Errorf("journal: sqlite read %s: %w", runID, err)
	}
	if len(rec.Events) == 0 {
		return journal.RunRecord{}, fmt.Errorf("journal: %s: no readable events", runID)
	}

	return rec, nil
}

func (j *Journal) Get(runID string) (journal.RunRecord, error) {
	return j.readRun(runID)
}

func (j *Journal) List(f journal.ListFilter) ([]journal.RunMeta, error) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	if j.closed {
		return nil, fmt.Errorf("journal: sqlite is closed")
	}
	query := `
	SELECT run_id, session_id, source, agent, task, started_at, ended_at, status,
	       llm_calls, tool_calls, in_tokens, out_tokens, blocked, masked, errors
	FROM journal_runs
	WHERE 1 = 1`
	args := make([]any, 0, 5)
	if f.SessionID != "" {
		query += " AND session_id = ?"
		args = append(args, f.SessionID)
	}
	if f.Status != "" {
		query += " AND status = ?"
		args = append(args, f.Status)
	}
	if !f.Since.IsZero() {
		query += " AND started_at >= ?"
		args = append(args, f.Since.UnixNano())
	}
	if !f.Before.IsZero() {
		query += " AND started_at < ?"
		args = append(args, f.Before.UnixNano())
	}
	query += " ORDER BY started_at DESC, run_id DESC"
	if f.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, f.Limit)
	}

	rows, err := j.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("journal: sqlite list: %w", err)
	}

	out := make([]journal.RunMeta, 0)
	for rows.Next() {
		var (
			meta               journal.RunMeta
			startedAt, endedAt int64
		)
		if err := rows.Scan(&meta.ID, &meta.SessionID, &meta.Source, &meta.Agent, &meta.Task, &startedAt, &endedAt, &meta.Status,
			&meta.Summary.LLMCalls, &meta.Summary.ToolCalls, &meta.Summary.InTokens, &meta.Summary.OutTokens,
			&meta.Summary.Blocked, &meta.Summary.Masked, &meta.Summary.Errors); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("journal: sqlite list scan: %w", err)
		}
		if startedAt != 0 {
			meta.StartedAt = time.Unix(0, startedAt).UTC()
		}
		if endedAt != 0 {
			meta.EndedAt = time.Unix(0, endedAt).UTC()
		}
		out = append(out, meta)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("journal: sqlite list: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("journal: sqlite list close: %w", err)
	}

	return out, nil
}

func (j *Journal) prune() error {
	tx, err := j.db.Begin()
	if err != nil {
		return fmt.Errorf("journal: sqlite retention begin: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	if err := j.trim(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("journal: sqlite retention commit: %w", err)
	}
	committed = true
	return nil
}

func (j *Journal) trim(tx *sql.Tx) error {
	rows, err := tx.Query(`
	SELECT run_id
	FROM journal_runs
	ORDER BY started_at DESC, run_id DESC
	LIMIT -1 OFFSET ?
`, j.maxRuns)
	if err != nil {
		return fmt.Errorf("journal: sqlite retention query: %w", err)
	}

	var oldIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return fmt.Errorf("journal: sqlite retention scan: %w", err)
		}
		oldIDs = append(oldIDs, id)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("journal: sqlite retention query: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("journal: sqlite retention close: %w", err)
	}

	for _, id := range oldIDs {
		if _, err := tx.Exec("DELETE FROM journal_events WHERE run_id = ?", id); err != nil {
			return fmt.Errorf("journal: sqlite retention events: %w", err)
		}
		if _, err := tx.Exec("DELETE FROM journal_sequences WHERE run_id = ?", id); err != nil {
			return fmt.Errorf("journal: sqlite retention sequence: %w", err)
		}
		if _, err := tx.Exec("DELETE FROM journal_runs WHERE run_id = ?", id); err != nil {
			return fmt.Errorf("journal: sqlite retention metadata: %w", err)
		}
	}
	return nil
}
