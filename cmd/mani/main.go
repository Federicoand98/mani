package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/Federicoand98/mani/app"
	"github.com/Federicoand98/mani/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fail("config load", err)
	}

	sub := ""
	if len(os.Args) > 1 {
		sub = os.Args[1]
	}
	// destinazione log: serve → stderr; run → silenzioso (discard) salvo --verbose/--debug; TUI → file
	dest := "file"
	switch sub {
	case "serve":
		dest = "stderr"
	case "run":
		if hasFlag(os.Args, "verbose", "debug") {
			dest = "stderr"
		} else {
			dest = "discard"
		}
	}
	app.SetupLogging(cfg.LogLevel, dest)

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

// hasFlag: true se uno dei nomi compare tra gli args (accetta -x, --x, --x=val).
func hasFlag(args []string, names ...string) bool {
	for _, a := range args {
		a = strings.TrimLeft(a, "-")
		if i := strings.IndexByte(a, '='); i >= 0 {
			a = a[:i]
		}
		for _, n := range names {
			if a == n {
				return true
			}
		}
	}
	return false
}
