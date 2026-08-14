// Package session holds a conversation and its persistence.
//
// A Session bundles a Memory, a Plan and metadata (id, title, timestamps, model).
// Store is the persistence port, with in-memory and on-disk adapters; serialization
// of messages lives entirely in the adapter, so core stays unaware of it.
package session

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"

	"github.com/Federicoand98/mani/core"
)

type StepStatus string

const (
	StepPending    StepStatus = "pending"
	StepInProgress StepStatus = "in_progress"
	StepDone       StepStatus = "done"
)

type PlanStep struct {
	Description string     `json:"description"`
	Status      StepStatus `json:"status"`
}

type Session struct {
	ID        string
	Title     string
	Model     string
	Plan      []PlanStep
	CreatedAt time.Time
	UpdatedAt time.Time
	memory    core.Memory
}

func New(model string) *Session {
	now := time.Now()

	return &Session{
		ID:        newID(),
		Model:     model,
		CreatedAt: now,
		UpdatedAt: now,
		memory:    core.NewInMemory(),
	}
}

func (s *Session) Memory() core.Memory { return s.memory }

func (s *Session) touch() {
	s.UpdatedAt = time.Now()
	if s.Title == "" {
		s.Title = deriveTitle(s.memory.Messages())
	}
}

func newID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func deriveTitle(msgs []core.Message) string {
	for _, m := range msgs {
		if m.Role != core.RoleUser {
			continue
		}

		for _, b := range m.Content {
			if t, ok := b.(core.TextBlock); ok {
				title := strings.TrimSpace(t.Text)
				if len(title) > 50 {
					title = title[:50] + "..."
				}
				return title
			}
		}
	}

	return "(no title)"
}
