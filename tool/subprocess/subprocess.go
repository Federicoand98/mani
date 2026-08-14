// Package subprocess turns any executable into a tool.
//
// The tool input is written as JSON on stdin and the result is read from stdout; a
// non-zero exit turns stderr into the error the model sees. It is the lightweight
// tier next to MCP: a five-line script becomes a governed tool.
package subprocess

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Federicoand98/mani/core"
	"github.com/Federicoand98/mani/tool"
)

type procTool struct {
	name      string
	desc      string
	command   string
	args      []string
	env       map[string]string
	schema    tool.ToolSchema
	risk      core.RiskLevel
	workspace string
}

func New(name, desc, command string, args []string, env map[string]string, schema tool.ToolSchema, risk core.RiskLevel, workspace string) tool.Tool {
	return &procTool{
		name:      name,
		desc:      desc,
		command:   command,
		args:      args,
		env:       env,
		schema:    schema,
		risk:      risk,
		workspace: workspace,
	}
}

func (t *procTool) Name() string              { return t.name }
func (t *procTool) Description() string       { return t.desc }
func (t *procTool) Schema() tool.ToolSchema   { return t.schema }
func (t *procTool) RiskLevel() core.RiskLevel { return t.risk }

func (t *procTool) Execute(ctx context.Context, input map[string]any) (string, error) {
	payload, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("tool %q: marshall input: %w", t.name, err)
	}

	cmd := exec.CommandContext(ctx, t.resolveCommand(), t.args...)
	cmd.Dir = t.workspace
	cmd.Env = t.buildEnv()
	cmd.Stdin = bytes.NewReader(payload)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("tool %q: %s", t.name, msg)
	}
	return stdout.String(), nil
}

// resolveCommand resolve a relative command path to a workspace path.
func (t *procTool) resolveCommand() string {
	if strings.ContainsRune(t.command, os.PathSeparator) && !filepath.IsAbs(t.command) {
		return filepath.Join(t.workspace, t.command)
	}
	return t.command
}

func (t *procTool) buildEnv() []string {
	if len(t.env) == 0 {
		return nil
	}
	env := os.Environ()
	for k, v := range t.env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	return env
}
