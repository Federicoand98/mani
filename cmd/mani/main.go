package main

import (
	"context"
	"log"
	"os"

	"github.com/Federicoand98/mani/app"
	"github.com/Federicoand98/mani/cli"
	"github.com/Federicoand98/mani/config"
	"github.com/Federicoand98/mani/session"
	"github.com/Federicoand98/mani/tool/bash"
	fstools "github.com/Federicoand98/mani/tool/fs"
	"github.com/Federicoand98/mani/tui"
)

func main() {
	ws, _ := os.Getwd()

	store, err := session.NewFileStore(config.SessionsDir())
	if err != nil {
		log.Fatal(err)
	}

	runtime := app.NewFromConfig(config.FromEnv()).
		WithSessionStore(store).
		WithTool(fstools.NewReadFileTool(ws)).
		WithTool(fstools.NewEditFileTool(ws)).
		WithTool(fstools.NewWriteFileTool(ws)).
		WithTool(fstools.NewDeleteFileTool(ws)).
		WithTool(bash.NewBashTool(ws)).
		UsePermissionManager()

	if os.Getenv("MANI_UI") == "repl" {
		cli.New(runtime).Run(context.Background())
		return
	}

	if err := tui.Run(runtime); err != nil {
		log.Fatal(err)
	}
}
