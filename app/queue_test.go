package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// helper
// ---------------------------------------------------------------------------

func newTestFileQueue(t *testing.T, maxPending int) (*FileQueue, string) {
	t.Helper()
	dir := t.TempDir()
	q, err := NewFileQueue(dir, maxPending)
	if err != nil {
		t.Fatalf("NewFileQueue: %v", err)
	}
	q.poll = 10 * time.Millisecond // i test non devono aspettare il polling reale
	return q, dir
}

// countState conta i task in uno stato (ignora i .tmp delle scritture atomiche).
func countState(t *testing.T, dir, state string) int {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(dir, state))
	if err != nil {
		t.Fatalf("ReadDir %s: %v", state, err)
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
			n++
		}
	}
	return n
}

// claimNow reclama con un timeout corto: fallisce il test se non arriva nulla.
func claimNow(t *testing.T, q TaskQueue) Task {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	task, err := q.Claim(ctx)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	return task
}

// mustNotClaim verifica che entro `d` non venga consegnato nulla.
func mustNotClaim(t *testing.T, q TaskQueue, d time.Duration, why string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	if task, err := q.Claim(ctx); err == nil {
		t.Fatalf("%s: consegnato invece il task %q", why, task.ID)
	}
}

// ---------------------------------------------------------------------------
// FileQueue — ciclo di vita
// ---------------------------------------------------------------------------

