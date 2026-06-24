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
	agent.SetEmitter(core.NewWriterEmitter(os.Stdout))
	memory := core.NewInMemory()

	sumTool := tool.MustDefine(
		"add",
		"Somma due numeri interi",
		core.RiskNone,
		func(ctx context.Context, in AddIn) (string, error) {
			return fmt.Sprintf("%d", in.A+in.B), nil
		},
	)

	agent.AddTool(tool.ToDefinition(sumTool), sumTool)

	prompt := "Quanto fa 5 + 3? utilizza i tool a tua disposizione"

	if err := agent.Run(context.Background(), memory, prompt); err != nil {
		log.Fatal(err)
	}
}
