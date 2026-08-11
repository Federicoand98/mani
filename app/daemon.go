package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Federicoand98/mani/session"
)

// Policy: how the deamon should respond to permission prompts
type Policy int

const (
	PolicyDeny Policy = iota
	PolicyAllow
)

// type Task struct {
// 	Source string // "cron" | "webhook
// 	Prompt string
// }

type cronSpec struct {
	id       string
	interval time.Duration
	prompt   string
	memory   string
}

type dailySpec struct {
	id      string
	hour    int
	minute  int
	prompt  string
	memory  string
	catchUp bool
}

type Daemon struct {
	rt            *Runtime
	queue         TaskQueue
	crons         []cronSpec
	dailies       []dailySpec
	addr          string
	webhookPrompt string
	webhookMemory string
	policy        Policy
	concurrency   int
	maxAttempts   int           // tentativi massimi per task
	backoff       time.Duration // backoff base (esponenziale sui tentativi)
	state         *triggerState
}

func NewTrigger(rt *Runtime, q TaskQueue) *Daemon {
	return &Daemon{
		rt:          rt,
		queue:       q,
		policy:      PolicyDeny,
		maxAttempts: 3,
		backoff:     30 * time.Second,
		// stato in RAM: BuildDaemon lo sostituisce con quello persistente se c'e' un path.
		// Inizializzarlo qui evita il nil-panic per chi usa NewTrigger come libreria.
		state: loadTriggerState(""),
	}
}

// Every schedules a cron job that will prompt the agent at the specified interval
func (d *Daemon) Every(id string, interval time.Duration, prompt, memory string) *Daemon {
	d.crons = append(d.crons, cronSpec{id: id, interval: interval, prompt: prompt, memory: memory})
	return d
}

// Daily schedules a daily job that will prompt the agent at the specified time
func (d *Daemon) Daily(id, clock, prompt, memory string, catchUp bool) *Daemon {
	h, m, err := parseClock(clock)
	if err != nil {
		slog.Warn("trigger: DailyAt orario invalido", "clock", clock)
		return d
	}

	d.dailies = append(d.dailies, dailySpec{
		id: id, hour: h, minute: m, prompt: prompt, memory: memory, catchUp: catchUp,
	})
	return d
}

func (d *Daemon) Webhook(addr, promptTemplate, memory string) *Daemon {
	d.addr = addr
	d.webhookPrompt = promptTemplate
	d.webhookMemory = memory
	return d
}

func (d *Daemon) AllowAll() *Daemon {
	d.policy = PolicyAllow
	return d
}

func (d *Daemon) Run(ctx context.Context) {
	if n, err := d.queue.Recover(); err != nil {
		slog.Warn("[daemon]: queue recover failed", "n", n, "err", err)
	} else if n > 0 {
		slog.Info("[daemon]: queue recovered", "n", n)
	}

	d.catchUp(ctx)

	for _, c := range d.crons {
		go d.runCron(ctx, c)
	}

	for _, dl := range d.dailies {
		go d.runDaily(ctx, dl)
	}

	if d.addr != "" {
		go d.runWebhook(ctx)
	}

	n := d.concurrency
	if n < 1 {
		n = 1
	}

	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			d.worker(ctx, worker)
		}(i)
	}
	wg.Wait()
}

func (d *Daemon) worker(ctx context.Context, id int) {
	for {
		t, err := d.queue.Claim(ctx)
		if err != nil {
			return
		}
		d.execute(ctx, t, id)
	}
}

func (d *Daemon) execute(ctx context.Context, t Task, worker int) {
	slog.Info("[daemon]: task start", "id", t.ID, "trigger", t.Trigger, "attempt", t.Attempt, "worker", worker)

	sess := session.New(d.rt.ModelName())
	if t.Memory == "persistent" {
		sess = d.rt.CurrentSession()
	}

	ch, cancel := d.rt.ExecuteIn(WithSource(ctx, t.Source), sess, t.Prompt)
	defer cancel()

	var runErr error
	var cancelled bool
	for ev := range ch {
		switch ev.Type {
		case EventPermissionRequest:
			p := ev.Payload.(PermissionRequestPayload)
			if d.policy == PolicyAllow {
				p.Respond <- AllowOnce
			} else {
				p.Respond <- Deny
			}
		case EventCancelled:
			cancelled = true
		case EventError:
			if p, ok := ev.Payload.(ErrorPayload); ok {
				runErr = p.Err
			}
		}
	}

	// Uno shutdown NON e' un fallimento del task e nemmeno un successo: il task
	// torna in coda senza consumare un tentativo, e riparte al prossimo avvio.
	if cancelled {
		_ = d.queue.Retry(t, time.Now())
		slog.Info("[daemon]: task interrotto, rimesso in coda", "id", t.ID, "trigger", t.Trigger)
		return
	}

	if runErr == nil {
		_ = d.queue.Ack(t)
		slog.Info("[daemon]: task done", "id", t.ID, "trigger", t.Trigger, "attempt", t.Attempt, "worker", worker)
		return
	}

	t.Attempt++
	if t.Attempt >= d.maxAttempts {
		_ = d.queue.Fail(t, runErr.Error())
		slog.Error("[daemon]: task failed", "id", t.ID, "trigger", t.Trigger, "attempt", t.Attempt, "worker", worker, "err", runErr)
		return
	}

	// backoff esponenziale: base, base*2, base*4…
	backoff := d.backoff * time.Duration(1<<(t.Attempt-1))

	_ = d.queue.Retry(t, time.Now().Add(backoff))
	slog.Warn("[daemon] task retry scheduled", "id", t.ID, "attempt", t.Attempt, "in", backoff, "err", runErr)
}

