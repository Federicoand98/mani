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
		fail(exitUsage, "config load", err)
	}

	arg := ""
	if len(os.Args) > 1 {
		arg = os.Args[1]
	}

	switch arg {
	case "-h", "--help", "help":
		usage(os.Stdout)
		return
	case "-v", "--version", "version":
		fmt.Println(versionString())
		return
	}

	if strings.HasPrefix(arg, "-") {
		fmt.Fprintf(os.Stderr, "mani: unknown flag %q\n\n", arg)
		usage(os.Stderr)
		os.Exit(exitUsage)
	}

	// destinazione log: serve → stderr; run → silenzioso (discard) salvo --verbose/--debug; TUI → file
	dest := "file"
	switch arg {
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

	if arg == "" {
		if err := runTUI(ctx, cfg); err != nil {
			fail(exitRuntime, "tui", err)
		}
		return
	}

	cmd, ok := lookupCommand(arg)
	if !ok {
		fmt.Fprintf(os.Stderr, "mani: unknown command %q\n\n", arg)
		usage(os.Stderr)
		os.Exit(exitUsage)
	}

	if err := cmd.run(ctx, os.Args[2:]); err != nil {
		fail(exitCodeFor(err), cmd.name, err)
	}
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
