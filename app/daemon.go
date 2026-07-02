package app

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Policy: how the deamon should respond to permission prompts
type Policy int

const (
	PolicyDeny Policy = iota
	PolicyAllow
)

type Task struct {
	Source string // "cron" | "webhook
	Prompt string
}

type cronSpec struct {
	interval time.Duration
	prompt   string
}

type dailySpec struct {
	hour   int
	minute int
	prompt string
}

type Daemon struct {
	rt      *Runtime
	queue   chan Task
	crons   []cronSpec
	dailies []dailySpec
	addr    string
	policy  Policy
}

func NewTrigger(rt *Runtime) *Daemon {
	return &Daemon{rt: rt, queue: make(chan Task, 64), policy: PolicyDeny}
}

// Every schedules a cron job that will prompt the agent at the specified interval
func (d *Daemon) Every(interval time.Duration, prompt string) *Daemon {
	d.crons = append(d.crons, cronSpec{interval: interval, prompt: prompt})
	return d
}

// Daily schedules a daily job that will prompt the agent at the specified time
func (d *Daemon) Daily(clock string, prompt string) *Daemon {
	h, m, err := parseClock(clock)
	if err != nil {
		slog.Warn("trigger: DailyAt orario invalido", "clock", clock)
		return d
	}

	d.dailies = append(d.dailies, dailySpec{hour: h, minute: m, prompt: prompt})
	return d
}

func (d *Daemon) Webhook(addr string) *Daemon {
	d.addr = addr
	return d
}

func (d *Daemon) AllowAll() *Daemon {
	d.policy = PolicyAllow
	return d
}

func (d *Daemon) Run(ctx context.Context) {
	for _, c := range d.crons {
		go d.runCron(ctx, c)
	}

	for _, dl := range d.dailies {
		go d.runDaily(ctx, dl)
	}

	if d.addr != "" {
		go d.runWebhook(ctx)
	}

	d.worker(ctx)
}

func (d *Daemon) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case task := <-d.queue:
			d.execute(ctx, task)
		}
	}
}

func (d *Daemon) execute(ctx context.Context, t Task) {
	slog.Info("trigger run", "source", t.Source, "prompt", t.Prompt)

	ch := d.rt.Execute(ctx, t.Prompt)
	for ev := range ch {
		switch ev.Type {
		case EventPermissionRequest:
			p := ev.Payload.(PermissionRequestPayload)
			if d.policy == PolicyAllow {
				p.Respond <- AllowOnce
			} else {
				p.Respond <- Deny
			}
		case EventError:
			slog.Error("trigger run error", "source", t.Source, "err", ev.Payload)
		}
	}

	slog.Info("trigger done", "source", t.Source)
}

func (d *Daemon) runCron(ctx context.Context, c cronSpec) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.enqueue(Task{Source: "cron", Prompt: c.prompt})
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
			d.enqueue(Task{Source: "daily", Prompt: dl.prompt})
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
		prompt := strings.TrimSpace(string(body))

		if prompt == "" {
			http.Error(w, "empty prompt", http.StatusBadRequest)
			return
		}

		if d.enqueue(Task{Source: "webhook", Prompt: prompt}) {
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
	select {
	case d.queue <- t:
		return true
	default:
		slog.Warn("trigger queue full, discarding", "source", t.Source, "prompt", t.Prompt)
		return false
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
