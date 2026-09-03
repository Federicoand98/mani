package server

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Federicoand98/mani/app"
	"github.com/Federicoand98/mani/config"
)

// shortTTL shrinks the idle window for the duration of one test.
func shortTTL(t *testing.T, d time.Duration) {
	t.Helper()
	old := sessionTTL
	sessionTTL = d
	t.Cleanup(func() { sessionTTL = old })
}

// seed inserts a session directly, bypassing app.Build: these tests are about
// bookkeeping, not about wiring a runtime.
//
// The runtime is real but inert — sweep and remove call Close on it — and it is
// built from a config pointing nowhere, so nothing here touches the network.
func seed(sm *sessionManager, id string, lastUsed time.Time, busy int) {
	rt := app.NewFromConfig(config.Config{
		Provider:  "ollama",
		Providers: map[string]config.ProviderConfig{"ollama": {BaseURL: "http://127.0.0.1:1", Model: "m"}},
	})
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.m[id] = &session{rt: rt, lastUsed: lastUsed, busy: busy}
}

func has(sm *sessionManager, id string) bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	_, ok := sm.m[id]
	return ok
}

func newTestManager() *sessionManager {
	return newSessionManager(app.DefaultSpec())
}

// ---------------------------------------------------------------------------
// sweep
// ---------------------------------------------------------------------------

func TestSweep_EvictsIdleSessions(t *testing.T) {
	shortTTL(t, time.Minute)
	sm := newTestManager()

	seed(sm, "old", time.Now().Add(-2*time.Minute), 0)
	seed(sm, "fresh", time.Now(), 0)

	sm.sweep()

	if has(sm, "old") {
		t.Error("an idle session survived the sweep")
	}
	if !has(sm, "fresh") {
		t.Error("a recently used session was evicted")
	}
}

// The guard that makes the lazy sweep safe: a turn in flight holds the runtime,
// and closing it would pull MCP sessions and the journal out from under it.
func TestSweep_SkipsBusySessions(t *testing.T) {
	shortTTL(t, time.Minute)
	sm := newTestManager()

	seed(sm, "working", time.Now().Add(-2*time.Minute), 1)

	sm.sweep()

	if !has(sm, "working") {
		t.Fatal("a busy session was evicted mid-turn")
	}

	// Once the turn ends the session is sweepable again — but release also
	// restarts the clock, so it survives until the TTL elapses from now.
	sm.release("working")
	sm.sweep()
	if !has(sm, "working") {
		t.Error("release must restart the idle clock, not expose the session immediately")
	}
}

func TestSweep_NothingToDo(t *testing.T) {
	sm := newTestManager()
	sm.sweep() // must not panic on an empty manager

	seed(sm, "fresh", time.Now(), 0)
	sm.sweep()
	if !has(sm, "fresh") {
		t.Error("a fresh session was evicted with the default TTL")
	}
}

// ---------------------------------------------------------------------------
// acquire / release
// ---------------------------------------------------------------------------

func TestAcquire_MarksBusyAndTouches(t *testing.T) {
	sm := newTestManager()
	stale := time.Now().Add(-time.Hour)
	seed(sm, "s1", stale, 0)

	if _, ok := sm.acquire("s1"); !ok {
		t.Fatal("acquire on an existing session returned false")
	}

	sm.mu.Lock()
	s := sm.m["s1"]
	busy, lastUsed := s.busy, s.lastUsed
	sm.mu.Unlock()

	if busy != 1 {
		t.Errorf("busy = %d, want 1", busy)
	}
	if !lastUsed.After(stale) {
		t.Error("acquire did not refresh lastUsed")
	}
}

// Acquiring rescues a session that was about to be swept: a client asking for it
// is exactly the evidence that it is still in use.
func TestAcquire_SavesSessionFromSweep(t *testing.T) {
	shortTTL(t, time.Minute)
	sm := newTestManager()
	seed(sm, "s1", time.Now().Add(-2*time.Minute), 0)

	if _, ok := sm.acquire("s1"); !ok {
		t.Fatal("acquire returned false")
	}
	sm.release("s1")

	sm.sweep()

	if !has(sm, "s1") {
		t.Error("a session used a moment ago was evicted")
	}
}

func TestAcquire_UnknownSession(t *testing.T) {
	sm := newTestManager()
	if rt, ok := sm.acquire("nope"); ok || rt != nil {
		t.Errorf("acquire(unknown) = (%v, %v), want (nil, false)", rt, ok)
	}
}

// A handler releases in a defer, so a session removed mid-turn still gets a
// release call: it must not panic.
func TestRelease_UnknownSession(t *testing.T) {
	sm := newTestManager()
	sm.release("nope")
}

func TestAcquire_NestedCallsBalance(t *testing.T) {
	shortTTL(t, time.Minute)
	sm := newTestManager()
	seed(sm, "s1", time.Now().Add(-2*time.Minute), 0)

	sm.acquire("s1")
	sm.acquire("s1")
	sm.release("s1")

	// One holder left: still not sweepable.
	sm.mu.Lock()
	sm.m["s1"].lastUsed = time.Now().Add(-2 * time.Minute)
	sm.mu.Unlock()
	sm.sweep()
	if !has(sm, "s1") {
		t.Fatal("a session with one holder left was evicted")
	}

	sm.release("s1")
	sm.mu.Lock()
	sm.m["s1"].lastUsed = time.Now().Add(-2 * time.Minute)
	sm.mu.Unlock()
	sm.sweep()
	if has(sm, "s1") {
		t.Error("with no holders left the session should have been evicted")
	}
}

// The busy counter is written from every connection goroutine: run it under
// `go test -race`, which is where a missing lock actually shows up.
func TestAcquireRelease_Concurrent(t *testing.T) {
	sm := newTestManager()
	seed(sm, "s1", time.Now(), 0)

	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			if _, ok := sm.acquire("s1"); ok {
				sm.release("s1")
			}
		})
	}
	wg.Wait()

	sm.mu.Lock()
	busy := sm.m["s1"].busy
	sm.mu.Unlock()
	if busy != 0 {
		t.Errorf("busy = %d after balanced acquire/release, want 0", busy)
	}
}

// ---------------------------------------------------------------------------
// create / remove, against a real runtime
// ---------------------------------------------------------------------------

// create is the only caller of sweep: the leak grows when new sessions arrive,
// so that is when the dead ones are collected.
func TestCreate_SweepsOnTheWayIn(t *testing.T) {
	shortTTL(t, time.Minute)
	s := testServer(t, "", "hi")
	sm := s.mgr

	seed(sm, "old", time.Now().Add(-2*time.Minute), 0)

	id, err := sm.create(context.Background())
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if has(sm, "old") {
		t.Error("create did not sweep the idle session")
	}
	if !has(sm, id) {
		t.Error("the new session is missing")
	}
}

func TestRemove(t *testing.T) {
	s := testServer(t, "", "hi")
	sm := s.mgr

	id, err := sm.create(context.Background())
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if !sm.remove(id) {
		t.Fatal("remove returned false for an existing session")
	}
	if has(sm, id) {
		t.Error("the session is still in the map")
	}
	if sm.remove(id) {
		t.Error("removing twice must report false the second time")
	}
}

func TestList(t *testing.T) {
	sm := newTestManager()
	seed(sm, "a", time.Now(), 0)
	seed(sm, "b", time.Now(), 0)

	if got := sm.list(); len(got) != 2 {
		t.Errorf("list() = %v, want 2 ids", got)
	}
}
