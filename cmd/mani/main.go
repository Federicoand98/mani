package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"

	"github.com/Federicoand98/mani/app"
	"github.com/Federicoand98/mani/config"
	"github.com/Federicoand98/mani/session"
	"github.com/Federicoand98/mani/tool/bash"
	fstools "github.com/Federicoand98/mani/tool/fs"
	"github.com/Federicoand98/mani/tui"
)

func main() {
	ws, _ := os.Getwd()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config load", "err", err)
		os.Exit(1)
	}

	app.SetupLogging(cfg.LogLevel)

	store, err := session.NewFileStore(config.SessionsDir())
	if err != nil {
		slog.Error("session store", "err", err)
		os.Exit(1)
	}

	runtime := app.NewFromConfig(cfg).
		WithSessionStore(store).
		WithTool(fstools.NewReadFileTool(ws)).
		WithTool(fstools.NewEditFileTool(ws)).
		WithTool(fstools.NewWriteFileTool(ws)).
		WithTool(fstools.NewDeleteFileTool(ws)).
		WithTool(bash.NewBashTool(ws)).
		UsePermissionManager()

	app.RegistrerContextInjection(runtime, ws)
	app.RegisterTrimCompaction(runtime, 20)
	app.RegisterPlanning(runtime)
	app.RegisterSubagents(runtime, 3)
	app.RegisterTracing(runtime)

	if len(os.Args) > 1 && os.Args[1] == "serve" {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()

		app.NewTrigger(runtime).
			Webhook(":8787").
			Run(ctx)

		return
	}

	if err := tui.Run(runtime); err != nil {
		slog.Error("tui", "err", err)
		os.Exit(1)
	}
}
