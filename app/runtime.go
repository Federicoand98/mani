package app

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Federicoand98/mani/config"
	"github.com/Federicoand98/mani/core"
	"github.com/Federicoand98/mani/llm/ollama"
	"github.com/Federicoand98/mani/session"
	"github.com/Federicoand98/mani/tool"
)

type Runtime struct {
	agent           *core.Agent
	cfg             config.Config
	thinkingEnabled bool
	permission      *PermissionManager
	store           session.Store    // prima era core.Memory
	current         *session.Session // sessione attiva
	cancel          context.CancelFunc
	mu              sync.Mutex // protege l'accesso a cancel
}

func NewFromConfig(cfg config.Config) *Runtime {
	var client core.LLMClient = ollama.NewOllamaClient(cfg.OllamaBaseURL, cfg.Model)
	client = NewRetryClient(client, 3, 500*time.Millisecond)
	agent := core.NewAgent(client)
	agent.SetContextLimit(cfg.ContextWindow)

	return &Runtime{
		agent:           agent,
		cfg:             cfg,
		thinkingEnabled: cfg.Thinking,
		store:           session.NewInMemoryStore(),
		current:         session.New(cfg.Model),
	}
}

// WithTool adds a tool to the runtime and returns a new runtime instance.
func (r *Runtime) WithTool(t tool.Tool) *Runtime {
	r.agent.AddTool(tool.ToDefinition(t), t)
	return r
}

func (r *Runtime) UsePermissionManager() *Runtime {
	r.permission = NewPermissionManager()
	r.agent.AddPreToolUseHook(r.permission.Hook())
	return r
}

func (r *Runtime) WithSessionStore(s session.Store) *Runtime {
	r.store = s
	return r
}

// ------------ HOOKS ----------------

func (r *Runtime) OnPreToolUse(fn func(context.Context, *core.PreToolUsePayload) error) *Runtime {
	r.agent.Hooks().OnPreToolUse(fn)
	return r
}

func (r *Runtime) OnPostToolUse(fn func(context.Context, *core.PostToolUsePayload) error) *Runtime {
	r.agent.Hooks().OnPostToolUse(fn)
	return r
}

func (r *Runtime) OnPreLLMCall(fn func(context.Context, *core.PreLLMCallPayload) error) *Runtime {
	r.agent.Hooks().OnPreLLMCall(fn)
	return r
}

func (r *Runtime) OnPostLLMCall(fn func(context.Context, *core.PostLLMCallPayload) error) *Runtime {
	r.agent.Hooks().OnPostLLMCall(fn)
	return r
}

func (r *Runtime) OnContextFull(fn func(context.Context, *core.ContextFullPayload) error) *Runtime {
	r.agent.Hooks().OnContextFull(fn)
	return r
}

func (r *Runtime) OnSessionStart(fn func(context.Context, *SessionEventPayload) error) *Runtime {
	r.agent.Hooks().Register(func(ctx context.Context, ev core.HookEvent) error {
		if ev.Type == HookSessionStart {
			payload := ev.Payload.(*SessionEventPayload)
			return fn(ctx, payload)
		}
		return nil
	})
	return r
}

func (r *Runtime) OnSessionEnd(fn func(context.Context, *SessionEventPayload) error) *Runtime {
	r.agent.Hooks().Register(func(ctx context.Context, ev core.HookEvent) error {
		if ev.Type == HookSessionEnd {
			payload := ev.Payload.(*SessionEventPayload)
			return fn(ctx, payload)
		}
		return nil
	})
	return r
}

// ----------------------------------

func (r *Runtime) Execute(ctx context.Context, input string) <-chan Event {
	// TODO: per ora il canale lo faccio buffered ma devo tener presente questo:
	// cosa succede se la CLI renderizza piu lentamente di quanto l'agent produce i token?
	ch := make(chan Event, 32)

	runCtx, cancel := context.WithCancel(ctx)
	r.mu.Lock()
	r.cancel = cancel
	r.mu.Unlock()

	r.agent.SetEmitter(&channelEmitter{ch: ch, thinking: r.thinkingEnabled})

	if r.permission != nil {
		r.permission.setEmit(func(p PermissionRequestPayload) {
			ch <- Event{Type: EventPermissionRequest, Payload: p}
		})
	}

	go func() {
		defer close(ch)
		defer cancel()

		err := r.agent.Run(runCtx, r.current.Memory(), input)

		switch {
		case errors.Is(err, context.Canceled):
			_ = r.store.Save(r.current) // memoria coerente marcata
			ch <- Event{Type: EventCancelled}
		case err != nil:
			ch <- Event{Type: EventError, Payload: ErrorPayload{Err: err}}
		default:
			_ = r.store.Save(r.current)
			ch <- Event{Type: EventDone}
		}
	}()

	return ch
}

// Cancel cancels the current turn. Safe to call concurrently.
func (r *Runtime) Cancel() {
	r.mu.Lock()
	c := r.cancel
	r.mu.Unlock()

	if c != nil {
		c()
	}
}

func (r *Runtime) ToggleThinking() bool {
	r.thinkingEnabled = !r.thinkingEnabled
	return r.thinkingEnabled
}

func (r *Runtime) ClearMemory() {
	r.current.Memory().Clear()
}

func (r *Runtime) Memory() string {
	return fmt.Sprintf("%v", r.current.Memory().Messages())
}

func (r *Runtime) ContextLimit() int {
	return r.cfg.ContextWindow
}

func (r *Runtime) NewSession() {
	r.fireSession(HookSessionEnd)
	r.current = session.New(r.cfg.Model)
	r.fireSession(HookSessionStart)
}

func (r *Runtime) SwitchSession(id string) error {
	s, err := r.store.Load(id)
	if err != nil {
		return err
	}
	r.fireSession(HookSessionEnd)
	r.current = s
	r.fireSession(HookSessionStart)
	return nil
}

func (r *Runtime) ListSessions() ([]session.Meta, error) {
	return r.store.List()
}

func (r *Runtime) CurrentSession() *session.Session {
	return r.current
}

func (r *Runtime) fireSession(t core.HookType) {
	_ = r.agent.Hooks().Fire(context.Background(), core.HookEvent{
		Type:    t,
		Payload: &SessionEventPayload{SessionID: r.current.ID, Title: r.current.Title},
	})
}

func (r *Runtime) ConfigString() string {
	return fmt.Sprintf("provider: %s\nmodel: %s\nui: %s\nthinking: %v\nbase_url: %s\n", r.cfg.Provider, r.cfg.Model, r.cfg.UI, r.cfg.Thinking, r.cfg.OllamaBaseURL)
}

func (r *Runtime) IsDebugMode() bool {
	return r.cfg.Debug
}
