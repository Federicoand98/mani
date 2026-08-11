package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Task struct {
	ID        string    `json:"id"`
	Source    string    `json:"source"`  // cron | daily | webhook
	Trigger   string    `json:"trigger"` // ID del trigger
	Prompt    string    `json:"prompt"`
	Memory    string    `json:"memory"` // "" / "fresh" | "persistent"
	Attempt   int       `json:"attempt"`
	NextAt    time.Time `json:"next_at"`
	CreatedAt time.Time `json:"created_at"`
	LastError string    `json:"last_error,omitempty"`
}

type TaskQueue interface {
	Enqueue(t Task) error
	Claim(ctx context.Context) (Task, error)
	Ack(t Task) error
	Retry(r Task, at time.Time) error
	Fail(t Task, reason string) error
	Recover() (int, error)
}

const (
	statePending = "pending"
	stateRunning = "running"
	stateFailed  = "failed"
	stateDone    = "done"
)

// -------------------------------
// FILE QUEUE
// -------------------------------

type FileQueue struct {
	dir        string
	maxPending int
	mu         sync.Mutex
	poll       time.Duration
}

func NewFileQueue(dir string, maxPending int) (*FileQueue, error) {
	if maxPending <= 0 {
		maxPending = 64
	}

	for _, s := range []string{statePending, stateRunning, stateDone, stateFailed} {
		if err := os.MkdirAll(filepath.Join(dir, s), 0o755); err != nil {
			return nil, fmt.Errorf("[queue]: mkdir %s: %w", s, err)
		}
	}

	return &FileQueue{
		dir:        dir,
		maxPending: maxPending,
		poll:       500 * time.Millisecond,
	}, nil
}

// filename: <millis>-<id>.json -> ordine alfabetico è ordine FIFO
func filename(t Task) string {
	at := t.NextAt
	if at.IsZero() {
		at = t.CreatedAt
	}
	return fmt.Sprintf("%013d-%s.json", at.UnixMilli(), t.ID)
}

func (q *FileQueue) pathFor(state string, t Task) string {
	return filepath.Join(q.dir, state, filename(t))
}

func (q *FileQueue) writeTo(state string, t Task) error {
	b, err := json.MarshalIndent(t, "", "	")
	if err != nil {
		return err
	}

	tmp := q.pathFor(state, t) + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}

	return os.Rename(tmp, q.pathFor(state, t))
}

func (q *FileQueue) Enqueue(t Task) error {
	entries, err := os.ReadDir(filepath.Join(q.dir, statePending))
	if err != nil {
		return err
	}

	if len(entries) >= q.maxPending {
		return fmt.Errorf("[queue]: max pending tasks reached")
	}

	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now()
	}

	return q.writeTo(statePending, t)
}

// Claim: find the next pending task and it move it to the running state
func (q *FileQueue) Claim(ctx context.Context) (Task, error) {
	ticker := time.NewTicker(q.poll)
	defer ticker.Stop()

	for {
		// Il controllo va PRIMA di tryClaim: altrimenti, con il ctx cancellato e la coda
		// piena, continueremmo a consegnare task da eseguire durante lo shutdown.
		if err := ctx.Err(); err != nil {
			return Task{}, err
		}

		if t, ok, err := q.tryClaim(); err != nil {
			return Task{}, err
		} else if ok {
			return t, nil
		}

		select {
		case <-ctx.Done():
			return Task{}, ctx.Err()
		case <-ticker.C:
		}
	}
}

