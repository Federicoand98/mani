package app

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Federicoand98/mani/core"
)

// PORT

type Journal interface {
	Start(rec RunRecord) error
	Append(ev RunEvent) error
	Finish(runID, status string) error
	Get(runID string) (RunRecord, error)
	List(f ListFilter) ([]RunMeta, error)
}

type ListFilter struct {
	SessionID string
	Limit     int
	Status    string
	Since     time.Time
}

func (f ListFilter) matches(m RunMeta) bool {
	if f.SessionID != "" && m.SessionID != f.SessionID {
		return false
	}

	if f.Status != "" && m.Status != f.Status {
		return false
	}

	if !f.Since.IsZero() && m.StartedAt.Before(f.Since) {
		return false
	}

	return true
}

// Events
type EventKind string

const (
	EvRunStart   EventKind = "run_start"
	EvLLMCall    EventKind = "llm_call"
	EvLLMReponse EventKind = "llm_response"
	EvToolCall   EventKind = "tool_call"
	EvToolResult EventKind = "tool_result"
	EvGuardrail  EventKind = "guardrail" // Data["action"] = "deny" | "mask" (vedi recordGovernance)
	EvRunEnd     EventKind = "run_end"
)

type RunEvent struct {
	RunID string         `json:"run_id"`
	Seq   int            `json:"seq"`
	At    time.Time      `json:"at"`
	Depth int            `json:"depth"` // 0 = root, >= 0 child
	Kind  EventKind      `json:"kind"`
	Data  map[string]any `json:"data,omitempty"`
}

type Summary struct {
	LLMCalls  int `json:"llm_calls"`
	ToolCalls int `json:"tool_calls"`
	InTokens  int `json:"in_tokens"`
	OutTokens int `json:"out_tokens"`
	Blocked   int `json:"blocked"`
	Masked    int `json:"masked"`
	Errors    int `json:"errors"`
}

// RunRecord: header + logs
type RunRecord struct {
	ID        string     `json:"id"`
	SessionID string     `json:"session_id,omitempty"`
	Source    string     `json:"source"`
	StartedAt time.Time  `json:"started_at"`
	EndedAt   time.Time  `json:"ended_at,omitempty"`
	Status    string     `json:"status"`
	Summary   Summary    `json:"summary"`
	Events    []RunEvent `json:"events,omitempty"`
}

// RunMeta: header without Events: useful for List()
type RunMeta struct {
	ID        string    `json:"id"`
	SessionID string    `json:"session_id,omitempty"`
	Source    string    `json:"source"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at,omitempty"`
	Status    string    `json:"status"`
	Summary   Summary   `json:"summary"`
}

func (r *RunRecord) Meta() RunMeta {
	return RunMeta{
		ID: r.ID, SessionID: r.SessionID, Source: r.Source,
		StartedAt: r.StartedAt, EndedAt: r.EndedAt, Status: r.Status, Summary: r.Summary,
	}
}

// apply: adds an event to Events and updates the Summary/Header.
// The only source of truth for the meaning of an event.
func (r *RunRecord) apply(ev RunEvent) {
	r.Events = append(r.Events, ev)

	switch ev.Kind {
	case EvRunStart:
		if r.StartedAt.IsZero() {
			r.StartedAt = ev.At
		}
		if s, ok := ev.Data["source"].(string); ok && r.Source == "" {
			r.Source = s
		}
		if s, ok := ev.Data["session_id"].(string); ok && r.SessionID == "" {
			r.SessionID = s
		}
		if r.Status == "" {
			r.Status = "running"
		}

	case EvLLMReponse:
		r.Summary.LLMCalls++
		r.Summary.InTokens += intFrom(ev.Data, "in_tokens")
		r.Summary.OutTokens += intFrom(ev.Data, "out_tokens")

	case EvToolResult:
		r.Summary.ToolCalls++
		if b, _ := ev.Data["is_error"].(bool); b {
			r.Summary.Errors++
		}

	case EvGuardrail:
		switch ev.Data["action"].(string) {
		case "deny":
			r.Summary.Blocked++
		case "mask":
			r.Summary.Masked++
		}

	case EvRunEnd:
		r.EndedAt = ev.At
		if s, ok := ev.Data["status"].(string); ok {
			r.Status = s
		}
	}
}

