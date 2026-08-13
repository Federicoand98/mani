package fs

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/Federicoand98/mani/core"
	"github.com/Federicoand98/mani/tool"
)

type GlobTool struct {
	Workspace
}

func NewGlobTool(workspaceRoot string) *GlobTool {
	return &GlobTool{Workspace: Workspace{root: workspaceRoot}}
}

func (g *GlobTool) Name() string              { return "glob" }
func (g *GlobTool) RiskLevel() core.RiskLevel { return core.RiskNone }
func (g *GlobTool) Description() string {
	return "Lists workspace files matching a glob pattern. " +
		"Use * for any character within one path segment, ** for any depth, ? for one character." +
		"Examples: \"*.md\", \"**/*.go/, \"cmd/**\". Returns paths relative to the workspace root."
}

func (g *GlobTool) Schema() tool.ToolSchema {
	return tool.ToolSchema{
		Name:        g.Name(),
		Description: g.Description(),
		InputSchema: tool.InputSchema{
			Type: "object",
			Properties: map[string]tool.PropertySchema{
				"pattern": {
					Type:        "string",
					Description: "Glob pattern to match against paths relative to the workspace root",
				},
			},
			Required: []string{"pattern"},
		},
	}
}

func (t *GlobTool) Execute(ctx context.Context, input map[string]any) (string, error) {
	raw, ok := input["pattern"]
	if !ok {
		return "", fmt.Errorf("glob: missing required input 'pattern'")
	}
	pattern, ok := raw.(string)
	if !ok || pattern == "" {
		return "", fmt.Errorf("glob: input 'pattern' should be a non-empty string")
	}

	re, err := globToRegexp(pattern)
	if err != nil {
		return "", fmt.Errorf("glob: invalid pattern %q: %w", pattern, err)
	}

	root, err := t.Workspace.Root()
	if err != nil {
		return "", err
	}

	var found []string
	truncated := false

	err = walkFiles(ctx, root, "", func(rel, _ string) (bool, error) {
		if !re.MatchString(rel) {
			return true, nil
		}
		if len(found) >= maxGlobResults {
			truncated = true
			return false, nil
		}
		found = append(found, rel)
		return true, nil
	})
	if err != nil {
		return "", fmt.Errorf("glob: %w", err)
	}

	if len(found) == 0 {
		return fmt.Sprintf("no files match %q", pattern), nil
	}

	sort.Strings(found)
	out := strings.Join(found, "\n")
	if truncated {
		out += fmt.Sprintf("\n\n... (truncated at %d files: narrow the pattern)", maxGlobResults)
	}
	return out, nil
}
