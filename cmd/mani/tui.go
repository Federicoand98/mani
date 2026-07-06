package main

import (
	"context"
	"os"

	"github.com/Federicoand98/mani/app"
	"github.com/Federicoand98/mani/config"
	"github.com/Federicoand98/mani/session"
	"github.com/Federicoand98/mani/tool/bash"
	fstools "github.com/Federicoand98/mani/tool/fs"
	"github.com/Federicoand98/mani/tool/mcp"
	"github.com/Federicoand98/mani/tui"
)

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
		UsePermissionManager()

	app.RegistrerContextInjection(runtime, ws)
	app.RegisterTrimCompaction(runtime, 20)
	app.RegisterPlanning(runtime)
	app.RegisterSubagents(runtime, 3)
	app.RegisterTracing(runtime)

	_ = runtime.AddMCPServer(ctx, mcp.ServerSpec{Name: "deepwiki", URL: "https://mcp.deepwiki.com/sse"})

	return tui.Run(runtime)
}
