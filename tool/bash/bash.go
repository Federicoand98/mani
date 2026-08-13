// Package bash provides the bash tool: runs a shell command in the workspace.
//
// It declares RiskExecute, so the permission layer gates it before execution.
package bash

import (
	"context"
	"errors"
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
	shell         shell
}

func NewBashTool(workspaceRoot string) *BashTool {
	return &BashTool{workspaceRoot: workspaceRoot, shell: detectShell()}
}

func (b *BashTool) Name() string {
	return "bash"
}

func (b *BashTool) Description() string {
	return fmt.Sprintf(
		"Executes a shell command in the workspace folder. The shell is %s - write commands in that dialect.",
		b.shell.dialect,
	)
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

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = b.workspaceRoot

	cmd.WaitDelay = 100 * time.Millisecond

	out, err := cmd.CombinedOutput()

	if ctx.Err() != nil {
		return "", fmt.Errorf("bash: %w", ctx.Err())
	}

	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return fmt.Sprintf("exit status %d\n%s", ee.ExitCode(), out), nil
	}

	if errors.Is(err, exec.ErrWaitDelay) {
		return string(out) + "\n(note: a background process is still running, output may be incomplete)", nil
	}

	if err != nil {
		return "", fmt.Errorf("bash: %w", err)
	}

	return string(out), nil
}
