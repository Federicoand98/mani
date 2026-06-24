package main

import (
	"context"
	"fmt"

	"github.com/Federicoand98/mani/app"
	"github.com/Federicoand98/mani/config"
	"github.com/Federicoand98/mani/core"
	"github.com/Federicoand98/mani/tool"
)

/*
 * Hooks example
 * This is an high-level example that demonstrates the use of hooks.
 * In this example, instead of building directly the `Agent` object, we will build the `App` object that wraps the `Agent`
 * `App` object exposes the full agentic lifecycle.
 */

type EchoIn struct {
	Text string `json:"text" desc:"The text to echo" required:"true"`
}

func main() {
	cfg := config.Config{
		Provider: "ollama",
		Model:    "qwen3.5:9b",
		Providers: map[string]config.ProviderConfig{
			"ollama": {BaseURL: "http://localhost:11434"},
		},
		ContextWindow: 8192,
		Thinking:      false,
	}

	echoTool := tool.MustDefine(
		"echo",
		"Echoes the input text back",
		core.RiskNone,
		func(ctx context.Context, in EchoIn) (string, error) {
			return in.Text, nil
		},
	)

	runtime := app.NewFromConfig(cfg)
	runtime.WithTool(echoTool)

	runtime.OnPreToolUse(func(ctx context.Context, p *core.PreToolUsePayload) error {
		fmt.Printf("\n[HOOK] PRE %s input=%v\n", p.ToolName, p.Input)
		return nil
	})

	runtime.OnPostToolUse(func(ctx context.Context, p *core.PostToolUsePayload) error {
		fmt.Printf("\n[HOOK] POST %s output=%s\n", p.ToolName, p.Result)
		return nil
	})

	prompt := "Use the echo tool to return the word 'hello world'"

	ch := runtime.Execute(context.Background(), prompt)

	for ev := range ch {
		switch ev.Type {
		case app.EventToken:
			fmt.Printf(ev.Payload.(app.TokenPayload).Text)

		case app.EventError:
			fmt.Printf("\nError: %v\n", ev.Payload)
		}
	}

	fmt.Println()
}
