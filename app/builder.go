package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/Federicoand98/mani/config"
	"github.com/Federicoand98/mani/core"
	"github.com/Federicoand98/mani/tool"
	"github.com/Federicoand98/mani/tool/mcp"
)

type DaemonOption func(*daemonOptions)

type daemonOptions struct {
	insecure bool
}

// Build builds a new Runtime from the given RuntimeSpec.
func Build(ctx context.Context, spec RuntimeSpec) (*Runtime, error) {
	base, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("build: config: %w", err)
	}

	cfg := mergeConfig(base, spec)

	ws := spec.Capabilities.Workspace
	if ws == "" {
		ws, _ = os.Getwd()
	}

	rt := NewFromConfig(cfg)

	if err := rt.ClientErr(); err != nil {
		return nil, fmt.Errorf("[builder]: identity.provider %q: %w", spec.Identity.Provider, err)
	}

	// i subagent vanno noti PRIMA di costruire il tool delegate (gli servono i nomi per l'enum)
	if len(spec.Capabilities.Subagents) > 0 {
		rt.subagents = make(map[string]SubagentSpec, len(spec.Capabilities.Subagents))
		for _, sa := range spec.Capabilities.Subagents {
			rt.subagents[sa.Name] = sa
		}
	}

	deps := ToolDeps{
		Workspace:   ws,
		Runtime:     rt,
		Subagents:   spec.Capabilities.Subagents,
		Depth:       spec.Limits.SubagentDepth,
		HostAllowed: hostAllowedFunc(spec.Policy.Network),
	}

	// 1. tool
	toolTimeout := 60 * time.Second
	if spec.Limits.ToolTimeout != "" {
		d, err := time.ParseDuration(spec.Limits.ToolTimeout)
		if err != nil {
			return nil, fmt.Errorf("build: limits.tool_timeout: %w", err)
		}
		toolTimeout = d
	}

	for _, ref := range spec.Capabilities.Tools {
		t, err := buildToolRef(ref, deps)
		if err != nil {
			return nil, fmt.Errorf("build: tool %q: %w", ref.Name, err)
		}
		if toolTimeout > 0 {
			t = withToolTimeout(t, toolTimeout)
		}
		rt.WithTool(t)
	}

	// 2. policy
	rt.permission = NewPermissionManager()
	rt.agent.AddPreToolUseHook(manifestPolicyHook(spec.Policy.Tools, rt.permission, rt))

	// 3. output schema (terminal tool `respond`)
	sysPrompt := spec.Identity.Prompt
	if spec.Output.Schema.Type != "" {
		schema := tool.ToolSchema{
			Name:        "respond",
			Description: "Return the final result according to the schema. Use this tool once, only at the end.",
			InputSchema: spec.Output.Schema,
		}
		rt.WithTool(tool.New(schema, core.RiskNone, func(context.Context, map[string]any) (string, error) {
			return "", nil
		}))
		rt.agent.SetFinalTool("respond")
		sysPrompt += "\n\nThis agent has an output schema: finish ALWAYS using the `respond` tool with an object matching the schema. Do not respond with plain text."
	}

	// 1. observability
	if spec.Observability.Tracing {
		RegisterTracing(rt)
	}

	if spec.Observability.Journal.Enabled {
		j, err := buildJournal(spec.Observability.Journal)
		if err != nil {
			return nil, fmt.Errorf("build: journal: %w", err)
		}
		RegisterJournal(rt, j)
	}

	// 2. context
	if spec.Context.Inject {
		registerContextInjectionWith(rt, sysPrompt, ws)
	}

	if spec.Context.Compaction.Enabled {
		RegisterTrimCompaction(rt, spec.Context.Compaction.Keep)
	}

	// 3. policy
	if len(spec.Policy.Rules) > 0 || len(spec.Policy.Redact) > 0 {
		RegisterPolicyRules(rt, spec.Policy)
	}

	if len(spec.Policy.Network.Allow) > 0 || len(spec.Policy.Network.Deny) > 0 {
		RegisterNetworkPolicy(rt, spec.Policy.Network)
	}

	// 4. limits
	if spec.Limits.MaxTokens > 0 || spec.Limits.MaxToolCalls > 0 {
		RegisterBudget(rt, spec.Limits)
	}
	if spec.Limits.MaxDuration != "" {
		rt.maxDuration, _ = time.ParseDuration(spec.Limits.MaxDuration)
	}

	// 8. MCP
	for _, m := range spec.Capabilities.MCP {
		if err := rt.AddMCPServer(ctx, mcp.ServerSpec{
			Name: m.Name, Command: m.Command, Args: m.Args, URL: m.URL,
		}); err != nil {
			return nil, fmt.Errorf("build: mcp %q: %w", m.Name, err)
		}
	}

	return rt, nil
}

