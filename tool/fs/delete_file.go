package fs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Federicoand98/mani/core"
	"github.com/Federicoand98/mani/tool"
)

/*
 * {
 	"path": "path del file da eliminare relativo"
 * }
*/

type DeleteFileTool struct {
	workspaceRoot string
}

func NewDeleteFileTool(workspaceRoot string) *DeleteFileTool {
	return &DeleteFileTool{workspaceRoot: workspaceRoot}
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

	abs, err := t.safePath(path)
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

func (t *DeleteFileTool) safePath(path string) (string, error) {
	root, err := filepath.Abs(t.workspaceRoot)
	if err != nil {
		return "", fmt.Errorf("edit_file: failed to resolve workspace root: %w", err)
	}

	abs := filepath.Clean(filepath.Join(root, path))

	if abs != root && !strings.HasPrefix(abs, root+string(filepath.Separator)) {
		return "", fmt.Errorf("edit_file: access to path '%s' is outside of the workspace", path)
	}

	if abs == root {
		return "", fmt.Errorf("edit_file: path cannot be the workspace root")
	}

	return abs, nil
}
