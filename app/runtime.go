package app

import (
	"context"
	"fmt"

	"github.com/Federicoand98/mani/config"
	"github.com/Federicoand98/mani/core"
	"github.com/Federicoand98/mani/llm/ollama"
	"github.com/Federicoand98/mani/tool"
)

type Runtime struct {
	agent           *core.Agent
	memory          core.Memory
	cfg             config.Config
	thinkingEnabled bool
	permission      *PermissionManager
}

func NewFromConfig(cfg config.Config) *Runtime {
	client := ollama.NewOllamaClient(cfg.OllamaBaseURL, cfg.OllamaModel)
	agent := core.NewAgent(client)

	return &Runtime{
		agent:           agent,
		memory:          core.NewInMemory(),
		cfg:             cfg,
		thinkingEnabled: true,
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

		if err := r.agent.Run(ctx, r.memory, input); err != nil {
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
	r.memory.Clear()
}

func (r *Runtime) Memory() string {
	return fmt.Sprintf("%v", r.memory.Messages())
}
