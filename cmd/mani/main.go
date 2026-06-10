package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Federicoand98/mani/config"
	"github.com/Federicoand98/mani/core"
	"github.com/Federicoand98/mani/llm/ollama"
)

const (
	colorReset   = "\033[0m"
	colorDimGrey = "\033[2m\033[90m"
)

func main() {
	cfg := config.FromEnv()
	ctx := context.Background()
	thinkingEnabled := true

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
		switch input {
		case "":
			continue
		case "/quit":
			fmt.Println("Goodbye!")
			return
		case "/thinking":
			thinkingEnabled = !thinkingEnabled
			if thinkingEnabled {
				fmt.Println("[thinking mode: ON - reasoning tokens will be visible]")
			} else {
				fmt.Println("[thinking mode: OFF - reasoning tokens will be hidden]")
			}
			continue
		}

		// spinner
		spinCtx, stopSpinner := context.WithCancel(ctx)
		go runSpinner(spinCtx)

		agent.SetStreamHandler(makeStreamHandler(&thinkingEnabled, stopSpinner))

		err := agent.Run(ctx, memory, input)
		stopSpinner()
		fmt.Println()

		if err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}

		// messages := memory.Messages()
		// lastMsg := messages[len(messages)-1]
		// fmt.Printf("\n%s\n\n", core.TextFrom(lastMsg.Content))
		fmt.Println()
	}
}

func runSpinner(ctx context.Context) {
	frames := []string{"⣾ thinking", "⣷ thinking", "⣯ thinking", "⣟ thinking", "⣻ thinking", "⣽ thinking"}
	i := 0

	for {
		select {
		case <-ctx.Done():
			fmt.Print("\r\033[2k")
			return
		case <-time.After(300 * time.Millisecond):
			fmt.Printf("\r%s", frames[i%len(frames)])
			i++
		}
	}
}

func makeStreamHandler(thinkingEnabled *bool, stopSpinner context.CancelFunc) core.TokenHandler {
	firstToken := true

	return func(token string, isThinking bool) {
		if firstToken {
			stopSpinner()
			firstToken = false
		}

		if isThinking {
			if *thinkingEnabled {
				fmt.Print(colorDimGrey + token + colorReset)
			}
			return
		}

		fmt.Print(token)
	}
}
