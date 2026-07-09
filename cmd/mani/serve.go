package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/Federicoand98/mani/app"
	"github.com/Federicoand98/mani/server"
)

func runServer(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	configPath := fs.String("config", "", "path al manifest YAML")
	addr := fs.String("addr", ":9000", "indirizzo di ascolto")
	tokenFlag := fs.String("token", "", "bearer token")
	insecure := fs.Bool("insecure", false, "run without auth (dev-only)")
	_ = fs.Bool("verbose", false, "mostra i log (serve li mostra comunque)")
	_ = fs.Bool("debug", false, "alias di --verbose")
	_ = fs.Parse(args)

	if *configPath == "" {
		return fmt.Errorf("serve: --config richiesto")
	}

	token := *tokenFlag
	if token == "" {
		token = os.Getenv("MANI_SERVER_TOKEN")
	}

	if token == "" && !*insecure {
		return fmt.Errorf("serve: no token or --insecure flag set")
	}

	if token == "" {
		slog.Warn("agent server without AUTH (insecure, dev only)")
	}

	spec, err := app.LoadManifest(*configPath)
	if err != nil {
		return err
	}

	return server.New(spec, token).ListenAndServe(ctx, *addr)
}
