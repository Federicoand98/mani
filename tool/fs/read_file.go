package fs

import (
	"context"
	"fmt"
	"os"

	"github.com/Federicoand98/mani/core"
	"github.com/Federicoand98/mani/tool"
)

type ReadFileTool struct {
	Workspace
}

func NewReadFileTool(workspaceRoot string) *ReadFileTool {
	return &ReadFileTool{Workspace: NewWorkspace(workspaceRoot)}
}

// --------------------------------------------------
// Tool interface implementation --------------------
// --------------------------------------------------

func (t *ReadFileTool) Name() string {
	return "read"
}

func (t *ReadFileTool) Description() string {
	return "Reads the content of a file. The file path should be relative to the workspace root."
}

func (t *ReadFileTool) RiskLevel() core.RiskLevel {
	return core.RiskNone
}

func (t *ReadFileTool) Schema() tool.ToolSchema {
	return tool.ToolSchema{
		Name:        t.Name(),
		Description: t.Description(),
		InputSchema: tool.InputSchema{
			Type: "object",
			Properties: map[string]tool.PropertySchema{
				"path": {
					Type:        "string",
					Description: "Relative path to the file from the workspace root",
				},
			},
			Required: []string{"path"},
		},
	}
}

func (t *ReadFileTool) Execute(ctx context.Context, input map[string]any) (string, error) {
	raw, ok := input["path"]
	if !ok {
		return "", fmt.Errorf("read: missing required input 'path'")
	}

	path, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("read: input 'path' should be a string")
	}

	safePath, err := t.Workspace.Resolve(path)
	if err != nil {
		return "", err
	}

	content, err := os.ReadFile(safePath)
	if err != nil {
		return "", fmt.Errorf("read: failed to read file: %w", err)
	}

	return string(content), nil
}
