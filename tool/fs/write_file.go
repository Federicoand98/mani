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
   "path":    "percorso file relativo alla workspace",
   "content": "contenuto completo del file"
 }
*/

type WriteFileTool struct {
	Workspace
}

func NewWriteFileTool(workspaceRoot string) *WriteFileTool {
	return &WriteFileTool{Workspace: NewWorkspace(workspaceRoot)}
}

func (t *WriteFileTool) Name() string {
	return "write_file"
}

func (t *WriteFileTool) Description() string {
	return "Write a new file given a path and content. The path is relative to the workspace root."
}

func (t *WriteFileTool) RiskLevel() core.RiskLevel {
	return core.RiskWrite
}

func (t *WriteFileTool) Schema() tool.ToolSchema {
	return tool.ToolSchema{
		Name:        t.Name(),
		Description: t.Description(),
		InputSchema: tool.InputSchema{
			Type: "object",
			Properties: map[string]tool.PropertySchema{
				"path": {
					Type:        "string",
					Description: "The path of the file to write, relative to the workspace root.",
				},
				"content": {
					Type:        "string",
					Description: "The content to write to the file.",
				},
			},
			Required: []string{"path", "content"},
		},
	}
}

func (t *WriteFileTool) Execute(ctx context.Context, input map[string]any) (string, error) {
	path, ok := input["path"].(string)
	if !ok {
		return "", fmt.Errorf("write_file: path must be a string")
	}

	content, ok := input["content"].(string)
	if !ok {
		return "", fmt.Errorf("write_file: content must be a string")
	}

	abs, err := t.Workspace.Resolve(path)
	if err != nil {
		return "", fmt.Errorf("write_file: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("write_file: context canceled: %w", err)
	}

	err = os.WriteFile(abs, []byte(content), 0o644)
	if err != nil {
		return "", fmt.Errorf("write_file: error writing file: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("write_file: context canceled: %w", err)
	}

	return fmt.Sprintf("File written to %s", path), nil
}
