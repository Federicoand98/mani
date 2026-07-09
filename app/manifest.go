package app

import (
	"fmt"
	"os"
	"time"

	"github.com/Federicoand98/mani/core"
	"github.com/Federicoand98/mani/tool"
	"gopkg.in/yaml.v3"
)

type (
	RiskPolicy string
	RiskName   string
)

const (
	RiskPolicyAllow RiskPolicy = "allow"
	RiskPolicyDeny  RiskPolicy = "deny"
	RiskPolicyAsk   RiskPolicy = "ask"
)

type CompactionCfg struct {
	Enabled bool `yaml:"enabled"`
	Keep    int  `yaml:"keep"`
}

type SubagentsCfg struct {
	Enabled bool `yaml:"enabled"`
	Depth   int  `yaml:"depth"`
}

// Features: enabled/disabled middleware features
// default: all features enabled
type Features struct {
	Planning         bool          `yaml:"planning"`
	ContextInjection bool          `yaml:"context_injection"`
	Tracing          bool          `yaml:"tracing"`
	Compaction       CompactionCfg `yaml:"compaction"`
	Subagents        SubagentsCfg  `yaml:"subagents"`
}

// MCPSpec: MCP server specification
type MCPSpec struct {
	Name    string   `yaml:"name"`
	Command string   `yaml:"command"`
	Args    []string `yaml:"args"`
	URL     string   `yaml:"url"`
}

// TriggerSpec: trigger specification. Trigger + prompt
type TriggerSpec struct {
	Type   string `yaml:"type"`  // trigger type: "every", "at", "every"
	Every  string `yaml:"every"` // es 30m
	At     string `yaml:"at"`    // es 09:00
	Addr   string `yaml:"addr"`  // es :8787
	Prompt string `yaml:"prompt"`
}

// SubagentSpec: Runtime spec for subagents. No trigger, no inner subagents. empty tool = inherit
type SubagentSpec struct {
	Name          string   `yaml:"name"`
	Description   string   `yaml:"description"`
	SystemPrompt  string   `yaml:"system_prompt"`
	Model         string   `yaml:"model"` // "" = inherit
	Tools         []string `yaml:"tools"`
	MaxIterations int      `yaml:"max_iterations"` // 0 = inherit
}

type ToolRef struct {
	Name    string            `yaml:"name"`
	Desc    string            `yaml:"description"`
	Command string            `yaml:"command"`
	Args    []string          `yaml:"args"`
	Env     map[string]string `yaml:"env"`
	Schema  tool.InputSchema  `yaml:"schema"`
	Risk    RiskName          `yaml:"risk"`
}

type RuntimeSpec struct {
	Provider      string                `yaml:"provider"`
	Model         string                `yaml:"model"`
	SystemPrompt  string                `yaml:"system_prompt"`
	Workspace     string                `yaml:"workspace"`
	Tools         []ToolRef             `yaml:"tools"`
	Features      Features              `yaml:"features"`
	Permissions   map[string]RiskPolicy `yaml:"permissions"`
	MCPServers    []MCPSpec             `yaml:"mcpservers"`
	Triggers      []TriggerSpec         `yaml:"triggers"`
	Subagents     []SubagentSpec        `yaml:"subagents"`
	OutputSchema  tool.InputSchema      `yaml:"output_schema"`
	ContextWindow int                   `yaml:"context_window"`
	MaxIterations int                   `yaml:"max_iterations"`
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
	declared := map[string]bool{}

	for _, ref := range s.Tools {
		if ref.Name == "" {
			return fmt.Errorf("manifest: tool name required")
		}

		if declared[ref.Name] {
			return fmt.Errorf("manifest: tool %s declared multiple times", ref.Name)
		}

		declared[ref.Name] = true

		if ref.isSubprocess() {
			if ref.Schema.Type == "" {
				return fmt.Errorf("manifest: subprocess tool %s requires schema type", ref.Name)
			}
		} else if !knownTool(ref.Name) {
			return fmt.Errorf("manifest: unknown tool %s", ref.Name)
		}
	}

	for name, p := range s.Permissions {
		switch p {
		case RiskPolicyAllow, RiskPolicyAsk, RiskPolicyDeny:
		default:
			return fmt.Errorf("manifest: unknown permission %s", name)
		}
	}

	if s.OutputSchema.Type != "" && s.OutputSchema.Type != "object" {
		return fmt.Errorf("manifest: output schema type must be 'object' or empty, got %s", s.OutputSchema.Type)
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
			if !knownTool(t) && !declared[t] {
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

func (t ToolRef) isSubprocess() bool {
	return t.Command != ""
}

var _ yaml.Unmarshaler = (*ToolRef)(nil)

func (t *ToolRef) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		return node.Decode(&t.Name)
	}

	type raw ToolRef
	var r raw
	if err := node.Decode(&r); err != nil {
		return err
	}

	*t = ToolRef(r)
	return nil
}

func (r RiskName) toCore() core.RiskLevel {
	switch r {
	case "write":
		return core.RiskWrite
	case "execute":
		return core.RiskExecute
	default:
		return core.RiskNone
	}
}
