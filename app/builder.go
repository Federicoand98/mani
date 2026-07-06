package app

import (
	"context"
	"fmt"
	"os"

	"github.com/Federicoand98/mani/config"
	"github.com/Federicoand98/mani/core"
	"github.com/Federicoand98/mani/tool/mcp"
)

// Build builds a new Runtime from the given RuntimeSpec.
func Build(ctx context.Context, spec RuntimeSpec) (*Runtime, error) {
	base, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("build: config: %w", err)
	}

	cfg := mergeConfig(base, spec)

	ws := spec.Workspace
	if ws == "" {
		ws, _ = os.Getwd()
	}

	deps := ToolDeps{Workspace: ws}

	rt := NewFromConfig(cfg)

	// 1. Load tools
	for _, name := range spec.Tools {
		t, err := buildTool(name, deps)
		if err != nil {
			return nil, fmt.Errorf("build: tool %q: %w", name, err)
		}
		rt.WithTool(t)
	}

	// 2. Load permissions. default allow, override with spec
	rt.permission = NewPermissionManager()
	rt.agent.AddPreToolUseHook(manifestPolicyHook(spec.Permissions, rt.permission))

	// 3. Features
	f := spec.Features
	if f.ContextInjection {
		registerContextInjectionWith(rt, spec.SystemPrompt, ws)
	}
	if f.Compaction.Enabled {
		RegisterTrimCompaction(rt, f.Compaction.Keep)
	}
	if f.Planning {
		RegisterPlanning(rt)
	}
	if f.Subagents.Enabled {
		registerSubagentsFromSpec(rt, spec.Subagents, f.Subagents.Depth, deps)
	}
	if f.Tracing {
		RegisterTracing(rt)
	}

	// 4. MCP
	for _, m := range spec.MCPServers {
		if err := rt.AddMCPServer(ctx, mcp.ServerSpec{
			Name:    m.Name,
			Command: m.Command,
			Args:    m.Args,
			URL:     m.URL,
		}); err != nil {
			return nil, fmt.Errorf("build: mcp %q: %w", m.Name, err)
		}
	}

	return rt, nil
}

// mergeConfig merges spec > base config.
func mergeConfig(base config.Config, spec RuntimeSpec) config.Config {
	c := base
	if spec.Provider != "" {
		c.Provider = spec.Provider
	}
	if spec.Model != "" {
		c.SetActiveModel(spec.Model)
	}
	if spec.ContextWindow != 0 {
		c.ContextWindow = spec.ContextWindow
	}
	if spec.MaxIterations != 0 {
		c.MaxIterations = spec.MaxIterations
	}
	return c
}

func manifestPolicyHook(policy map[string]RiskPolicy, mgr *PermissionManager) core.PreToolUseHook {
	return func(toolName string, level core.RiskLevel, input map[string]any) error {
		switch resolvePolicy(policy, toolName) {
		case RiskPolicyDeny:
			return fmt.Errorf("permission: tool %q denied", toolName)
		case RiskPolicyAsk:
			return mgr.check(toolName, level, input)
		default:
			return nil
		}
	}
}

func resolvePolicy(policy map[string]RiskPolicy, name string) RiskPolicy {
	if p, ok := policy[name]; ok {
		return p
	}
	if d, has := policy["default"]; has {
		return d
	}
	return RiskPolicyAllow
}
