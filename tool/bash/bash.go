package bash

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/Federicoand98/mani/core"
	"github.com/Federicoand98/mani/tool"
)

/*
 * {
 	"command": "",
 * }
*/

type BashTool struct {
	workspaceRoot string
}

func NewBashTool(workspaceRoot string) *BashTool {
	return &BashTool{workspaceRoot: workspaceRoot}
}

func (b *BashTool) Name() string {
	return "bash"
}

func (b *BashTool) Description() string {
	return "Executes bash commands in the workspace folder"
}

func (b *BashTool) RiskLevel() core.RiskLevel {
	return core.RiskExecute
}

func (b *BashTool) Schema() tool.ToolSchema {
	return tool.ToolSchema{
		Name:        b.Name(),
		Description: b.Description(),
		InputSchema: tool.InputSchema{
			Type: "object",
			Properties: map[string]tool.PropertySchema{
				"command": {
					Type:        "string",
					Description: "The bash command to execute",
				},
			},
			Required: []string{"command"},
		},
	}
}

func (b *BashTool) Execute(ctx context.Context, input map[string]any) (string, error) {
	command, ok := input["command"].(string)
	if !ok {
		return "", fmt.Errorf("bash: command must be a string")
	}

	// CDW in the workspace folder
	// 30 seconds timeout
	// Catch stdout and stderr and return them as a string concatenated
	// exit code != 0 ritorn output + "exit code N" come errore

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = b.workspaceRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("bash: %w", err)
	}

	return string(out), nil
}
