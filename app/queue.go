package app

import (
	"context"
	"sync"
	"time"
)

type QTask struct {
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
	Enqueue(t QTask) error
	Claim(ctx context.Context) (QTask, error)
	Ack(t QTask) error
	Retry(r QTask, at time.Time) error
	Fail(t QTask, reason string) error
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
