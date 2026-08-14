package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/Federicoand98/mani/core"
	"github.com/Federicoand98/mani/llm/ollama"
	"github.com/Federicoand98/mani/tool"
)

/*
 * Custom tool example
 * In this example, we define a custom tool that adds two integers together.
 */

type AddIn struct {
	A int `json:"a" desc:"The first number to add" required:"true"`
	B int `json:"b" desc:"The second number to add" required:"true"`
}

func main() {
	client := ollama.NewOllamaClient("http://localhost:11434", "qwen3.5:9b")

	agent := core.NewAgent(client)
	memory := core.NewInMemory()

	sumTool := tool.MustDefine(
		"add",
		"Adds two integers",
		core.RiskNone,
		func(ctx context.Context, in AddIn) (string, error) {
			return fmt.Sprintf("%d", in.A+in.B), nil
		},
	)

	agent.AddTool(tool.ToDefinition(sumTool), sumTool)

	prompt := "What is 5 + 3? Use the tools available to you."

	// Run takes an Emitter (streaming sink) and returns the turn's result.
	// core.NewWriterEmitter(os.Stdout) streams tokens; nil would stay silent.
	res, err := agent.Run(context.Background(), memory, prompt, core.NewWriterEmitter(os.Stdout))
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("\n\nfinal answer: %s\n", res.Text)
}
