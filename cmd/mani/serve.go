package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/Federicoand98/mani/app"
	"github.com/Federicoand98/mani/server"
)

func runServer(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	configPath := fs.String("config", "", "path al manifest YAML")
	addr := fs.String("addr", ":9000", "indirizzo di ascolto")
	_ = fs.Parse(args)

	if *configPath == "" {
		return fmt.Errorf("serve: --config richiesto")
	}

	spec, err := app.LoadManifest(*configPath)
	if err != nil {
		return err
	}

	return server.New(spec).ListenAndServe(ctx, *addr)
}
