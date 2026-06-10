package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Federicoand98/mani/config"
	"github.com/Federicoand98/mani/core"
	"github.com/Federicoand98/mani/llm/ollama"
)

const (
	colorReset   = "\033[0m"
	colorDimGrey = "\033[2m\033[90m"
)

// mu serializza tutte le scritture su stdout tra il goroutine dello spinner
// e il goroutine principale che chiama il token handler.
var mu sync.Mutex

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

		spinCtx, stopSpinner := context.WithCancel(ctx)
		go runSpinner(spinCtx)

		agent.SetStreamHandler(makeStreamHandler(&thinkingEnabled, stopSpinner))

		err := agent.Run(ctx, memory, input)
		stopSpinner()

		// cancella la riga dello spinner in ogni caso (errore, nessun token ricevuto, ecc.)
		mu.Lock()
		fmt.Print("\r\033[2K")
		mu.Unlock()

		fmt.Println()

		if err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}
		fmt.Println()
	}
}

func runSpinner(ctx context.Context) {
	frames := []string{
		"⠋ thinking",
		"⠙ thinking",
		"⠹ thinking",
		"⠸ thinking",
		"⠼ thinking",
		"⠴ thinking",
		"⠦ thinking",
		"⠧ thinking",
	}

	// primo frame subito, senza aspettare il tick
	mu.Lock()
	fmt.Print("\r" + frames[0])
	mu.Unlock()

	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()

	i := 1
	for {
		select {
		case <-ctx.Done():
			// il chiamante (handler o main loop) pulisce la riga: qui non stampiamo nulla
			return
		case <-ticker.C:
			mu.Lock()
			// controlla di nuovo dentro il lock: il contesto potrebbe essere stato
			// cancellato mentre aspettavamo di acquisire il mutex
			select {
			case <-ctx.Done():
				mu.Unlock()
				return
			default:
				fmt.Printf("\r%s", frames[i%len(frames)])
				mu.Unlock()
			}
			i++
		}
	}
}

func makeStreamHandler(thinkingEnabled *bool, stopSpinner context.CancelFunc) core.TokenHandler {
	cleared := false

	return func(token string, isThinking bool) {
		mu.Lock()
		defer mu.Unlock()

		// al primo token: pulisci la riga dello spinner e fermalo
		if !cleared {
			fmt.Print("\r\033[2K")
			stopSpinner()
			cleared = true
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
