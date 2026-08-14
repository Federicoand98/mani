package fs

import (
	"context"
	"fmt"
	"os"

	"github.com/Federicoand98/mani/core"
	"github.com/Federicoand98/mani/tool"
)

/*
 * {
 	"path": "path del file da eliminare relativo"
 * }
*/

type DeleteFileTool struct {
	Workspace
}

func NewDeleteFileTool(workspaceRoot string) *DeleteFileTool {
	return &DeleteFileTool{Workspace: NewWorkspace(workspaceRoot)}
}

func (t *DeleteFileTool) Name() string {
	return "delete_file"
}

func (t *DeleteFileTool) Description() string {
	return "Deletes a file from the workspace given its path. Path is relative to the workspace root."
}

func (t *DeleteFileTool) RiskLevel() core.RiskLevel {
	return core.RiskWrite
}

func (t *DeleteFileTool) Schema() tool.ToolSchema {
	return tool.ToolSchema{
		Name:        t.Name(),
		Description: t.Description(),
		InputSchema: tool.InputSchema{
			Type: "object",
			Properties: map[string]tool.PropertySchema{
				"path": {
					Type:        "string",
					Description: "The path of the file to delete, relative to the workspace root.",
				},
			},
			Required: []string{"path"},
		},
	}
}

func (t *DeleteFileTool) Execute(ctx context.Context, input map[string]any) (string, error) {
	path, ok := input["path"].(string)
	if !ok {
		return "", fmt.Errorf("delete_file: path must be a string")
	}

	abs, err := t.Workspace.Resolve(path)
	if err != nil {
		return "", fmt.Errorf("delete_file: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("delete_file: context cancelled: %w", err)
	}

	if err := os.Remove(abs); err != nil {
		return "", fmt.Errorf("delete_file: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("delete_file: context cancelled: %w", err)
	}

	return fmt.Sprintf("File deleted successfully %s", path), nil
}
