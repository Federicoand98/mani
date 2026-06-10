package fs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Federicoand98/mani/tool"
)

type ReadFileTool struct {
	workspaceRoot string
}

func NewReadFileTool(workspaceRoot string) *ReadFileTool {
	return &ReadFileTool{workspaceRoot: workspaceRoot}
}

// --------------------------------------------------
// Tool interface implementation --------------------
// --------------------------------------------------

func (t *ReadFileTool) Name() string {
	return "read_file"
}

func (t *ReadFileTool) Description() string {
	return "Reads the content of a file. The file path should be relative to the workspace root."
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

func (t *ReadFileTool) Execute(ctx context.Context, input map[string]interface{}) (string, error) {
	raw, ok := input["path"]
	if !ok {
		return "", fmt.Errorf("read_file: missing required input 'path'")
	}

	path, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("read_file: input 'path' should be a string")
	}

	safePath, err := t.safePath(path)
	if err != nil {
		return "", err
	}

	content, err := os.ReadFile(safePath)
	if err != nil {
		return "", fmt.Errorf("read_file: failed to read file: %w", err)
	}

	return string(content), nil
}

// --------------------------------------------------
// Private methods ----------------------------------
// --------------------------------------------------

func (t *ReadFileTool) safePath(path string) (string, error) {
	root, err := filepath.Abs(t.workspaceRoot)
	if err != nil {
		return "", fmt.Errorf("read_file: failed to resolve workspace root: %w", err)
	}

	// pulisce i ../../
	abs := filepath.Clean(filepath.Join(root, path))

	if abs != root && !strings.HasPrefix(abs, root+string(filepath.Separator)) {
		return "", fmt.Errorf("read_file: access to path '%s' is outside of the workspace", path)
	}

	if abs == root {
		return "", fmt.Errorf("read_file: path cannot be the workspace root")
	}

	return abs, nil
}
