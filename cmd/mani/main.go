package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"

	"github.com/Federicoand98/mani/app"
	"github.com/Federicoand98/mani/config"
	"github.com/Federicoand98/mani/session"
	"github.com/Federicoand98/mani/tool/bash"
	fstools "github.com/Federicoand98/mani/tool/fs"
	"github.com/Federicoand98/mani/tool/mcp"
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

	if len(os.Args) > 1 && os.Args[1] == "run" {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		if err := runFromManifest(ctx, os.Args[2:]); err != nil {
			slog.Error("run", "err", err)
			os.Exit(1)
		}
		return
	}

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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	runtime.AddMCPServer(ctx, mcp.ServerSpec{Name: "deepwiki", URL: "https://mcp.deepwiki.com/sse"})

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

func runFromManifest(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	configPath := fs.String("config", "", "path to the manifest config file")
	task := fs.String("task", "", "the task to run (headless mode)")
	_ = fs.Parse(args)

	if *configPath == "" {
		return fmt.Errorf("run: --config required")
	}

	spec, err := app.LoadManifest(*configPath)
	if err != nil {
		return err
	}

	rt, err := app.Build(ctx, spec)
	if err != nil {
		return err
	}

	if *task == "" {
		return fmt.Errorf("run: --task required (for now)")
	}

	for ev := range rt.Execute(ctx, *task) {
		switch ev.Type {
		case app.EventDone:
			fmt.Println(rt.LastResponse())
		case app.EventError:
			if p, ok := ev.Payload.(app.ErrorPayload); ok {
				return p.Err
			}
		}
	}
	return nil
}
