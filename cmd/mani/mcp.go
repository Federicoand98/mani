package main

import (
	"context"
	"flag"

	"github.com/Federicoand98/mani/manifest"
	"github.com/Federicoand98/mani/server/mcpserver"
)

func runMCP(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("mcp", flag.ExitOnError)
	configPath := fs.String("config", "", "path to the YAML manifest")
	_ = fs.Parse(args)

	if *configPath == "" {
		return usagef("--config is required")
	}

	spec, err := manifest.Load(*configPath)
	if err != nil {
		return usagef("%v", err)
	}

	srv, err := mcpserver.New(ctx, spec, versionString())
	if err != nil {
		return err
	}

	defer srv.Close()

	return srv.Run(ctx)
}
