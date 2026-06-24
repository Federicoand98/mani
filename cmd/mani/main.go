package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/Federicoand98/mani/app"
	"github.com/Federicoand98/mani/config"
	"github.com/Federicoand98/mani/core"
	"github.com/Federicoand98/mani/session"
	"github.com/Federicoand98/mani/tool"
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

	// Se si volesse creare un tool custom
	type WeatherIn struct {
		City string `json:"city" desc: "Nome della città" required: "true"`
		Days int    `json:"days" desc: "Numero di giorni"`
	}

	weatherTool := tool.MustDefine(
		"get_weather",
		"Ritorna il meteo di una città",
		core.RiskNone,
		func(ctx context.Context, in WeatherIn) (string, error) {
			return fmt.Sprintf("Meteo di %s: ...", in.City), nil
		},
	)

	runtime.WithTool(weatherTool)

	if err := tui.Run(runtime); err != nil {
		log.Fatal(err)
	}
}

func hookToolAudit(ctx context.Context, p *core.PostToolUsePayload) error {
	log.Printf("[AUDIT] post_tool_use: %s\n\n", p.ToolName)
	return nil
}
