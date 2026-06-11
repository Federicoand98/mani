package main

import (
	"context"
	"os"

	"github.com/Federicoand98/mani/app"
	"github.com/Federicoand98/mani/cli"
	"github.com/Federicoand98/mani/config"
	fstools "github.com/Federicoand98/mani/tool/fs"
)

func main() {
	ws, _ := os.Getwd()

	runtime := app.NewFromConfig(config.FromEnv()).
		WithTool(fstools.NewReadFileTool(ws)).
		WithTool(fstools.NewEditFileTool(ws))

	cli.New(runtime).Run(context.Background())
}

// const (
// 	colorReset   = "\033[0m"
// 	colorDimGrey = "\033[2m\033[90m"
// )

// mu serializza tutte le scritture su stdout tra il goroutine dello spinner
// e il goroutine principale che chiama il token handler.

// func main() {
// 	cfg := config.FromEnv()
// 	ctx := context.Background()

// 	thinkingEnabled := true
// 	workspaceDir, _ := os.Getwd()

// 	readFileTool := fstools.NewReadFileTool(workspaceDir)

// 	client := ollama.NewOllamaClient(cfg.OllamaBaseURL, cfg.OllamaModel)
// 	memory := core.NewInMemory()
// 	agent := core.NewAgent(client)
// 	agent.AddTool(tool.ToDefinition(readFileTool), readFileTool)

// 	fmt.Printf("mani agent - %s\nCtrl+C to exit\n\n", cfg.OllamaModel)

// 	scanner := bufio.NewScanner(os.Stdin)

// 	for {
// 		fmt.Print("> ")
// 		if !scanner.Scan() {
// 			break
// 		}

// 		input := strings.TrimSpace(scanner.Text())
// 		switch input {
// 		case "":
// 			continue
// 		case "/quit":
// 			fmt.Println("Goodbye!")
// 			return
// 		case "/thinking":
// 			thinkingEnabled = !thinkingEnabled
// 			if thinkingEnabled {
// 				fmt.Println("[thinking mode: ON - reasoning tokens will be visible]")
// 			} else {
// 				fmt.Println("[thinking mode: OFF - reasoning tokens will be hidden]")
// 			}
// 			continue
// 		}

// 		spinCtx, stopSpinner := context.WithCancel(ctx)
// 		go runSpinner(spinCtx)

// 		agent.SetStreamHandler(makeStreamHandler(&thinkingEnabled, stopSpinner))

// 		err := agent.Run(ctx, memory, input)
// 		stopSpinner()

// 		// cancella la riga dello spinner in ogni caso (errore, nessun token ricevuto, ecc.)
// 		mu.Lock()
// 		fmt.Print("\r\033[2K")
// 		mu.Unlock()

// 		fmt.Println()

// 		if err != nil {
// 			fmt.Printf("Error: %v\n", err)
// 			continue
// 		}
// 		fmt.Println()
// 	}
// }
