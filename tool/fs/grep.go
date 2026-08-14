package fs

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/Federicoand98/mani/core"
	"github.com/Federicoand98/mani/tool"
)

type GrepTool struct {
	Workspace
}

func NewGrepTool(workspaceRoot string) *GrepTool {
	return &GrepTool{Workspace: NewWorkspace(workspaceRoot)}
}

func (t *GrepTool) Name() string              { return "grep" }
func (t *GrepTool) RiskLevel() core.RiskLevel { return core.RiskNone }
func (t *GrepTool) Description() string {
	return "Searches a regular expression (RE2 syntax) across workspace files and returns matching lines " +
		"as \"path:line:text\". Optional 'path' narrows the search to a subdirectory, " +
		"optional 'glob' to matching filenames (e.g. \"**/*.go\"). Results are capped."
}

func (t *GrepTool) Schema() tool.ToolSchema {
	return tool.ToolSchema{
		Name:        t.Name(),
		Description: t.Description(),
		InputSchema: tool.InputSchema{
			Type: "object",
			Properties: map[string]tool.PropertySchema{
				"pattern": {
					Type:        "string",
					Description: "Regular expression to search for (RE2 syntax: no backreferences and no lookahead)",
				},
				"path": {
					Type:        "string",
					Description: "Optional subdirectory to search in, relative to the workspace root",
				},
				"glob": {
					Type:        "string",
					Description: "Optional glob restricting which files are searched, e.g. \"**/*.go\"",
				},
			},
			Required: []string{"pattern"},
		},
	}
}

func (t *GrepTool) Execute(ctx context.Context, input map[string]any) (string, error) {
	raw, ok := input["pattern"]
	if !ok {
		return "", fmt.Errorf("grep: missing required input 'pattern'")
	}
	pattern, ok := raw.(string)
	if !ok || pattern == "" {
		return "", fmt.Errorf("grep: input 'pattern' should be a non-empty string")
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", fmt.Errorf("grep: invalid regular expression %q: %w", pattern, err)
	}

	sub, _ := input["path"].(string)
	sub = strings.TrimPrefix(strings.TrimSpace(sub), "./")

	var nameRe *regexp.Regexp
	if g, _ := input["glob"].(string); g != "" {
		nameRe, err = globToRegexp(g)
		if err != nil {
			return "", fmt.Errorf("grep: invalid glob %q: %w", g, err)
		}
	}

	root, err := t.Workspace.Root()
	if err != nil {
		return "", err
	}

	if sub != "" && sub != "." {
		if _, err := t.Workspace.Resolve(sub); err != nil {
			return "", err
		}
	} else {
		sub = ""
	}

	var out strings.Builder
	matches, filesSearched := 0, 0
	truncated := false

	err = walkFiles(ctx, root, sub, func(rel, abs string) (bool, error) {
		if nameRe != nil && !nameRe.MatchString(rel) {
			return true, nil
		}
		content, ok := readTextFile(abs)
		if !ok {
			return true, nil
		}
		filesSearched++

		for i, line := range strings.Split(content, "\n") {
			if !re.MatchString(line) {
				continue
			}
			if matches >= maxGrepMatches {
				truncated = true
				return false, nil
			}
			fmt.Fprintf(&out, "%s:%d:%s\n", rel, i+1, truncateLine(line))
			matches++
		}
		return true, nil
	})
	if err != nil {
		return "", fmt.Errorf("grep: %w", err)
	}

	if matches == 0 {
		return fmt.Sprintf("no matches for %q (searched %d files)", pattern, filesSearched), nil
	}

	if truncated {
		fmt.Fprintf(&out, "\n... truncated at %d matches. Narrow the search with 'path' or 'glob'.\n", maxGrepMatches)
	}
	return out.String(), nil
}
