// Package app is the application service that wires a runnable agent together.
//
// It owns the Runtime (agent, provider, tools, permissions, exposed as a stream of
// Event), the declarative manifest (RuntimeSpec and its eight blocks), and the
// middleware composed on top of the core hooks: policy rules, limits, planning,
// compaction, subagents, tracing and the run journal. It also holds the trigger
// Daemon and the durable task queue.
//
// Library users can skip app entirely and wire core with the adapters they need.
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
	maxDuration     time.Duration // 0 = no timeout
	mcpSessions     []*mcp.Session
	subagents       map[string]SubagentSpec
	journal         Journal
	cancel          context.CancelFunc
	clientErr       error      // != nil if provider is not available
	mu              sync.Mutex // protege l'accesso a cancel
}

type runIDKey struct{}

func NewFromConfig(cfg config.Config) *Runtime {
	auth, _ := config.LoadAuth()
	// var client core.LLMClient = ollama.NewOllamaClient(cfg.OllamaBaseURL, cfg.Model)
	// client = NewRetryClient(client, 3, 500*time.Millisecond)

	client, clientErr := newLLMClient(cfg, auth)
	if clientErr != nil {
		client = unavailableClient{err: clientErr}
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
func (r *Runtime) ClientErr() error  { return r.clientErr }

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
	r.clientErr = nil
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

	if r.journal != nil {
		attachJournalHooks(child.Hooks(), r.journal)
	}

	return child
}

func (r *Runtime) spawnNamed(name string) (*core.Agent, SubagentSpec, error) {
	spec, ok := r.subagents[name]
	if !ok {
		return nil, SubagentSpec{}, fmt.Errorf("subagent %s not found", name)
	}

	client := r.agent.Client
	if spec.Model != "" {
		auth, _ := config.LoadAuth()
		cfg := r.cfg
		cfg.SetActiveModel(spec.Model)
		c, err := newLLMClient(cfg, auth)
		if err != nil {
			return nil, spec, fmt.Errorf("sub-agent %q: model %q: %w", name, spec.Model, err)
		}
		client = c
	}

	child := core.NewAgent(client)
	child.SetContextLimit(r.cfg.ContextWindow)

	maxIter := r.cfg.MaxIterations
	child.SetMaxIterations(maxIter)

	tools, err := r.subagentsTools(spec.Tools)
	if err != nil {
		return nil, spec, fmt.Errorf("sub-agent %q: %w", name, err)
	}

	for _, t := range tools {
		child.AddTool(tool.ToDefinition(t), t)
	}

	if r.permission != nil {
		child.AddPreToolUseHook(r.permission.Hook())
	}

	if r.journal != nil {
		attachJournalHooks(child.Hooks(), r.journal)
	}

	return child, spec, nil
}

func (r *Runtime) subagentsTools(names []string) ([]tool.Tool, error) {
	if len(names) == 0 {
		out := make([]tool.Tool, 0, len(r.tools))
		for _, t := range r.tools {
			if t.Name() == "delegate" {
				continue
			}
			out = append(out, t)
		}
		return out, nil
	}

	byName := make(map[string]tool.Tool, len(r.tools))
	for _, t := range r.tools {
		byName[t.Name()] = t
	}

	out := make([]tool.Tool, 0, len(names))
	for _, name := range names {
		t, ok := byName[name]
		if !ok {
			return nil, fmt.Errorf("tool %q not found", name)
		}
		out = append(out, t)
	}
	return out, nil
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

func (r *Runtime) ExecuteIn(ctx context.Context, sess *session.Session, input string) (<-chan Event, context.CancelFunc) {
	ch := make(chan Event, 32)

	var runCtx context.Context
	var cancel context.CancelFunc
	if r.maxDuration > 0 {
		runCtx, cancel = context.WithTimeout(ctx, r.maxDuration)
	} else {
		runCtx, cancel = context.WithCancel(ctx)
	}

	runID := newRunID()
	runCtx = context.WithValue(runCtx, runIDKey{}, runID)
	runCtx = context.WithValue(runCtx, budgetKey{}, &budgetState{})
	source := sourceFrom(ctx)

	em := &channelEmitter{ch: ch, thinking: r.thinkingEnabled}
	runCtx = WithPermissionEmit(runCtx, func(prp PermissionRequestPayload) {
		ch <- Event{Type: EventPermissionRequest, Payload: prp}
	})

	if r.journal != nil {
		_ = r.journal.Start(RunRecord{
			ID: runID, SessionID: sess.ID, Source: source, StartedAt: time.Now(), Status: "running",
		})
	}

	go func() {
		defer close(ch)
		defer cancel()

		res, err := r.agent.Run(runCtx, sess.Memory(), input, em)

		switch {
		case errors.Is(err, context.Canceled):
			_ = r.store.Save(sess)
			r.finishRun(runID, "cancelled")
			ch <- Event{Type: EventCancelled}
		case err != nil:
			r.finishRun(runID, "error")
			ch <- Event{Type: EventError, Payload: ErrorPayload{Err: err}}
		default:
			_ = r.store.Save(sess)
			r.finishRun(runID, "ok")
			ch <- Event{Type: EventDone, Payload: DonePayload{
				Result: res.FinalResult,
				Text:   res.Text,
			}}
		}
	}()

	return ch, cancel
}

func (r *Runtime) Execute(ctx context.Context, input string) <-chan Event {
	ch, cancel := r.ExecuteIn(ctx, r.current, input)

	r.mu.Lock()
	r.cancel = cancel
	r.mu.Unlock()

	return ch
}

func (r *Runtime) finishRun(runID string, status string) {
	if r.journal != nil {
		_ = r.journal.Finish(runID, status)
	}
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

// Close closes all MCP sessions and clears the session list.
func (r *Runtime) Close() {
	for _, s := range r.mcpSessions {
		_ = s.Close()
	}
	r.mcpSessions = nil
}

// LastResponse returns the last response from the agent in the current session
func (r *Runtime) LastResponse() string {
	msg := r.current.Memory().Messages()

	for i := len(msg) - 1; i >= 0; i-- {
		if msg[i].Role != core.RoleAssistant {
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
