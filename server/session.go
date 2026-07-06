package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"

	"github.com/Federicoand98/mani/app"
)

type sessionManager struct {
	spec app.RuntimeSpec
	mu   sync.Mutex
	m    map[string]*app.Runtime
}

func newSessionManager(spec app.RuntimeSpec) *sessionManager {
	return &sessionManager{spec: spec, mu: sync.Mutex{}, m: make(map[string]*app.Runtime)}
}

func (sm *sessionManager) create(ctx context.Context) (string, error) {
	rt, err := app.Build(ctx, sm.spec)
	if err != nil {
		return "", err
	}

	id := newID()
	sm.mu.Lock()
	sm.m[id] = rt
	sm.mu.Unlock()
	return id, nil
}

func (sm *sessionManager) get(id string) (*app.Runtime, bool) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	rt, ok := sm.m[id]
	return rt, ok
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
	rt, ok := sm.m[id]
	delete(sm.m, id)
	sm.mu.Unlock()
	if ok {
		rt.Close()
	}
	return ok
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
