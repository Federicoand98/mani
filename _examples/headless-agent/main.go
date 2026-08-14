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

	// Headless: pass a nil Emitter to stay silent, and read the answer from the
	// returned RunResult instead of digging through memory.
	res, err := agent.Run(context.Background(), memory, prompt, nil)
	if err != nil {
		log.Fatal(err)
	}

	log.Println("final answer:", res.Text)

	// RunResult.FinalResult is non-nil only when a final tool (structured output)
	// is configured with agent.SetFinalTool.
	if res.FinalResult != nil {
		log.Println("structured result:", res.FinalResult)
	}
}