func intFrom(m map[string]any, k string) int {
	switch v := m[k].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

func sortMetaNewestFirst(ms []RunMeta) {
	sort.Slice(ms, func(i, j int) bool {
		return ms[i].StartedAt.After(ms[j].StartedAt)
	})
}

// ======================================================
// ADAPTERS
// ======================================================

// == InMemoryJournal ==

type InMemoryJournal struct {
	mu    sync.Mutex
	runs  map[string]*RunRecord
	order []string
	max   int
}

func NewInMemoryJournal(max int) *InMemoryJournal {
	if max <= 0 {
		max = 100
	}
	return &InMemoryJournal{runs: make(map[string]*RunRecord), max: max}
}

func (j *InMemoryJournal) Start(rec RunRecord) error {
	j.mu.Lock()
	defer j.mu.Unlock()

	if _, exists := j.runs[rec.ID]; exists {
		return nil
	}

	if rec.Status == "" {
		rec.Status = "running"
	}

	cp := rec // copia
	j.runs[rec.ID] = &cp
	j.order = append(j.order, rec.ID)

	// eviction
	for len(j.order) > j.max {
		oldest := j.order[0]
		j.order = j.order[1:]
		delete(j.runs, oldest)
	}

	return nil
}

func (j *InMemoryJournal) Append(ev RunEvent) error {
	j.mu.Lock()
	defer j.mu.Unlock()

	rec, ok := j.runs[ev.RunID]
	if !ok {
		return nil
	}

	rec.apply(ev)
	return nil
}

func (j *InMemoryJournal) Finish(runID, status string) error {
	j.mu.Lock()
	defer j.mu.Unlock()

	rec, ok := j.runs[runID]
	if !ok {
		return nil
	}

	rec.EndedAt = time.Now()
	rec.Status = status
	return nil
}

func (j *InMemoryJournal) Get(runID string) (RunRecord, error) {
	j.mu.Lock()
	defer j.mu.Unlock()

	rec, ok := j.runs[runID]
	if !ok {
		return RunRecord{}, fmt.Errorf("run %q not found", runID)
	}

	cp := *rec
	cp.Events = append([]RunEvent(nil), rec.Events...)
	return cp, nil
}

func (j *InMemoryJournal) List(f ListFilter) ([]RunMeta, error) {
	j.mu.Lock()
	defer j.mu.Unlock()

	out := make([]RunMeta, 0, len(j.runs))
	for _, rec := range j.runs {
		m := rec.Meta()
		if !f.matches(m) {
			continue
		}
		out = append(out, rec.Meta())
	}

	sortMetaNewestFirst(out)

	if f.Limit > 0 && len(out) > f.Limit {
		out = out[:f.Limit]
	}

	return out, nil
}

// == JSONLJournal ==

type JSONLJournal struct {
	dir string
	mu  sync.Mutex
	seq map[string]int
}

func NewJSONLJournal(dir string) (*JSONLJournal, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("journal: mkdir %s: %w", dir, err)
	}

	return &JSONLJournal{dir: dir, seq: make(map[string]int)}, nil
}

func (j *JSONLJournal) path(runID string) string {
	return filepath.Join(j.dir, runID+".jsonl")
}

