package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/Federicoand98/mani/app"
	"github.com/Federicoand98/mani/config"
	"github.com/Federicoand98/mani/session"
	"github.com/Federicoand98/mani/tool/bash"
	"github.com/Federicoand98/mani/tool/fetch"
	fstools "github.com/Federicoand98/mani/tool/fs"
	"github.com/Federicoand98/mani/tool/mcp"
	"github.com/Federicoand98/mani/tui"
)

func runTUICommand(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("tui", flag.ExitOnError)
	_ = fs.Parse(args)

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	return runTUI(ctx, cfg)
}

func runTUI(ctx context.Context, cfg config.Config) error {
	ws, _ := os.Getwd()

	store, err := session.NewFileStore(config.SessionsDir())
	if err != nil {
		return err
	}

	runtime := app.NewFromConfig(cfg).
		WithSessionStore(store).
		WithTool(fstools.NewReadFileTool(ws)).
		WithTool(fstools.NewEditFileTool(ws)).
		WithTool(fstools.NewWriteFileTool(ws)).
		WithTool(fstools.NewDeleteFileTool(ws)).
		WithTool(bash.NewBashTool(ws)).
		WithTool(fetch.New()).
		UsePermissionManager()

	if err := runtime.ClientErr(); err != nil {
		fmt.Fprintf(os.Stderr, "[tui]: warning: provider %q is not usable: %v\n", cfg.Provider, err)
	}

	app.RegistrerContextInjection(runtime, ws)
	app.RegisterTrimCompaction(runtime, 20)
	app.RegisterPlanning(runtime)
	app.RegisterSubagents(runtime, 3)
	app.RegisterTracing(runtime)

	_ = runtime.AddMCPServer(ctx, mcp.ServerSpec{Name: "deepwiki", URL: "https://mcp.deepwiki.com/sse"})

	return tui.Run(runtime)
}
