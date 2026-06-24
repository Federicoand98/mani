package main

import (
	"context"
	"log"
	"time"

	"github.com/Federicoand98/mani/core"
	"github.com/Federicoand98/mani/llm/ollama"
	"github.com/Federicoand98/mani/tool"
)

/*
 * Headless Agent example
 * Autonomous agent that runs in the background without a user interface
 * It executes tasks autonomously and prints the last output from the agent's memory
 */

type NowIn struct{}

func main() {
	client := ollama.NewOllamaClient("http://localhost:11434", "qwen3.5:9b")

	agent := core.NewAgent(client)
	memory := core.NewInMemory()

	nowTool := tool.MustDefine(
		"now",
		"Returns the current date and time",
		core.RiskNone,
		func(ctx context.Context, in NowIn) (string, error) {
			return time.Now().Format(time.RFC3339), nil
		},
	)

	agent.AddTool(tool.ToDefinition(nowTool), nowTool)

	prompt := "What is the current date and time? use the now tool"

	if err := agent.Run(context.Background(), memory, prompt); err != nil {
		log.Fatal(err)
	}

	// extract the last output from the agent's memory
	msgs := memory.Messages()
	if len(msgs) == 0 {
		log.Fatal("no messages in memory")
	}

	last := msgs[len(msgs)-1]
	log.Println("last output:", last)

	for _, block := range last.Content {
		if tb, ok := block.(core.TextBlock); ok {
			log.Println("text block:", tb.Text)
		}
	}
}