func (j *JSONLJournal) writeEvent(ev RunEvent) error {
	j.mu.Lock()
	ev.Seq = j.seq[ev.RunID]
	j.seq[ev.RunID]++
	j.mu.Unlock()

	if ev.At.IsZero() {
		ev.At = time.Now()
	}

	f, err := os.OpenFile(j.path(ev.RunID), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	line, err := json.Marshal(ev)
	if err != nil {
		return err
	}

	if _, err := f.Write(append(line, '\n')); err != nil {
		return err
	}

	return nil
}

func (j *JSONLJournal) Start(rec RunRecord) error {
	return j.writeEvent(RunEvent{
		RunID: rec.ID, At: rec.StartedAt, Kind: EvRunStart,
		Data: map[string]any{"source": rec.Source, "session_id": rec.SessionID},
	})
}

func (j *JSONLJournal) Append(ev RunEvent) error {
	return j.writeEvent(ev)
}

func (j *JSONLJournal) Finish(runID, status string) error {
	return j.writeEvent(RunEvent{
		RunID: runID, At: time.Now(), Kind: EvRunEnd,
		Data: map[string]any{"status": status},
	})
}

func (j *JSONLJournal) readRun(runID string) (RunRecord, error) {
	f, err := os.Open(j.path(runID))
	if err != nil {
		return RunRecord{}, fmt.Errorf("journal: open %s: %w", runID, err)
	}
	defer f.Close()

	rec := RunRecord{ID: runID}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var ev RunEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		rec.apply(ev)
	}
	if err := sc.Err(); err != nil {
		return RunRecord{}, err
	}

	// Zero eventi leggibili = file corrotto o troncato. Senza questo controllo
	// readRun restituirebbe un RunRecord col solo ID, e List lo mostrerebbe come
	// una run fantasma senza data ne' stato.
	if len(rec.Events) == 0 {
		return RunRecord{}, fmt.Errorf("journal: %s: no readable events", runID)
	}

	return rec, nil
}

func (j *JSONLJournal) Get(runID string) (RunRecord, error) {
	return j.readRun(runID)
}

func (j *JSONLJournal) List(f ListFilter) ([]RunMeta, error) {
	entries, err := os.ReadDir(j.dir)
	if err != nil {
		return nil, err
	}

	out := make([]RunMeta, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || filepath.Ext(name) != ".jsonl" {
			continue
		}

		if !f.Since.IsZero() {
			if info, err := e.Info(); err == nil && info.ModTime().Before(f.Since) {
				continue
			}
		}

		id := name[:len(name)-len(".jsonl")]
		rec, err := j.readRun(id)
		if err != nil {
			continue
		}

		m := rec.Meta()
		if !f.matches(m) {
			continue
		}

		out = append(out, rec.Meta())
	}

	sortMetaNewestFirst(out)
	if f.Limit > 0 && len(out) > f.Limit {
		out = out[:f.Limit]
	}

	return out, nil
}

// == MultiJournal ==

type MultiJournal struct {
	sinks []Journal
}

func NewMultiJournal(sinks ...Journal) *MultiJournal {
	return &MultiJournal{sinks: sinks}
}

func (m *MultiJournal) Start(rec RunRecord) error {
	for _, j := range m.sinks {
		_ = j.Start(rec)
	}
	return nil
}

func (m *MultiJournal) Append(ev RunEvent) error {
	for _, j := range m.sinks {
		_ = j.Append(ev)
	}
	return nil
}

func (m *MultiJournal) Finish(runID, status string) error {
	for _, j := range m.sinks {
		_ = j.Finish(runID, status)
	}
	return nil
}

func (m *MultiJournal) Get(runID string) (RunRecord, error) {
	return m.sinks[0].Get(runID)
}

func (m *MultiJournal) List(f ListFilter) ([]RunMeta, error) {
	return m.sinks[0].List(f)
}

