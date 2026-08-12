package app

import (
	"fmt"

	"github.com/Federicoand98/mani/core"
	"github.com/Federicoand98/mani/tool"
	"github.com/Federicoand98/mani/tool/bash"
	"github.com/Federicoand98/mani/tool/fs"
	"github.com/Federicoand98/mani/tool/subprocess"
)

// ToolDeps: dependencies for a tool that needs to be passed to the tool's constructor
type ToolDeps struct {
	Workspace string
	Runtime   *Runtime
	Subagents []SubagentSpec
	Depth     int
}

// ToolConstructor: build a tool from the given dependencies
type ToolConstructor func(deps ToolDeps) (tool.Tool, error)

var toolContructors = map[string]ToolConstructor{
	"read":  func(deps ToolDeps) (tool.Tool, error) { return fs.NewReadFileTool(deps.Workspace), nil },
	"edit":  func(deps ToolDeps) (tool.Tool, error) { return fs.NewEditFileTool(deps.Workspace), nil },
	"write": func(deps ToolDeps) (tool.Tool, error) { return fs.NewWriteFileTool(deps.Workspace), nil },
	"bash":  func(deps ToolDeps) (tool.Tool, error) { return bash.NewBashTool(deps.Workspace), nil },
	"planning": func(deps ToolDeps) (tool.Tool, error) {
		if deps.Runtime == nil {
			return nil, fmt.Errorf("[tool_catalog]: runtime is not available")
		}
		return newPlanTool(deps.Runtime), nil
	},
	"delegate": func(deps ToolDeps) (tool.Tool, error) {
		if deps.Runtime == nil {
			return nil, fmt.Errorf("[tool_catalog]: runtime is not available")
		}

		depth := deps.Depth
		if depth <= 0 {
			depth = 5
		}

		if len(deps.Subagents) == 0 {
			return newDelegateTool(deps.Runtime, depth), nil
		}

		names := make([]string, 0, len(deps.Subagents))
		for _, sa := range deps.Subagents {
			names = append(names, sa.Name)
		}

		return newNamedDelegateTool(deps.Runtime, depth, names), nil
	},
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

func buildToolRef(ref ToolRef, deps ToolDeps) (tool.Tool, error) {
	if !ref.isSubprocess() {
		return buildTool(ref.Name, deps)
	}

	risk := ref.Risk.toCore()
	if ref.Risk == "" {
		risk = core.RiskExecute
	}

	schema := tool.ToolSchema{
		Name:        ref.Name,
		Description: ref.Desc,
		InputSchema: ref.Schema,
	}

	return subprocess.New(ref.Name, ref.Desc, ref.Command, ref.Args, ref.Env, schema, risk, deps.Workspace), nil
}

func knownTool(name string) bool {
	_, ok := toolContructors[name]
	return ok
}
