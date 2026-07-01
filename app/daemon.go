package app

import (
	"context"
	"io"
	"log"
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

type Daemon struct {
	rt     *Runtime
	queue  chan Task
	crons  []cronSpec
	addr   string
	policy Policy
}

func New(rt *Runtime) *Daemon {
	return &Daemon{rt: rt, queue: make(chan Task, 64), policy: PolicyDeny}
}

// Every schedules a cron job that will prompt the agent at the specified interval
func (d *Daemon) Every(interval time.Duration, prompt string) *Daemon {
	d.crons = append(d.crons, cronSpec{interval: interval, prompt: prompt})
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
	log.Printf("[trigger: %s] execute %q", t.Source, t.Prompt)

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
			log.Printf("[trigger error] %v", ev.Payload)
		}
	}

	log.Printf("[trigger: %s] done", t.Source)
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

	log.Printf("[trigger] webhook su %s (POST /hook)", d.addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Printf("[trigger] webhook: %v", err)
	}
}

func (d *Daemon) enqueue(t Task) bool {
	select {
	case d.queue <- t:
		return true
	default:
		log.Printf("[trigger: %s] queue full, discard %q", t.Source, t.Prompt)
		return false
	}
}
