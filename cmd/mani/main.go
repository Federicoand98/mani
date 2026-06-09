package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Federicoand98/mani/config"
	"github.com/Federicoand98/mani/core"
	"github.com/Federicoand98/mani/llm/ollama"
)

func main() {
	cfg := config.FromEnv()
	ctx := context.Background()

	client := ollama.NewOllamaClient(cfg.OllamaBaseURL, cfg.OllamaModel)
	agent := core.NewAgent(client)
	memory := core.NewInMemory()

	fmt.Printf("mani agent - %s\nCtrl+C to exit\n\n", cfg.OllamaModel)

	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		if input == "/quit" {
			fmt.Println("Goodbye!")
			break
		}

		err := agent.Run(ctx, memory, input)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}

		messages := memory.Messages()
		lastMsg := messages[len(messages)-1]
		fmt.Printf("\n%s\n\n", core.TextFrom(lastMsg.Content))
	}
}
