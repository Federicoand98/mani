package app

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

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
    errors INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS journal_runs_started_at
    ON journal_runs (started_at DESC, run_id DESC);
`

// SQLiteJournal stores the same event stream as JSONLJournal in a single
// SQLite database. The modernc driver keeps the adapter cgo-free.
type SQLiteJournal struct {
	mu      sync.RWMutex
	db      *sql.DB
	maxRuns int
	closed  bool
}

var _ Journal = (*SQLiteJournal)(nil)

// NewSQLiteJournal opens a cgo-free SQLite journal. Retention defaults to 100
// runs when non-positive.
func NewSQLiteJournal(path string, retention int) (*SQLiteJournal, error) {
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

	maxRuns := journalRetention(retention)
	j := &SQLiteJournal{db: db, maxRuns: maxRuns}
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

func (j *SQLiteJournal) Close() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return nil
	}
	j.closed = true
	return j.db.Close()
}

func (j *SQLiteJournal) Start(rec RunRecord) error {
	return j.writeEvent(RunEvent{
		RunID: rec.ID, At: rec.StartedAt, Kind: EvRunStart,
		Data: map[string]any{"source": rec.Source, "session_id": rec.SessionID},
	})
}

func (j *SQLiteJournal) Append(ev RunEvent) error {
	return j.writeEvent(ev)
}

func (j *SQLiteJournal) Finish(runID, status string) error {
	return j.writeEvent(RunEvent{
		RunID: runID, At: time.Now(), Kind: EvRunEnd,
		Data: map[string]any{"status": status},
	})
}

func (j *SQLiteJournal) writeEvent(ev RunEvent) error {
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

	if ev.Kind == EvRunStart {
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

func (j *SQLiteJournal) updateRunMeta(tx *sql.Tx, ev RunEvent) error {
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
    llm_calls = llm_calls + CASE WHEN ? = ? THEN 1 ELSE 0 END,
    tool_calls = tool_calls + CASE WHEN ? = ? THEN 1 ELSE 0 END,
    in_tokens = in_tokens + CASE WHEN ? = ? THEN ? ELSE 0 END,
    out_tokens = out_tokens + CASE WHEN ? = ? THEN ? ELSE 0 END,
    blocked = blocked + CASE WHEN ? = ? AND ? = 'deny' THEN 1 ELSE 0 END,
    masked = masked + CASE WHEN ? = ? AND ? = 'mask' THEN 1 ELSE 0 END,
    errors = errors + CASE WHEN ? = ? AND ? THEN 1 ELSE 0 END
WHERE run_id = ?`
	data := ev.Data
	source, _ := data["source"].(string)
	sessionID, _ := data["session_id"].(string)
	action, _ := data["action"].(string)
	isError, _ := data["is_error"].(bool)
	args := []any{
		string(ev.Kind), string(EvRunStart), ev.At.UTC().UnixNano(),
		string(ev.Kind), string(EvRunEnd), ev.At.UTC().UnixNano(),
		string(ev.Kind), string(EvRunStart), string(ev.Kind), string(EvRunEnd), dataString(data, "status"),
		string(ev.Kind), string(EvRunStart), source,
		string(ev.Kind), string(EvRunStart), sessionID,
		string(ev.Kind), string(EvLLMReponse),
		string(ev.Kind), string(EvToolResult),
		string(ev.Kind), string(EvLLMReponse), intFrom(data, "in_tokens"),
		string(ev.Kind), string(EvLLMReponse), intFrom(data, "out_tokens"),
		string(ev.Kind), string(EvGuardrail), action,
		string(ev.Kind), string(EvGuardrail), action,
		string(ev.Kind), string(EvToolResult), isError,
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

func (j *SQLiteJournal) backfillRuns() error {
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
	var events []RunEvent
	for rows.Next() {
		var ev RunEvent
		var at int64
		var kind, data string
		if err := rows.Scan(&ev.RunID, &ev.Seq, &at, &ev.Depth, &kind, &data); err != nil {
			_ = rows.Close()
			return fmt.Errorf("journal: sqlite metadata scan: %w", err)
		}
		ev.At = time.Unix(0, at).UTC()
		ev.Kind = EventKind(kind)
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

func (j *SQLiteJournal) readRun(runID string) (RunRecord, error) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	if j.closed {
		return RunRecord{}, fmt.Errorf("journal: sqlite is closed")
	}
	rows, err := j.db.Query(`
SELECT seq, at, depth, kind, data
FROM journal_events
WHERE run_id = ?
ORDER BY seq
`, runID)
	if err != nil {
		return RunRecord{}, fmt.Errorf("journal: sqlite read %s: %w", runID, err)
	}
	defer rows.Close()

	rec := RunRecord{ID: runID}
	for rows.Next() {
		var (
			seq   int
			at    int64
			depth int
			kind  string
			data  string
		)
		if err := rows.Scan(&seq, &at, &depth, &kind, &data); err != nil {
			return RunRecord{}, fmt.Errorf("journal: sqlite scan %s: %w", runID, err)
		}

		var eventData map[string]any
		if data != "" && data != "null" {
			if err := json.Unmarshal([]byte(data), &eventData); err != nil {
				return RunRecord{}, fmt.Errorf("journal: sqlite decode %s: %w", runID, err)
			}
		}
		rec.apply(RunEvent{
			RunID: runID,
			Seq:   seq,
			At:    time.Unix(0, at).UTC(),
			Depth: depth,
			Kind:  EventKind(kind),
			Data:  eventData,
		})
	}
	if err := rows.Err(); err != nil {
		return RunRecord{}, fmt.Errorf("journal: sqlite read %s: %w", runID, err)
	}
	if len(rec.Events) == 0 {
		return RunRecord{}, fmt.Errorf("journal: %s: no readable events", runID)
	}

	return rec, nil
}

func (j *SQLiteJournal) Get(runID string) (RunRecord, error) {
	return j.readRun(runID)
}

func (j *SQLiteJournal) List(f ListFilter) ([]RunMeta, error) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	if j.closed {
		return nil, fmt.Errorf("journal: sqlite is closed")
	}
	query := `
	SELECT run_id, session_id, source, started_at, ended_at, status,
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
	query += " ORDER BY started_at DESC, run_id DESC"
	if f.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, f.Limit)
	}

	rows, err := j.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("journal: sqlite list: %w", err)
	}

	out := make([]RunMeta, 0)
	for rows.Next() {
		var (
			meta               RunMeta
			startedAt, endedAt int64
		)
		if err := rows.Scan(&meta.ID, &meta.SessionID, &meta.Source, &startedAt, &endedAt, &meta.Status,
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

func (j *SQLiteJournal) prune() error {
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

func (j *SQLiteJournal) trim(tx *sql.Tx) error {
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
