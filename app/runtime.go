package app

import (
	"context"
	"fmt"

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
}

func NewFromConfig(cfg config.Config) *Runtime {
	client := ollama.NewOllamaClient(cfg.OllamaBaseURL, cfg.OllamaModel)
	agent := core.NewAgent(client)

	return &Runtime{
		agent:           agent,
		cfg:             cfg,
		thinkingEnabled: true,
		store:           session.NewInMemoryStore(),
		current:         session.New(cfg.OllamaModel),
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

func (r *Runtime) Execute(ctx context.Context, input string) <-chan Event {
	// TODO: per ora il canale lo faccio buffered ma devo tener presente questo:
	// cosa succede se la CLI renderizza piu lentamente di quanto l'agent produce i token?
	ch := make(chan Event, 32)

	r.agent.SetEmitter(&channelEmitter{ch: ch, thinking: r.thinkingEnabled})

	if r.permission != nil {
		r.permission.setEmit(func(p PermissionRequestPayload) {
			ch <- Event{Type: EventPermissionRequest, Payload: p}
		})
	}

	go func() {
		defer close(ch)

		if err := r.agent.Run(ctx, r.current.Memory(), input); err != nil {
			ch <- Event{Type: EventError, Payload: ErrorPayload{Err: err}}
			return
		}

		ch <- Event{Type: EventDone}
	}()

	return ch
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

func (r *Runtime) NewSession() {
	r.current = session.New(r.cfg.OllamaModel)
}

func (r *Runtime) SwitchSession(id string) error {
	s, err := r.store.Load(id)
	if err != nil {
		return err
	}
	r.current = s
	return nil
}

func (r *Runtime) ListSessions() ([]session.Meta, error) {
	return r.store.List()
}

func (r *Runtime) CurrentSession() *session.Session {
	return r.current
}