func (q *FileQueue) tryClaim() (Task, bool, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	entries, err := os.ReadDir(filepath.Join(q.dir, statePending))
	if err != nil {
		return Task{}, false, err
	}

	now := time.Now().UnixMilli()
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}

		if ms, err := strconv.ParseInt(strings.SplitN(name, "-", 2)[0], 10, 64); err != nil || ms > now {
			break
		}

		src := filepath.Join(q.dir, statePending, name)
		dst := filepath.Join(q.dir, stateRunning, name)
		if err := os.Rename(src, dst); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return Task{}, false, err
		}

		b, err := os.ReadFile(dst)
		if err != nil {
			return Task{}, false, err
		}

		var t Task
		if err := json.Unmarshal(b, &t); err != nil {
			_ = os.Rename(dst, filepath.Join(q.dir, stateFailed, name))
			continue
		}
		return t, true, nil
	}
	return Task{}, false, nil
}

func (q *FileQueue) move(from, to string, t Task) error {
	src := filepath.Join(q.dir, from, filename(t))
	if err := q.writeTo(to, t); err != nil {
		return err
	}
	if err := os.Remove(src); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// Ack: task completato con successo → done/. Il nome file non cambia (NextAt invariato).
func (q *FileQueue) Ack(t Task) error { return q.move(stateRunning, stateDone, t) }

func (q *FileQueue) Retry(t Task, at time.Time) error {
	old := filepath.Join(q.dir, stateRunning, filename(t))
	t.NextAt = at
	if err := q.writeTo(statePending, t); err != nil {
		return err
	}
	if err := os.Remove(old); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (q *FileQueue) Fail(t Task, reason string) error {
	old := filepath.Join(q.dir, stateRunning, filename(t))
	t.LastError = reason
	if err := q.writeTo(stateFailed, t); err != nil {
		return err
	}
	if err := os.Remove(old); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (q *FileQueue) Recover() (int, error) {
	entries, err := os.ReadDir(filepath.Join(q.dir, stateRunning))
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		src := filepath.Join(q.dir, stateRunning, name)
		if err := os.Rename(src, filepath.Join(q.dir, statePending, name)); err == nil {
			n++
		}
	}
	return n, nil
}

// -------------------------------
// In Memory QUEUE
// -------------------------------

type InMemoryQueue struct {
	mu      sync.Mutex
	pending []Task
	max     int
	notify  chan struct{}
}

func NewInMemoryQueue(max int) *InMemoryQueue {
	if max <= 0 {
		max = 64
	}
	return &InMemoryQueue{
		max:    max,
		notify: make(chan struct{}),
	}
}

func (q *InMemoryQueue) Enqueue(t Task) error {
	q.mu.Lock()
	if len(q.pending) >= q.max {
		q.mu.Unlock()
		return fmt.Errorf("[queue]: queue is full (%d/%d)", len(q.pending), q.max)
	}

	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now()
	}

	q.pending = append(q.pending, t)
	q.mu.Unlock()

	select {
	case q.notify <- struct{}{}:
	default:
	}

	return nil
}

func (q *InMemoryQueue) Claim(ctx context.Context) (Task, error) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		// come in FileQueue: niente consegne a shutdown iniziato
		if err := ctx.Err(); err != nil {
			return Task{}, err
		}

		q.mu.Lock()
		now := time.Now()
		for i, t := range q.pending {
			if t.NextAt.IsZero() || !t.NextAt.After(now) {
				// rimuove l'elemento i (era [:1]: il task reclamato restava in coda)
				q.pending = append(q.pending[:i], q.pending[i+1:]...)
				q.mu.Unlock()
				return t, nil
			}
		}
		q.mu.Unlock()

		select {
		case <-ctx.Done():
			return Task{}, ctx.Err()
		case <-q.notify:
		case <-ticker.C:
		}
	}
}

func (q *InMemoryQueue) Ack(Task) error                   { return nil }
func (q *InMemoryQueue) Retry(t Task, at time.Time) error { t.NextAt = at; return q.Enqueue(t) }
func (q *InMemoryQueue) Recover() (int, error)            { return 0, nil }
func (q *InMemoryQueue) Fail(t Task, reason string) error {
	slog.Warn("[queue]: task failed permanently", "id", t.ID, "trigger", t.Trigger, "err", reason)
	return nil
}