func AllowInsecureWebhook() DaemonOption {
	return func(o *daemonOptions) {
		o.insecure = true
	}
}

func BuildDaemon(rt *Runtime, spec RuntimeSpec, opts ...DaemonOption) (*Daemon, error) {
	var o daemonOptions
	for _, opt := range opts {
		opt(&o)
	}

	q, err := buildQueue(spec.Run.Scheduler)
	if err != nil {
		return nil, fmt.Errorf("build: queue: %w", err)
	}

	d := NewTrigger(rt, q)
	d.concurrency = spec.Run.Scheduler.Concurrency

	// il backoff arriva dal YAML come stringa: qui lo parsiamo una volta sola
	r := resolveRetry(spec.Run.Scheduler.Retry)
	backoff, err := time.ParseDuration(r.Backoff)
	if err != nil {
		return nil, fmt.Errorf("build: queue.retry.backoff %q: %w", r.Backoff, err)
	}
	d.maxAttempts = r.MaxAttempts
	d.backoff = backoff
	if spec.Run.Scheduler.Path != "" {
		d.state = loadTriggerState(filepath.Join(spec.Run.Scheduler.Path, "triggers.json"))
	} else {
		d.state = loadTriggerState("") // in RAM: nessun catch-up tra riavvii
	}

	for _, t := range spec.Run.Triggers {
		id := triggerID(t)
		switch t.Type {
		case "every":
			dur, err := time.ParseDuration(t.Every)
			if err != nil {
				return nil, fmt.Errorf("build: invalid duration %q: %w", t.Every, err)
			}
			d.Every(id, dur, t.Prompt, t.Memory)
		case "daily":
			if _, _, err := parseClock(t.At); err != nil {
				return nil, fmt.Errorf("build: trigger %q: %w", id, err)
			}
			d.Daily(id, t.At, t.Prompt, t.Memory, t.CatchUp)
		case "webhook":
			token := os.Getenv("MANI_WEBHOOK_TOKEN")
			if token == "" && !o.insecure {
				return nil, fmt.Errorf("build: webhook trigger %q: MANI_WEBHOOK_TOKEN not set (or pass --insecure to run without authentication)", id)
			}
			if token == "" {
				slog.Warn("[daemon]: webhook trigger without authentication (insecure, dev only)", "trigger", id, "addr", t.Addr)
			}
			d.Webhook(t.Addr, t.Prompt, t.Memory, token)
		default:
			return nil, fmt.Errorf("build: invalid trigger type %q", t.Type)
		}
	}
	return d, nil
}

func buildQueue(spec SchedulerSpec) (TaskQueue, error) {
	if spec.Path == "" {
		return NewInMemoryQueue(spec.MaxPending), nil
	}
	return NewFileQueue(spec.Path, spec.MaxPending)
}

func resolveRetry(r RetrySpec) RetrySpec {
	if r.MaxAttempts <= 0 {
		r.MaxAttempts = 3
	}
	if r.Backoff == "" {
		r.Backoff = "30s"
	}
	return r
}

// mergeConfig merges spec > base config.
func mergeConfig(base config.Config, spec RuntimeSpec) config.Config {
	c := base
	if spec.Identity.Provider != "" {
		c.Provider = spec.Identity.Provider
	}
	if spec.Identity.Model != "" {
		c.SetActiveModel(spec.Identity.Model)
	}
	if spec.Context.Window != 0 {
		c.ContextWindow = spec.Context.Window
	}
	if spec.Limits.MaxIterations != 0 {
		c.MaxIterations = spec.Limits.MaxIterations
	}
	return c
}

func manifestPolicyHook(policy map[string]RiskPolicy, mgr *PermissionManager, rt *Runtime) core.PreToolUseHook {
	return func(ctx context.Context, toolName string, level core.RiskLevel, input map[string]any) error {
		switch resolvePolicy(policy, toolName) {
		case RiskPolicyDeny:
			rt.recordGovernance(ctx, "deny", toolName, "permission")
			return fmt.Errorf("permission: tool %q denied", toolName)
		case RiskPolicyAsk:
			return mgr.check(ctx, toolName, level, input)
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

func buildJournal(spec JournalSpec) (Journal, error) {
	retention := spec.Retention
	if retention <= 0 {
		retention = 100
	}
	mem := NewInMemoryJournal(retention)

	if spec.Path == "" {
		return mem, nil // solo RAM
	}

	disk, err := NewJSONLJournal(spec.Path)
	if err != nil {
		return nil, err
	}
	// InMemory primo → Get/List del daemon/CLI leggono dalla RAM.
	return NewMultiJournal(mem, disk), nil
}
