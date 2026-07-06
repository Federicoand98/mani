package app

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type RiskPolicy string

const (
	RiskPolicyAllow RiskPolicy = "allow"
	RiskPolicyDeny  RiskPolicy = "deny"
	RiskPolicyAsk   RiskPolicy = "ask"
)

type CompactionCfg struct {
	Enabled bool `json:"enabled"`
	Keep    int  `json:"keep"`
}

type SubagentsCfg struct {
	Enabled bool `json:"enabled"`
	Depth   int  `json:"depth"`
}

// Features: enabled/disabled middleware features
// default: all features enabled
type Features struct {
	Planning         bool          `json:"planning"`
	ContextInjection bool          `json:"context_injection"`
	Tracing          bool          `json:"tracing"`
	Compaction       CompactionCfg `json:"compaction"`
	Subagents        SubagentsCfg  `json:"subagents"`
}

// MCPSpec: MCP server specification
type MCPSpec struct {
	Name    string   `json:"name"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
	URL     string   `json:"url"`
}

// TriggerSpec: trigger specification. Trigger + prompt
type TriggerSpec struct {
	Type   string `json:"type"`  // trigger type: "every", "at", "every"
	Every  string `json:"every"` // es 30m
	At     string `json:"at"`    // es 09:00
	Addr   string `json:"addr"`  // es :8787
	Prompt string `json:"prompt"`
}

// SubagentSpec: Runtime spec for subagents. No trigger, no inner subagents. empty tool = inherit
type SubagentSpec struct {
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	SystemPrompt  string   `json:"system_prompt"`
	Model         string   `json:"model"` // "" = inherit
	Tools         []string `json:"tools"`
	MaxIterations int      `json:"max_iterations"` // 0 = inherit
}

type RuntimeSpec struct {
	Provider      string                `json:"provider"`
	Model         string                `json:"model"`
	SystemPrompt  string                `json:"system_prompt"`
	Workspace     string                `json:"workspace"`
	Tools         []string              `json:"tools"`
	Features      Features              `json:"features"`
	Permissions   map[string]RiskPolicy `json:"permissions"`
	MCPServers    []MCPSpec             `json:"mcpservers"`
	Triggers      []TriggerSpec         `json:"triggers"`
	Subagents     []SubagentSpec        `json:"subagents"`
	ContextWindow int                   `json:"context_window"`
	MaxIterations int                   `json:"max_iterations"`
}

func DefaultSpec() RuntimeSpec {
	return RuntimeSpec{
		Features: Features{
			Planning:         true,
			ContextInjection: true,
			Tracing:          true,
			Compaction:       CompactionCfg{Enabled: true, Keep: 20},
			Subagents:        SubagentsCfg{Enabled: true, Depth: 5},
		},
	}
}

func LoadManifest(path string) (RuntimeSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return RuntimeSpec{}, fmt.Errorf("manifest: read %s: %w", path, err)
	}

	spec := DefaultSpec()
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return RuntimeSpec{}, fmt.Errorf("manifest: unmarshal %s: %w", path, err)
	}

	if err := spec.Validate(); err != nil {
		return RuntimeSpec{}, fmt.Errorf("manifest: validate %s: %w", path, err)
	}

	return spec, nil
}

func (s RuntimeSpec) Validate() error {
	for _, name := range s.Tools {
		if !knownTool(name) {
			return fmt.Errorf("manifest: unknown tool %s", name)
		}
	}

	for name, p := range s.Permissions {
		switch p {
		case RiskPolicyAllow, RiskPolicyAsk, RiskPolicyDeny:
		default:
			return fmt.Errorf("manifest: unknown permission %s", name)
		}
	}

	seen := map[string]bool{}
	for _, sa := range s.Subagents {
		if sa.Name == "" {
			return fmt.Errorf("manifest: subagent name required")
		}

		if sa.Description == "" {
			return fmt.Errorf("manifest: subagent description required")
		}

		seen[sa.Name] = true

		for _, t := range sa.Tools {
			if !knownTool(t) {
				return fmt.Errorf("manifest: unknown tool %s", t)
			}
		}
	}

	for _, t := range s.Triggers {
		switch t.Type {
		case "every":
			if t.Every == "" {
				return fmt.Errorf("trigger every: campo 'every' richiesto")
			}
			if _, err := time.ParseDuration(t.Every); err != nil {
				return fmt.Errorf("trigger every: durata %q non valida: %w", t.Every, err)
			}
			if t.Prompt == "" {
				return fmt.Errorf("trigger every: 'prompt' richiesto")
			}
		case "daily":
			if t.At == "" {
				return fmt.Errorf("trigger daily: campo 'at' richiesto")
			}
			if t.Prompt == "" {
				return fmt.Errorf("trigger daily: 'prompt' richiesto")
			}
		case "webhook":
			if t.Addr == "" {
				return fmt.Errorf("trigger webhook: campo 'addr' richiesto")
			}
			// prompt opzionale (template)
		default:
			return fmt.Errorf("trigger type sconosciuto: %q", t.Type)
		}
	}

	return nil
}
