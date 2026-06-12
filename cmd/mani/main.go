package main

import (
	"context"
	"os"

	"github.com/Federicoand98/mani/app"
	"github.com/Federicoand98/mani/cli"
	"github.com/Federicoand98/mani/config"
	"github.com/Federicoand98/mani/tool/bash"
	fstools "github.com/Federicoand98/mani/tool/fs"
)

func main() {
	ws, _ := os.Getwd()

	runtime := app.NewFromConfig(config.FromEnv()).
		WithTool(fstools.NewReadFileTool(ws)).
		WithTool(fstools.NewEditFileTool(ws)).
		WithTool(fstools.NewWriteFileTool(ws)).
		WithTool(fstools.NewDeleteFileTool(ws)).
		WithTool(bash.NewBashTool(ws))

	cli.New(runtime).Run(context.Background())
}
