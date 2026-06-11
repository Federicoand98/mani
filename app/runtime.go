package app

import (
	"context"

	"github.com/Federicoand98/mani/config"
	"github.com/Federicoand98/mani/core"
	"github.com/Federicoand98/mani/llm/ollama"
	"github.com/Federicoand98/mani/tool"
)

type Runtime struct {
	agent  *core.Agent
	memory core.Memory
	cfg    config.Config
}

func NewFromConfig(cfg config.Config) *Runtime {
	client := ollama.NewOllamaClient(cfg.OllamaBaseURL, cfg.OllamaModel)
	agent := core.NewAgent(client)

	return &Runtime{
		agent:  agent,
		memory: core.NewInMemory(),
		cfg:    cfg,
	}
}

// WithTool adds a tool to the runtime and returns a new runtime instance.
func (r *Runtime) WithTool(t tool.Tool) *Runtime {
	r.agent.AddTool(tool.ToDefinition(t), t)
	return r
}

func (r *Runtime) Execute(ctx context.Context, input string) <-chan Event {
	// TODO: per ora il canale lo faccio buffered ma devo tener presente questo:
	// cosa succede se la CLI renderizza piu lentamente di quanto l'agent produce i token?
	ch := make(chan Event, 32)

	r.agent.SetStreamHandler(func(token string, isThinking bool) {
		if isThinking {
			ch <- Event{Type: EventThinking, Payload: TokenPayload{Text: token}}
		} else {
			ch <- Event{Type: EventToken, Payload: TokenPayload{Text: token}}
		}
	})

	r.agent.SetToolEventHandler(func(name string, input map[string]any, result string, isError bool) {
		ch <- Event{Type: EventToolCall, Payload: ToolCallPayload{Name: name, Input: input}}
		ch <- Event{Type: EventToolResult, Payload: ToolCallResultPayload{Name: name, Result: result, IsError: isError}}
	})

	go func() {
		defer close(ch)
		err := r.agent.Run(ctx, r.memory, input)
		if err != nil {
			ch <- Event{Type: EventError, Payload: ErrorPayload{Err: err}}
			return
		}

		ch <- Event{Type: EventDone}
	}()

	return ch
}
