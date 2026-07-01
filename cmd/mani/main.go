package main

import (
	"context"
	"log"
	"os"
	"os/signal"

	"github.com/Federicoand98/mani/app"
	"github.com/Federicoand98/mani/config"
	"github.com/Federicoand98/mani/core"
	"github.com/Federicoand98/mani/session"
	"github.com/Federicoand98/mani/tool/bash"
	fstools "github.com/Federicoand98/mani/tool/fs"
	"github.com/Federicoand98/mani/tui"
)

func main() {
	ws, _ := os.Getwd()

	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	store, err := session.NewFileStore(config.SessionsDir())
	if err != nil {
		log.Fatal(err)
	}

	runtime := app.NewFromConfig(cfg).
		WithSessionStore(store).
		WithTool(fstools.NewReadFileTool(ws)).
		WithTool(fstools.NewEditFileTool(ws)).
		WithTool(fstools.NewWriteFileTool(ws)).
		WithTool(fstools.NewDeleteFileTool(ws)).
		WithTool(bash.NewBashTool(ws)).
		OnPostToolUse(hookToolAudit).
		UsePermissionManager()

	app.RegistrerContextInjection(runtime, ws)
	app.RegisterTrimCompaction(runtime, 20)
	app.RegisterPlanning(runtime)
	app.RegisterSubagents(runtime, 3)

	if len(os.Args) > 1 && os.Args[1] == "serve" {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()

		app.NewTrigger(runtime).
			Webhook(":8787").
			Run(ctx)
		// Every(30*time.Minute, "test").

		return
	}

	if err := tui.Run(runtime); err != nil {
		log.Fatal(err)
	}
}

func hookToolAudit(ctx context.Context, p *core.PostToolUsePayload) error {
	log.Printf("[AUDIT] post_tool_use: %s\n\n", p.ToolName)
	return nil
}
