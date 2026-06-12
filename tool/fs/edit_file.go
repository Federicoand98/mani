package fs

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Federicoand98/mani/core"
	"github.com/Federicoand98/mani/tool"
)

/*
 * EditFile
 *
 * Input Schema
 *
 * {
 * 	"path": "",
 * 	"old_content": "",
 * 	"new_content": ""
 * }
 */

type EditFileTool struct {
	Workspace
}

func NewEditFileTool(workspaceRoot string) *EditFileTool {
	return &EditFileTool{Workspace: NewWorkspace(workspaceRoot)}
}

// --------------------------------------------------
// Tool interface implementation --------------------
// --------------------------------------------------

func (t *EditFileTool) Name() string {
	return "edit_file"
}

func (t *EditFileTool) Description() string {
	return "Edits a file by replacing an old string with a new string. The file path should be relative to the workspace root."
}

func (t *EditFileTool) RiskLevel() core.RiskLevel {
	return core.RiskWrite
}

func (t *EditFileTool) Schema() tool.ToolSchema {
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
				"old_content": {
					Type:        "string",
					Description: "The old content of the file that needs to be replaced",
				},
				"new_content": {
					Type:        "string",
					Description: "The new content to replace the old content",
				},
			},
			Required: []string{"path", "old_content", "new_content"},
		},
	}
}

func (t *EditFileTool) Execute(ctx context.Context, input map[string]any) (string, error) {
	raw, ok := input["path"]
	if !ok {
		return "", fmt.Errorf("edit_file: missing required input 'path'")
	}

	path, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("edit_file: input 'path' should be a string")
	}

	rawOld, ok := input["old_content"]
	if !ok {
		return "", fmt.Errorf("edit_file: missing required input 'old_content'")
	}

	oldContent, ok := rawOld.(string)
	if !ok {
		return "", fmt.Errorf("edit_file: input 'old_content' should be a string")
	}

	if oldContent == "" {
		return "", fmt.Errorf("edit_file: input 'old_content' cannot be empty")
	}

	rawNew, ok := input["new_content"]
	if !ok {
		return "", fmt.Errorf("edit_file: missing required input 'new_content'")
	}

	newContent, ok := rawNew.(string)
	if !ok {
		return "", fmt.Errorf("edit_file: input 'new_content' should be a string")
	}

	safePath, err := t.Workspace.Resolve(path)
	if err != nil {
		return "", err
	}

	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("edit_file: context canceled: %w", err)
	}

	content, err := os.ReadFile(safePath)
	if err != nil {
		return "", fmt.Errorf("edit_file: failed to read file: %w", err)
	}

	contentStr := string(content)
	occurrences := strings.Count(contentStr, oldContent)
	if occurrences == 0 {
		return "", fmt.Errorf("edit_file: old content not found in file")
	}
	if occurrences > 1 {
		return "", fmt.Errorf("edit_file: old content is ambiguous; found %d occurrences", occurrences)
	}

	updated := strings.Replace(contentStr, oldContent, newContent, 1)

	info, err := os.Stat(safePath)
	if err != nil {
		return "", fmt.Errorf("edit_file: failed to stat file: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("edit_file: context canceled: %w", err)
	}

	if err := os.WriteFile(safePath, []byte(updated), info.Mode().Perm()); err != nil {
		return "", fmt.Errorf("edit_file: failed to write file: %w", err)
	}

	return fmt.Sprintf("edited file: %s", path), nil
}
