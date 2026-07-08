package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/Federicoand98/mani/app"
	"github.com/Federicoand98/mani/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fail("config load", err)
	}

	// headless (run/serve): log su stderr → visibili nel terminale. TUI: log su file.
	headless := len(os.Args) > 1 && (os.Args[1] == "run" || os.Args[1] == "serve")
	app.SetupLogging(cfg.LogLevel, headless)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "run":
			if err := runFromManifest(ctx, os.Args[2:]); err != nil {
				fail("run", err)
			}
			return
		case "serve":
			if err := runServer(ctx, os.Args[2:]); err != nil {
				fail("serve", err)
			}
			return
		}
	}

	if err := runTUI(ctx, cfg); err != nil {
		fail("tui", err)
	}
}

// fail stampa l'errore su stderr (visibile nel terminale, non solo nel log slog
// che è rediretto su file) ed esce con status 1.
func fail(context string, err error) {
	fmt.Fprintf(os.Stderr, "mani %s: %v\n", context, err)
	os.Exit(1)
}