func (d *Daemon) runCron(ctx context.Context, c cronSpec) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.enqueue(Task{Source: "cron", Trigger: c.id, Prompt: c.prompt, Memory: c.memory})
			d.state.mark(c.id, time.Now())
		}
	}
}

func (d *Daemon) runDaily(ctx context.Context, dl dailySpec) {
	for {
		wait := time.Until(nextOccurrence(time.Now(), dl.hour, dl.minute))
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			d.enqueue(Task{Source: "daily", Trigger: dl.id, Prompt: dl.prompt, Memory: dl.memory})
			d.state.mark(dl.id, time.Now())
		}
	}
}

func (d *Daemon) runWebhook(ctx context.Context) {
	mux := http.NewServeMux()

	mux.HandleFunc("/hook", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "method not allowed (POST only)", http.StatusMethodNotAllowed)
			return
		}

		body, _ := io.ReadAll(r.Body)
		sbody := strings.TrimSpace(string(body))
		prompt := renderWebhookPrompt(d.webhookPrompt, sbody)

		if prompt == "" {
			http.Error(w, "empty prompt", http.StatusBadRequest)
			return
		}

		if d.enqueue(Task{Source: "webhook", Trigger: "webhook", Prompt: prompt, Memory: d.webhookMemory}) {
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte("accepted\n"))
		} else {
			http.Error(w, "queue full, try again later", http.StatusServiceUnavailable)
		}
	})

	srv := &http.Server{Addr: d.addr, Handler: mux}
	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()

	slog.Info("webhook listening", "addr", d.addr, "path", "/hook")
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("webhook server", "err", err)
	}
}

func (d *Daemon) enqueue(t Task) bool {
	if t.ID == "" {
		t.ID = newRunID()
	}

	if err := d.queue.Enqueue(t); err != nil {
		slog.Warn("[daemon] enqueue failed", "trigger", t.Trigger, "err", err)
		return false
	}

	return true
}

func (d *Daemon) catchUp(ctx context.Context) {
	for _, dl := range d.dailies {
		if !dl.catchUp {
			continue
		}
		// ultima occorrenza attesa PRIMA di adesso
		prev := nextOccurrence(time.Now(), dl.hour, dl.minute).Add(-24 * time.Hour)
		last, ok := d.state.lastRun(dl.id)
		if ok && !last.Before(prev) {
			continue // già eseguito dopo l'ultima occorrenza attesa
		}
		slog.Info("catch-up: trigger perso, accodo", "trigger", dl.id, "expected", prev)
		d.enqueue(Task{Source: "daily", Trigger: dl.id, Prompt: dl.prompt, Memory: dl.memory})
		d.state.mark(dl.id, time.Now())
	}
}

func parseClock(clock string) (int, int, error) {
	var h, m int
	if _, err := fmt.Sscanf(clock, "%d:%d", &h, &m); err != nil {
		return 0, 0, fmt.Errorf("expected HH:MM: %w", err)
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, 0, fmt.Errorf("hour must be between 0 and 23, minute must be between 0 and 59")
	}
	return h, m, nil
}

func nextOccurrence(now time.Time, hour, minute int) time.Time {
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
	if !next.After(now) {
		next = next.Add(24 * time.Hour)
	}
	return next
}

func renderWebhookPrompt(template, body string) string {
	if template == "" {
		return body
	}
	if strings.Contains(template, "{{body}}") {
		return strings.ReplaceAll(template, "{{body}}", body)
	}
	if body == "" {
		return template
	}
	return template + "\n\n" + body
}

func triggerID(spec TriggerSpec) string {
	if spec.Name != "" {
		return spec.Name
	}
	h := sha256.Sum256([]byte(spec.Type + "|" + spec.Every + "|" + spec.At + "|" + spec.Prompt))
	return hex.EncodeToString(h[:6])
}

type triggerState struct {
	path string
	mu   sync.Mutex
	Last map[string]time.Time
}

func loadTriggerState(path string) *triggerState {
	ts := &triggerState{path: path, Last: map[string]time.Time{}}
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &ts.Last)
	}
	return ts
}

func (s *triggerState) mark(id string, at time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Last[id] = at

	// path vuoto = stato solo in RAM (queue.path assente): niente persistenza,
	// altrimenti scriveremmo un ".tmp" spurio nella working dir a ogni trigger.
	if s.path == "" {
		return
	}

	if b, err := json.MarshalIndent(s.Last, "", "  "); err == nil {
		tmp := s.path + ".tmp"
		if os.WriteFile(tmp, b, 0o644) == nil {
			_ = os.Rename(tmp, s.path)
		}
	}
}

func (s *triggerState) lastRun(id string) (time.Time, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.Last[id]
	return t, ok
}