// Il ciclo nominale: pending -> running -> done, con lo stato che È la directory.
func TestFileQueue_Lifecycle(t *testing.T) {
	q, dir := newTestFileQueue(t, 10)

	if err := q.Enqueue(Task{ID: "a1", Trigger: "tr", Prompt: "ciao"}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if got := countState(t, dir, statePending); got != 1 {
		t.Fatalf("dopo Enqueue pending=%d, atteso 1", got)
	}

	task := claimNow(t, q)
	if task.ID != "a1" || task.Prompt != "ciao" || task.Trigger != "tr" {
		t.Errorf("task non deserializzato correttamente: %+v", task)
	}
	if got := countState(t, dir, statePending); got != 0 {
		t.Errorf("dopo Claim pending=%d, atteso 0", got)
	}
	if got := countState(t, dir, stateRunning); got != 1 {
		t.Errorf("dopo Claim running=%d, atteso 1", got)
	}

	if err := q.Ack(task); err != nil {
		t.Fatalf("Ack: %v", err)
	}
	if got := countState(t, dir, stateRunning); got != 0 {
		t.Errorf("dopo Ack running=%d, atteso 0", got)
	}
	if got := countState(t, dir, stateDone); got != 1 {
		t.Errorf("dopo Ack done=%d, atteso 1", got)
	}
}

// Enqueue persiste tutti i campi che servono a rieseguire il task dopo un riavvio.
func TestFileQueue_PersistsTaskFields(t *testing.T) {
	q, _ := newTestFileQueue(t, 10)

	want := Task{ID: "p1", Source: "daily", Trigger: "nightly", Prompt: "report", Memory: "persistent", Attempt: 2}
	if err := q.Enqueue(want); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	got := claimNow(t, q)
	if got.Source != want.Source || got.Trigger != want.Trigger ||
		got.Memory != want.Memory || got.Attempt != want.Attempt || got.Prompt != want.Prompt {
		t.Errorf("campi non persistiti:\n got  %+v\n want %+v", got, want)
	}
	if got.CreatedAt.IsZero() {
		t.Error("CreatedAt deve essere valorizzato da Enqueue")
	}
}

// ---------------------------------------------------------------------------
// FileQueue — ordinamento e backoff
// ---------------------------------------------------------------------------

// Il nome file inizia con l'istante di eleggibilità: l'ordine alfabetico È il FIFO.
func TestFileQueue_FIFOByEligibility(t *testing.T) {
	q, _ := newTestFileQueue(t, 10)
	now := time.Now()

	// accodati in ordine sparso, devono uscire in ordine cronologico
	for _, tk := range []Task{
		{ID: "terzo", NextAt: now.Add(-1 * time.Second)},
		{ID: "primo", NextAt: now.Add(-3 * time.Second)},
		{ID: "secondo", NextAt: now.Add(-2 * time.Second)},
	} {
		if err := q.Enqueue(tk); err != nil {
			t.Fatalf("Enqueue %s: %v", tk.ID, err)
		}
	}

	want := []string{"primo", "secondo", "terzo"}
	for i, exp := range want {
		if got := claimNow(t, q); got.ID != exp {
			t.Fatalf("posizione %d: atteso %q, ottenuto %q", i, exp, got.ID)
		}
	}
}

// Un task con NextAt nel futuro non è eleggibile (è il meccanismo del backoff).
func TestFileQueue_FutureTaskNotClaimed(t *testing.T) {
	q, _ := newTestFileQueue(t, 10)

	if err := q.Enqueue(Task{ID: "futuro", NextAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	mustNotClaim(t, q, 200*time.Millisecond, "un task con NextAt nel futuro non deve essere reclamato")
}

// ---------------------------------------------------------------------------
// FileQueue — retry, dead letter, recovery
// ---------------------------------------------------------------------------

// Regressione della trappola del nome-file: il nome dipende da NextAt, quindi
// cambiare il campo prima di rimuovere il vecchio file lascerebbe un duplicato.
func TestFileQueue_RetryLeavesNoDuplicate(t *testing.T) {
	q, dir := newTestFileQueue(t, 10)

	if err := q.Enqueue(Task{ID: "r1"}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	task := claimNow(t, q)

	task.Attempt++
	if err := q.Retry(task, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("Retry: %v", err)
	}

	if got := countState(t, dir, stateRunning); got != 0 {
		t.Errorf("dopo Retry running=%d, atteso 0 (file duplicato rimasto in running/)", got)
	}
	if got := countState(t, dir, statePending); got != 1 {
		t.Errorf("dopo Retry pending=%d, atteso 1", got)
	}
}

// Il conteggio dei tentativi sopravvive al giro in coda: è ciò che rende il retry
// resistente a un riavvio del processo.
func TestFileQueue_RetryPersistsAttempt(t *testing.T) {
	q, _ := newTestFileQueue(t, 10)

	if err := q.Enqueue(Task{ID: "r2"}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	task := claimNow(t, q)
	task.Attempt = 2
	if err := q.Retry(task, time.Now().Add(-time.Second)); err != nil { // già eleggibile
		t.Fatalf("Retry: %v", err)
	}

	if got := claimNow(t, q); got.Attempt != 2 {
		t.Errorf("Attempt = %d, atteso 2", got.Attempt)
	}
}

// Tentativi esauriti: il task finisce in failed/ con il motivo, per l'ispezione a mano.
func TestFileQueue_FailWritesDeadLetter(t *testing.T) {
	q, dir := newTestFileQueue(t, 10)

	if err := q.Enqueue(Task{ID: "f1"}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	task := claimNow(t, q)
	if err := q.Fail(task, "boom"); err != nil {
		t.Fatalf("Fail: %v", err)
	}

	if got := countState(t, dir, stateRunning); got != 0 {
		t.Errorf("dopo Fail running=%d, atteso 0", got)
	}
	if got := countState(t, dir, stateFailed); got != 1 {
		t.Fatalf("dopo Fail failed=%d, atteso 1", got)
	}

	entries, _ := os.ReadDir(filepath.Join(dir, stateFailed))
	raw, err := os.ReadFile(filepath.Join(dir, stateFailed, entries[0].Name()))
	if err != nil {
		t.Fatalf("lettura dead letter: %v", err)
	}
	var got Task
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("dead letter non deserializzabile: %v", err)
	}
	if got.LastError != "boom" {
		t.Errorf("LastError = %q, atteso 'boom'", got.LastError)
	}
}

// Dopo un crash i task restano in running/: al riavvio devono tornare eleggibili.
func TestFileQueue_RecoverOrphanedRunning(t *testing.T) {
	q, dir := newTestFileQueue(t, 10)

	if err := q.Enqueue(Task{ID: "orfano"}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	claimNow(t, q) // reclamato e mai ack-ato: simula il crash

	n, err := q.Recover()
	if err != nil {
		t.Fatalf("Recover: %v", err)
	}
	if n != 1 {
		t.Errorf("Recover = %d, atteso 1", n)
	}
	if got := countState(t, dir, stateRunning); got != 0 {
		t.Errorf("dopo Recover running=%d, atteso 0", got)
	}
	if got := claimNow(t, q); got.ID != "orfano" {
		t.Errorf("il task recuperato deve essere richiamabile, ottenuto %q", got.ID)
	}
}

// Recover non tocca done/ e failed/: sono stati terminali.
func TestFileQueue_RecoverIgnoresTerminalStates(t *testing.T) {
	q, dir := newTestFileQueue(t, 10)

	q.Enqueue(Task{ID: "ok"})
	q.Ack(claimNow(t, q))
	q.Enqueue(Task{ID: "ko"})
	q.Fail(claimNow(t, q), "x")

	n, err := q.Recover()
	if err != nil {
		t.Fatalf("Recover: %v", err)
	}
	if n != 0 {
		t.Errorf("Recover = %d, atteso 0 (nessun orfano)", n)
	}
	if got := countState(t, dir, stateDone); got != 1 {
		t.Errorf("done=%d, atteso 1", got)
	}
	if got := countState(t, dir, stateFailed); got != 1 {
		t.Errorf("failed=%d, atteso 1", got)
	}
}

// ---------------------------------------------------------------------------
// FileQueue — backpressure, robustezza, concorrenza
// ---------------------------------------------------------------------------

// Oltre max_pending, Enqueue rifiuta: è la contropressione che sostituisce
// il vecchio "scarta silenziosamente".
func TestFileQueue_EnqueueRespectsMaxPending(t *testing.T) {
	q, _ := newTestFileQueue(t, 2)

	if err := q.Enqueue(Task{ID: "1"}); err != nil {
		t.Fatalf("primo Enqueue: %v", err)
	}
	if err := q.Enqueue(Task{ID: "2"}); err != nil {
		t.Fatalf("secondo Enqueue: %v", err)
	}
	if err := q.Enqueue(Task{ID: "3"}); err == nil {
		t.Error("il terzo Enqueue deve fallire: coda piena")
	}
}

// Un file corrotto non deve bloccare la coda: va isolato in failed/ e si prosegue.
func TestFileQueue_CorruptedFileDoesNotBlock(t *testing.T) {
	q, dir := newTestFileQueue(t, 10)

	bad := filepath.Join(dir, statePending, "0000000000001-corrotto.json")
	if err := os.WriteFile(bad, []byte("{non json"), 0o644); err != nil {
		t.Fatalf("scrittura file corrotto: %v", err)
	}
	if err := q.Enqueue(Task{ID: "buono"}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	if got := claimNow(t, q); got.ID != "buono" {
		t.Errorf("atteso il task valido, ottenuto %q", got.ID)
	}
	if got := countState(t, dir, stateFailed); got != 1 {
		t.Errorf("il file corrotto deve finire in failed/, failed=%d", got)
	}
}

// Con N worker su M task, ogni task va consegnato ESATTAMENTE una volta:
// il rename atomico è il meccanismo di mutua esclusione.
func TestFileQueue_ConcurrentClaimDeliversOnce(t *testing.T) {
	q, dir := newTestFileQueue(t, 100)

	const tasks, workers = 20, 4
	for i := range tasks {
		if err := q.Enqueue(Task{ID: string(rune('A' + i))}); err != nil {
			t.Fatalf("Enqueue %d: %v", i, err)
		}
	}

	var (
		mu   sync.Mutex
		seen = map[string]int{}
		wg   sync.WaitGroup
	)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				task, err := q.Claim(ctx)
				if err != nil {
					return
				}
				_ = q.Ack(task)

				mu.Lock()
				seen[task.ID]++
				done := len(seen)
				mu.Unlock()

				if done == tasks {
					cancel()
					return
				}
			}
		}()
	}
	wg.Wait()

	if len(seen) != tasks {
		t.Errorf("task distinti consegnati = %d, attesi %d", len(seen), tasks)
	}
	for id, n := range seen {
		if n != 1 {
			t.Errorf("task %q consegnato %d volte, atteso 1", id, n)
		}
	}
	if got := countState(t, dir, stateDone); got != tasks {
		t.Errorf("done=%d, atteso %d", got, tasks)
	}
}

// Regressione: a shutdown iniziato la coda non deve consegnare altro lavoro,
// anche se ci sono task eleggibili (altrimenti si esegue con ctx cancellato).
func TestFileQueue_CancelledCtxStopsDelivery(t *testing.T) {
	q, dir := newTestFileQueue(t, 10)

	if err := q.Enqueue(Task{ID: "x"}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := q.Claim(ctx); err == nil {
		t.Fatal("con ctx cancellato Claim non deve consegnare task")
	}
	if got := countState(t, dir, statePending); got != 1 {
		t.Errorf("il task deve restare in pending/, pending=%d", got)
	}
}

// ---------------------------------------------------------------------------
// InMemoryQueue — l'adapter di default quando queue.path è assente
// ---------------------------------------------------------------------------

// Regressione dello slicing errato: il task reclamato deve USCIRE dalla coda,
// altrimenti viene riconsegnato all'infinito.
func TestInMemoryQueue_ClaimRemovesTask(t *testing.T) {
	q := NewInMemoryQueue(10)
	for _, id := range []string{"A", "B", "C"} {
		if err := q.Enqueue(Task{ID: id}); err != nil {
			t.Fatalf("Enqueue %s: %v", id, err)
		}
	}

	for i, want := range []string{"A", "B", "C"} {
		if got := claimNow(t, q); got.ID != want {
			t.Fatalf("posizione %d: atteso %q, ottenuto %q", i, want, got.ID)
		}
	}
	mustNotClaim(t, q, 300*time.Millisecond, "coda vuota: non deve riconsegnare")
}

func TestInMemoryQueue_FutureTaskNotClaimed(t *testing.T) {
	q := NewInMemoryQueue(10)
	if err := q.Enqueue(Task{ID: "futuro", NextAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	mustNotClaim(t, q, 300*time.Millisecond, "task con NextAt nel futuro")
}

func TestInMemoryQueue_CancelledCtxStopsDelivery(t *testing.T) {
	q := NewInMemoryQueue(10)
	if err := q.Enqueue(Task{ID: "x"}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := q.Claim(ctx); err == nil {
		t.Error("con ctx cancellato Claim non deve consegnare task")
	}
}

func TestInMemoryQueue_EnqueueRespectsMax(t *testing.T) {
	q := NewInMemoryQueue(1)
	if err := q.Enqueue(Task{ID: "1"}); err != nil {
		t.Fatalf("primo Enqueue: %v", err)
	}
	if err := q.Enqueue(Task{ID: "2"}); err == nil {
		t.Error("il secondo Enqueue deve fallire: coda piena")
	}
}

// Retry rimette in coda con il nuovo istante di eleggibilità.
func TestInMemoryQueue_RetryRequeues(t *testing.T) {
	q := NewInMemoryQueue(10)
	if err := q.Enqueue(Task{ID: "r"}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	task := claimNow(t, q)

	if err := q.Retry(task, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("Retry: %v", err)
	}
	mustNotClaim(t, q, 300*time.Millisecond, "il task rimesso in coda per il futuro")
}

// Entrambi gli adapter devono rispettare lo stesso contratto.
func TestTaskQueue_ContractIsShared(t *testing.T) {
	fq, _ := newTestFileQueue(t, 10)

	queues := map[string]TaskQueue{
		"file":     fq,
		"inmemory": NewInMemoryQueue(10),
	}

	for name, q := range queues {
		t.Run(name, func(t *testing.T) {
			if err := q.Enqueue(Task{ID: "c1", Prompt: "p"}); err != nil {
				t.Fatalf("Enqueue: %v", err)
			}
			task := claimNow(t, q)
			if task.ID != "c1" {
				t.Fatalf("ID = %q, atteso 'c1'", task.ID)
			}
			if err := q.Ack(task); err != nil {
				t.Fatalf("Ack: %v", err)
			}
			mustNotClaim(t, q, 300*time.Millisecond, "dopo Ack la coda è vuota")
		})
	}
}
