package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/Federicoand98/mani/config"
	"github.com/Federicoand98/mani/core"
	"github.com/Federicoand98/mani/llm/ollama"
	"github.com/Federicoand98/mani/session"
	"github.com/Federicoand98/mani/tool"
	"github.com/Federicoand98/mani/tool/mcp"
)

type Runtime struct {
	agent           *core.Agent
	cfg             config.Config
	thinkingEnabled bool
	permission      *PermissionManager
	store           session.Store    // prima era core.Memory
	current         *session.Session // sessione attiva
	tools           []tool.Tool
	mcpSessions     []*mcp.Session
	cancel          context.CancelFunc
	mu              sync.Mutex // protege l'accesso a cancel
}

type runIDKey struct{}

func NewFromConfig(cfg config.Config) *Runtime {
	auth, _ := config.LoadAuth()
	// var client core.LLMClient = ollama.NewOllamaClient(cfg.OllamaBaseURL, cfg.Model)
	// client = NewRetryClient(client, 3, 500*time.Millisecond)

	client, err := newLLMClient(cfg, auth)
	if err != nil {
		client = NewRetryClient(ollama.NewOllamaClient(cfg.ProviderBaseURL("ollama"), cfg.ProviderModel("ollama")), 3, 500*time.Millisecond)
	}

	agent := core.NewAgent(client)
	agent.SetContextLimit(cfg.ContextWindow)
	agent.SetMaxIterations(cfg.MaxIterations)

	return &Runtime{
		agent:           agent,
		cfg:             cfg,
		thinkingEnabled: cfg.Thinking,
		store:           session.NewInMemoryStore(),
		current:         session.New(cfg.ActiveModel()),
	}
}

// WithTool adds a tool to the runtime and returns a new runtime instance.
func (r *Runtime) WithTool(t tool.Tool) *Runtime {
	r.agent.AddTool(tool.ToDefinition(t), t)
	r.tools = append(r.tools, t)
	return r
}

func (r *Runtime) AddMCPServer(ctx context.Context, spec mcp.ServerSpec) error {
	sess, err := mcp.Connect(ctx, spec)
	if err != nil {
		return err
	}
	r.mcpSessions = append(r.mcpSessions, sess)

	for _, t := range sess.Tools() {
		r.WithTool(t)
	}

	slog.Info("[mcp] server connected", "name", spec.Name, "tools", len(sess.Tools()))
	return nil
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

func (r *Runtime) UseProvider(name string) error {
	prev := r.cfg.Provider
	r.cfg.Provider = name
	if err := r.rebuildClient(); err != nil {
		r.cfg.Provider = prev
		return err
	}

	_ = config.Save(r.cfg)
	return nil
}

func (r *Runtime) UseModel(name string) error {
	prev := r.cfg.ActiveModel()
	r.cfg.SetActiveModel(name)
	if err := r.rebuildClient(); err != nil {
		r.cfg.SetActiveModel(prev)
		return err
	}

	_ = config.Save(r.cfg)
	return nil
}

func (r *Runtime) AvailableProviders() []string {
	out := make([]string, 0, len(r.cfg.Providers))
	for k := range r.cfg.Providers {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func (r *Runtime) ListModels(ctx context.Context) ([]string, error) {
	if ml, ok := r.agent.Client.(core.ModelLister); ok {
		return ml.ListModels(ctx)
	}
	return nil, fmt.Errorf("provider %q does not support model listing", r.cfg.Provider)
}

func (r *Runtime) Provider() string  { return r.cfg.Provider }
func (r *Runtime) ModelName() string { return r.cfg.ActiveModel() }

func (r *Runtime) SetMaxIterations(n int) *Runtime {
	r.agent.SetMaxIterations(n)
	return r
}

func (r *Runtime) rebuildClient() error {
	auth, _ := config.LoadAuth()
	client, err := newLLMClient(r.cfg, auth)
	if err != nil {
		return err
	}

	r.agent.Client = client
	return nil
}

// ------------- SUBAGENTS ----------------

func (r *Runtime) spawnChild() *core.Agent {
	child := core.NewAgent(r.agent.Client)
	child.SetContextLimit(r.cfg.ContextWindow)
	child.SetMaxIterations(r.cfg.MaxIterations)

	for _, t := range r.tools {
		child.AddTool(tool.ToDefinition(t), t)
	}

	if r.permission != nil {
		child.AddPreToolUseHook(r.permission.Hook())
	}

	return child
}

// ------------- PLANNING ----------------

func (r *Runtime) SetPlan(steps []session.PlanStep) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.current.Plan = steps
}

func (r *Runtime) Plan() []session.PlanStep {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]session.PlanStep(nil), r.current.Plan...) // copia difensiva
}

// PlanText: rappresenta il piano attuale come testo, utile per TUI
func (r *Runtime) PlanText() string {
	return renderPlanText(r.Plan())
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
	runCtx = context.WithValue(runCtx, runIDKey{}, newRunID())
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

// LastResponse returns the last response from the agent in the current session
func (r *Runtime) LastResponse() string {
	msg := r.current.Memory().Messages()

	for i := len(msg) - 1; i >= 0; i-- {
		if msg[i].Role == core.RoleAssistant {
			continue
		}

		for _, b := range msg[i].Content {
			if tb, ok := b.(core.TextBlock); ok {
				return tb.Text
			}
		}
	}

	return ""
}

// --------------- Commands ----------------

func (r *Runtime) Login(provider string, cred config.Credential) error {
	auth, _ := config.LoadAuth()
	auth.Set(provider, cred)
	if err := config.SaveAuth(auth); err != nil {
		return err
	}

	if provider == r.cfg.Provider {
		return r.rebuildClient()
	}

	return nil
}

func (r *Runtime) Logout(provider string) error {
	auth, _ := config.LoadAuth()
	auth.Delete(provider)
	return config.SaveAuth(auth)
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
	r.current = session.New(r.cfg.ActiveModel())
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
	return fmt.Sprintf("provider: %s\nmodel: %s\nui: %s\nthinking: %v\nbase_url: %s\n", r.cfg.Provider, r.cfg.ActiveModel(), r.cfg.UI, r.cfg.Thinking, r.cfg.ProviderBaseURL(r.cfg.Provider))
}

func (r *Runtime) IsDebugMode() bool {
	return r.cfg.Debug
}

func runIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(runIDKey{}).(string)
	return id
}

func newRunID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
