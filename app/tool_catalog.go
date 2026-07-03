package app

import (
	"fmt"

	"github.com/Federicoand98/mani/tool"
	"github.com/Federicoand98/mani/tool/bash"
	"github.com/Federicoand98/mani/tool/fs"
)

// ToolDeps: dependencies for a tool that needs to be passed to the tool's constructor
type ToolDeps struct {
	Workspace string
}

// ToolConstructor: build a tool from the given dependencies
type ToolConstructor func(deps ToolDeps) (tool.Tool, error)

var toolContructors = map[string]ToolConstructor{
	"read":  func(deps ToolDeps) (tool.Tool, error) { return fs.NewReadFileTool(deps.Workspace), nil },
	"edit":  func(deps ToolDeps) (tool.Tool, error) { return fs.NewEditFileTool(deps.Workspace), nil },
	"write": func(deps ToolDeps) (tool.Tool, error) { return fs.NewWriteFileTool(deps.Workspace), nil },
	"bash":  func(deps ToolDeps) (tool.Tool, error) { return bash.NewBashTool(deps.Workspace), nil },
}

// RegisterToolConstructor registers a tool constructor for the given name
func RegisterToolConstructor(name string, c ToolConstructor) {
	toolContructors[name] = c
}

func buildTool(name string, deps ToolDeps) (tool.Tool, error) {
	c, ok := toolContructors[name]
	if !ok {
		return nil, fmt.Errorf("unknown tool: %q", name)
	}
	return c(deps)
}

func knownTool(name string) bool {
	_, ok := toolContructors[name]
	return ok
}
