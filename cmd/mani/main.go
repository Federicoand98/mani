package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"

	"github.com/Federicoand98/mani/app"
	"github.com/Federicoand98/mani/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config load", "err", err)
		os.Exit(1)
	}

	app.SetupLogging(cfg.LogLevel)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "run":
			if err := runFromManifest(ctx, os.Args[2:]); err != nil {
				slog.Error("run", "err", err)
				os.Exit(1)
			}
			return
		case "serve":
			if err := runServer(ctx, os.Args[2:]); err != nil {
				slog.Error("serve", "err", err)
				os.Exit(1)
			}
			return
		}
	}

	if err := runTUI(ctx, cfg); err != nil {
		slog.Error("tui", "err", err)
		os.Exit(1)
	}
}
