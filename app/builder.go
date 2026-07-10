package app

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/Federicoand98/mani/config"
	"github.com/Federicoand98/mani/core"
	"github.com/Federicoand98/mani/tool"
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

	var perToolTimeout time.Duration
	if spec.Budget.PerToolTimeout != "" {
		perToolTimeout, _ = time.ParseDuration(spec.Budget.PerToolTimeout)
	}

	for _, name := range spec.Tools {
		t, err := buildToolRef(name, deps)
		if err != nil {
			return nil, fmt.Errorf("build: tool %q: %w", name, err)
		}

		if perToolTimeout > 0 {
			t = withToolTimeout(t, perToolTimeout)
		}

		rt.WithTool(t)
	}

	// 2. Load permissions. default allow, override with spec
	rt.permission = NewPermissionManager()
	rt.agent.AddPreToolUseHook(manifestPolicyHook(spec.Permissions, rt.permission))

	sysPrompt := spec.SystemPrompt
	if spec.OutputSchema.Type != "" {
		schema := tool.ToolSchema{
			Name:        "respond",
			Description: "Return the final result according to the schema. Use this tool once, only at the end.",
			InputSchema: spec.OutputSchema,
		}

		rt.WithTool(tool.New(schema, core.RiskNone, func(ctx context.Context, m map[string]any) (string, error) {
			return "", nil
		}))

		rt.agent.SetFinalTool("respond")

		sysPrompt += "\n\nThis agent has an output schema: finish ALWAYS using the `respond` tool with an object matching the schema. Do not respond with plain text."
	}

	// 3. Features
	f := spec.Features
	if f.ContextInjection {
		registerContextInjectionWith(rt, sysPrompt, ws)
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
	if len(spec.Guardrails.Deny) > 0 || len(spec.Guardrails.Mask) > 0 {
		RegisterGuardrails(rt, spec.Guardrails)
	}
	if spec.Budget.MaxTokens > 0 || spec.Budget.MaxToolCalls > 0 {
		RegisterBudget(rt, spec.Budget)
	}
	if spec.Budget.MaxDuration != "" {
		rt.maxDuration, _ = time.ParseDuration(spec.Budget.MaxDuration)
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

func BuildDaemon(rt *Runtime, specs []TriggerSpec) (*Daemon, error) {
	d := NewTrigger(rt)
	for _, t := range specs {
		switch t.Type {
		case "every":
			dur, err := time.ParseDuration(t.Every)
			if err != nil {
				return nil, fmt.Errorf("build: invalid duration %q: %w", t.Every, err)
			}
			d.Every(dur, t.Prompt)
		case "daily":
			d.Daily(t.At, t.Prompt)
		case "webhook":
			d.Webhook(t.Addr, t.Prompt)
		default:
			return nil, fmt.Errorf("build: invalid trigger type %q", t.Type)
		}
	}
	return d, nil
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
