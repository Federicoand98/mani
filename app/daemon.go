package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
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

type webhookSpec struct {
	id     string
	path   string
	prompt string
	memory string
	token  string
}

type Daemon struct {
	rt          *Runtime
	queue       TaskQueue
	crons       []cronSpec
	dailies     []dailySpec
	webhooks    []webhookSpec
	addr        string
	policy      Policy
	concurrency int
	maxAttempts int           // tentativi massimi per task
	backoff     time.Duration // backoff base (esponenziale sui tentativi)
	state       *triggerState
}

const (
	maxWebhookBody  = 64 << 10 // 64KB
	defaultHookPath = "/hook"
)

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
		slog.Error("[daemon]: daily trigger disabled, invalid time", "trigger", id, "at", clock, "err", err)
		return d
	}

	d.dailies = append(d.dailies, dailySpec{
		id: id, hour: h, minute: m, prompt: prompt, memory: memory, catchUp: catchUp,
	})
	return d
}

func (d *Daemon) Webhook(addr string, w webhookSpec) *Daemon {
	if addr != "" {
		d.addr = addr
	}
	// il default vive qui perche' e' l'imbuto: http.ServeMux va in panic su un
	// pattern vuoto, e un manifest pre-0.1.4 non dichiara nessun path.
	if w.path == "" {
		w.path = defaultHookPath
	}
	d.webhooks = append(d.webhooks, w)
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
		slog.Info("[daemon]: task interrupted, requeued", "id", t.ID, "trigger", t.Trigger)
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
	if host, _, err := net.SplitHostPort(d.addr); err == nil && (host == "" || host == "0.0.0.0" || host == "::") {
		slog.Warn("[daemon]: webhook listening on all interfaces", "addr", d.addr, "hint", "use 127.0.0.1:PORT to restrict access to localhost")
	}

	srv := &http.Server{Addr: d.addr, Handler: d.webhookHandler()}
	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()

	slog.Info("[daemon]: webhook listening", "addr", d.addr, "path", "/hook")
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("[daemon]: webhook server", "err", err)
	}
}

func (d *Daemon) webhookHandler() http.Handler {
	mux := http.NewServeMux()

	// auth per path: ogni rotta ha il suo token, quindi revocarne uno non tocca le altre
	for _, w := range d.webhooks {
		mux.Handle(w.path, BearerAuth(w.token, d.hookHandler(w)))
	}

	return mux
}

func (d *Daemon) hookHandler(w webhookSpec) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(rw, "method not allowed (POST only)", http.StatusMethodNotAllowed)
			return
		}

		r.Body = http.MaxBytesReader(rw, r.Body, maxWebhookBody)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(rw, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}

		prompt := renderWebhookPrompt(w.prompt, strings.TrimSpace(string(body)))
		if prompt == "" {
			http.Error(rw, "empty prompt", http.StatusBadRequest)
			return
		}

		if d.enqueue(Task{Source: "webhook", Trigger: w.id, Prompt: prompt, Memory: w.memory}) {
			rw.WriteHeader(http.StatusAccepted)
			_, _ = rw.Write([]byte("accepted\n"))
		} else {
			http.Error(rw, "queue full, try again later", http.StatusServiceUnavailable)
		}
	})
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
		// the last occurrence expected BEFORE now
		prev := previousOccurrence(time.Now(), dl.hour, dl.minute)
		last, ok := d.state.lastRun(dl.id)
		if ok && !last.Before(prev) {
			continue // already run after the last expected occurrence
		}
		slog.Info("[daemon]: catch-up, missed trigger enqueued", "trigger", dl.id, "expected", prev)
		d.enqueue(Task{Source: "daily", Trigger: dl.id, Prompt: dl.prompt, Memory: dl.memory})
		d.state.mark(dl.id, time.Now())
	}
}

// parseClock reads an "HH:MM" wall-clock time in the local timezone.
//
// It is deliberately strict about trailing input: fmt.Sscanf ignores whatever
// follows the pattern, so "09:00 UTC" used to parse as 09:00 local, silently
// running the trigger at a time the manifest did not ask for.
func parseClock(clock string) (int, int, error) {
	invalid := func() (int, int, error) {
		return 0, 0, fmt.Errorf("expected HH:MM in local time, found %q", clock)
	}

	hs, ms, ok := strings.Cut(clock, ":")
	if !ok || !isDigits(hs) || !isDigits(ms) {
		return invalid()
	}

	h, err := strconv.Atoi(hs)
	if err != nil {
		return invalid()
	}
	m, err := strconv.Atoi(ms)
	if err != nil {
		return invalid()
	}

	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, 0, fmt.Errorf("hour must be between 0 and 23, minute must be between 0 and 59, found %q", clock)
	}
	return h, m, nil
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// occurrenceOn returns hour:minute on the day of t shifted by dayOffset days,
// in t's location.
//
// The day is moved here, inside time.Date, instead of by adding 24 hours to the
// result: across a daylight-saving boundary a day lasts 23 or 25 hours, so
// adding a fixed 24 would drift the trigger by an hour for the rest of the year.
// time.Date also normalises the month and year rollover for free.
func occurrenceOn(t time.Time, dayOffset, hour, minute int) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day()+dayOffset, hour, minute, 0, 0, t.Location())
}

// nextOccurrence returns the first hour:minute strictly after now.
func nextOccurrence(now time.Time, hour, minute int) time.Time {
	next := occurrenceOn(now, 0, hour, minute)
	if !next.After(now) {
		next = occurrenceOn(now, 1, hour, minute)
	}
	return next
}

// previousOccurrence returns the most recent hour:minute at or before now.
func previousOccurrence(now time.Time, hour, minute int) time.Time {
	prev := occurrenceOn(now, 0, hour, minute)
	if prev.After(now) {
		prev = occurrenceOn(now, -1, hour, minute)
	}
	return prev
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