func (m *MultiJournal) Close() error {
	var firstErr error
	for _, j := range m.sinks {
		if c, ok := j.(interface{ Close() error }); ok {
			if err := c.Close(); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// ======================================================
// HOOKS
// ======================================================

type sourceKey struct{}

func WithSource(ctx context.Context, src string) context.Context {
	return context.WithValue(ctx, sourceKey{}, src)
}

func sourceFrom(ctx context.Context) string {
	if src, ok := ctx.Value(sourceKey{}).(string); ok && src != "" {
		return src
	}
	return ""
}

// attachJournalHooks: attachs the hooks of the journal for a single agent.
func attachJournalHooks(h *core.HookManager, j Journal) {
	h.OnPreLLMCall(func(ctx context.Context, plp *core.PreLLMCallPayload) error {
		appendSafe(j, RunEvent{
			RunID: runIDFrom(ctx), Depth: depthFrom(ctx), At: time.Now(), Kind: EvLLMCall, Data: map[string]any{"messages": len(plp.Messages), "tools": len(plp.Tools)},
		})
		return nil
	})

	h.OnPostLLMCall(func(ctx context.Context, plp *core.PostLLMCallPayload) error {
		appendSafe(j, RunEvent{
			RunID: runIDFrom(ctx), Depth: depthFrom(ctx), At: time.Now(), Kind: EvLLMReponse,
			Data: map[string]any{
				"stop_reason": string(plp.Response.StopReason),
				"in_tokens":   plp.Response.Usage.InputTokens,
				"out_tokens":  plp.Response.Usage.OutputTokens,
			},
		})
		return nil
	})

	h.OnPreToolUse(func(ctx context.Context, ptup *core.PreToolUsePayload) error {
		appendSafe(j, RunEvent{
			RunID: runIDFrom(ctx), Depth: depthFrom(ctx), At: time.Now(), Kind: EvToolCall, Data: map[string]any{"tool": ptup.ToolName, "input": ptup.Input},
		})
		return nil
	})

	h.OnPostToolUse(func(ctx context.Context, ptup *core.PostToolUsePayload) error {
		appendSafe(j, RunEvent{
			RunID: runIDFrom(ctx), Depth: depthFrom(ctx), At: time.Now(), Kind: EvToolResult, Data: map[string]any{"tool": ptup.ToolName, "is_error": ptup.IsError, "result_len": len(ptup.Result)},
		})
		return nil
	})
}

// appendSafe: best effort append to the journal, logs warnings on failure.
func appendSafe(j Journal, ev RunEvent) {
	if j == nil {
		return
	}

	if err := j.Append(ev); err != nil {
		slog.Warn("journal append failed", "run", ev.RunID, "kind", ev.Kind, "err", err)
	}
}

func (r *Runtime) recordGovernance(ctx context.Context, action, tool, label string) {
	if r.journal == nil {
		return
	}
	data := map[string]any{"tool": tool, "action": action}
	if label != "" {
		data["label"] = label
	}
	appendSafe(r.journal, RunEvent{
		RunID: runIDFrom(ctx), Depth: depthFrom(ctx), At: time.Now(),
		Kind: EvGuardrail, Data: data,
	})
}

// RegisterJournal attaches the journal to the runtime and registers the journal hooks.
func RegisterJournal(rt *Runtime, j Journal) {
	rt.journal = j
	attachJournalHooks(rt.agent.Hooks(), j)
}

func journalRetention(retention int) int {
	if retention <= 0 {
		return 100
	}
	return retention
}

func newPersistentJournal(spec JournalSpec) (Journal, error) {
	switch backend := strings.ToLower(strings.TrimSpace(spec.Backend)); backend {
	case "", "jsonl":
		return NewJSONLJournal(spec.Path)
	case "sqlite":
		return NewSQLiteJournal(spec.Path, journalRetention(spec.Retention))
	default:
		return nil, fmt.Errorf("unsupported journal backend %q (jsonl|sqlite)", spec.Backend)
	}
}

func OpenJournalReader(spec RuntimeSpec) (Journal, bool) {
	js := spec.Observability.Journal
	if !js.Enabled || js.Path == "" {
		return nil, false
	}
	j, err := newPersistentJournal(js)
	if err != nil {
		return nil, false
	}
	return j, true
}
