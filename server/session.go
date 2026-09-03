package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"sync"
	"time"

	"github.com/Federicoand98/mani/app"
)

type session struct {
	rt       *app.Runtime
	lastUsed time.Time
	busy     int
}

type sessionManager struct {
	spec app.RuntimeSpec
	mu   sync.Mutex
	m    map[string]*session
}

// ponytail: costante di fatto, variabile solo per poterla accorciare nei test.
// Diventa un flag di `mani serve` se qualcuno ha bisogno di un'attesa diversa.
var sessionTTL = 30 * time.Minute

func newSessionManager(spec app.RuntimeSpec) *sessionManager {
	return &sessionManager{spec: spec, mu: sync.Mutex{}, m: make(map[string]*session)}
}

func (sm *sessionManager) create(ctx context.Context) (string, error) {
	rt, err := app.Build(ctx, sm.spec)
	if err != nil {
		return "", err
	}

	sm.sweep()

	id := newID()
	sm.mu.Lock()
	sm.m[id] = &session{rt: rt, lastUsed: time.Now(), busy: 0}
	sm.mu.Unlock()
	return id, nil
}

func (sm *sessionManager) acquire(id string) (*app.Runtime, bool) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s, ok := sm.m[id]
	if !ok {
		return nil, false
	}
	s.busy++
	s.lastUsed = time.Now()
	return s.rt, true
}

func (sm *sessionManager) release(id string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if s, ok := sm.m[id]; ok {
		s.busy--
		s.lastUsed = time.Now()
	}
}

func (sm *sessionManager) list() []string {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	ids := make([]string, 0, len(sm.m))
	for id := range sm.m {
		ids = append(ids, id)
	}
	return ids
}

func (sm *sessionManager) remove(id string) bool {
	sm.mu.Lock()
	s, ok := sm.m[id]
	delete(sm.m, id)
	sm.mu.Unlock()
	if ok {
		s.rt.Close()
	}
	return ok
}

func (sm *sessionManager) sweep() {
	cutoff := time.Now().Add(-sessionTTL)

	sm.mu.Lock()
	var dead []*session
	for id, s := range sm.m {
		if s.busy == 0 && s.lastUsed.Before(cutoff) {
			dead = append(dead, s)
			delete(sm.m, id)
			slog.Info("[server]: session evicted, idle too long", "session", id, "idle", time.Since(s.lastUsed))
		}
	}
	sm.mu.Unlock()

	for _, s := range dead {
		s.rt.Close()
	}
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
