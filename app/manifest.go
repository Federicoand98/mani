package app

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/Federicoand98/mani/core"
	"github.com/Federicoand98/mani/tool"
	"gopkg.in/yaml.v3"
)

type RuntimeSpec struct {
	Identity      IdentitySpec      `yaml:"identity"`
	Capabilities  CapabilitiesSpec  `yaml:"capabilities"`
	Context       ContextSpec       `yaml:"context"`
	Output        OutputSpec        `yaml:"output"`
	Policy        PolicySpec        `yaml:"policy"`
	Limits        LimitsSpec        `yaml:"limits"`
	Run           RunSpec           `yaml:"run"`
	Observability ObservabilitySpec `yaml:"observability"`
}

// --- 1. identity ---

type IdentitySpec struct {
	Name          string `yaml:"name"`
	Description   string `yaml:"description"`
	Provider      string `yaml:"provider"`
	Model         string `yaml:"model"`
	Prompt        string `yaml:"prompt"`
	promptInclude string `yaml:"-"`
}

// --- 2. capabilities ---

type CapabilitiesSpec struct {
	Workspace string         `yaml:"workspace"`
	Tools     []ToolRef      `yaml:"tools"`
	MCP       []MCPSpec      `yaml:"mcp"`
	Subagents []SubagentSpec `yaml:"subagents"`
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

type SubagentSpec struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Prompt      string   `yaml:"prompt"`
	Model       string   `yaml:"model"` // "" = inherit
	Tools       []string `yaml:"tools"`
}

type MCPSpec struct {
	Name    string   `yaml:"name"`
	Command string   `yaml:"command"`
	Args    []string `yaml:"args"`
	URL     string   `yaml:"url"`
}

// --- 3. context ---

type ContextSpec struct {
	Window     int           `yaml:"window"`
	Inject     bool          `yaml:"inject"`
	Compaction CompactionCfg `yaml:"compaction"`
}

type CompactionCfg struct {
	Enabled bool `yaml:"enabled"`
	Keep    int  `yaml:"keep"`
}

// --- 4. output ---

type OutputSpec struct {
	Schema tool.InputSchema `yaml:"schema"`
}

// --- 5. policy ---
type (
	RiskPolicy string
	RiskName   string
)

const (
	RiskPolicyAllow RiskPolicy = "allow"
	RiskPolicyDeny  RiskPolicy = "deny"
	RiskPolicyAsk   RiskPolicy = "ask"
)

type PolicySpec struct {
	Tools   map[string]RiskPolicy `yaml:"tools"`
	Rules   []RuleSpec            `yaml:"rules"`
	Redact  []RedactSpec          `yaml:"redact"`
	Network NetworkSpec           `yaml:"network"`
}

type RuleSpec struct {
	Tool    string `yaml:"tool"`
	Pattern string `yaml:"pattern"`
	Action  string `yaml:"action"`
	Label   string `yaml:"label"`
}

type RedactSpec struct {
	Pattern string `yaml:"pattern"`
	With    string `yaml:"with"`
}

type NetworkSpec struct {
	Allow []string `yaml:"allow"`
	Deny  []string `yaml:"deny"`
}

// --- 6. limits ---

type LimitsSpec struct {
	MaxTokens     int    `yaml:"max_tokens"`
	MaxToolCalls  int    `yaml:"max_tool_calls"`
	MaxDuration   string `yaml:"max_duration"`
	MaxIterations int    `yaml:"max_iterations"`
	ToolTimeout   string `yaml:"tool_timeout"`
	SubagentDepth int    `yaml:"subagent_depth"`
}

// --- 7. run ---

type RunSpec struct {
	Triggers  []TriggerSpec `yaml:"triggers"`
	Scheduler SchedulerSpec `yaml:"scheduler"`
}

type SchedulerSpec struct {
	Path        string    `yaml:"path"`
	Concurrency int       `yaml:"concurrency"`
	MaxPending  int       `yaml:"max_pending"`
	Retry       RetrySpec `yaml:"retry"`
}

type RetrySpec struct {
	MaxAttempts int    `yaml:"max_attempts"` // default 3
	Backoff     string `yaml:"backoff"`      // default "30s"
}

type TriggerSpec struct {
	Type    string `yaml:"type"`
	Every   string `yaml:"every"`
	At      string `yaml:"at"`
	Addr    string `yaml:"addr"`
	Prompt  string `yaml:"prompt"`
	Name    string `yaml:"name"`     // ← identità stabile (opzionale)
	Memory  string `yaml:"memory"`   // ← "" = fresh (default) | "persistent"
	CatchUp bool   `yaml:"catch_up"` // ← default false
}

// --- 8. observability ---

type ObservabilitySpec struct {
	Tracing bool        `yaml:"tracing"`
	Journal JournalSpec `yaml:"journal"`
}

type JournalSpec struct {
	Enabled   bool   `yaml:"enabled"`
	Path      string `yaml:"path"`
	Retention int    `yaml:"retention"` // ring buffer
}

func DefaultSpec() RuntimeSpec {
	return RuntimeSpec{
		Context: ContextSpec{
			Inject:     true,
			Compaction: CompactionCfg{Enabled: true, Keep: 20},
		},
		Limits: LimitsSpec{
			SubagentDepth: 5,
		},
		Observability: ObservabilitySpec{Tracing: true},
	}
}

func LoadManifest(path string) (RuntimeSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return RuntimeSpec{}, fmt.Errorf("[manifest]: read %s: %w", path, err)
	}

	spec := DefaultSpec()

	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&spec); err != nil && !errors.Is(err, io.EOF) {
		return RuntimeSpec{}, fmt.Errorf("[manifest]: decode %s: %w", path, err)
	}

	if err := spec.resolveIncludes(filepath.Dir(path)); err != nil {
		return RuntimeSpec{}, fmt.Errorf("[manifest]: resolve includes %s: %w", path, err)
	}

	if err := spec.Validate(); err != nil {
		return RuntimeSpec{}, fmt.Errorf("[manifest]: validate %s: %w", path, err)
	}

	return spec, nil
}

func (s RuntimeSpec) Validate() error {
	// --- capabilities.tools ---
	declared := map[string]bool{}
	for _, ref := range s.Capabilities.Tools {
		if ref.Name == "" {
			return fmt.Errorf("[manifest]: capabilities.tools: tool name required")
		}
		if declared[ref.Name] {
			return fmt.Errorf("[manifest]: tool %q declarated too many times", ref.Name)
		}
		declared[ref.Name] = true

		if ref.isSubprocess() {
			if ref.Schema.Type == "" {
				return fmt.Errorf("[manifest]: tool subprocess %q requires schema.type", ref.Name)
			}
		} else if !knownTool(ref.Name) {
			return fmt.Errorf("[manifest]: tool unknown %q (built-in: read, edit, write, bash, planning, delegate)", ref.Name)
		}
	}

	if len(s.Capabilities.Subagents) > 0 && !declared["delegate"] {
		return fmt.Errorf("[manifest]: capabilities.subagents but \"delegate\" tool not declared")
	}

	seen := map[string]bool{}
	for _, sa := range s.Capabilities.Subagents {
		if sa.Name == "" {
			return fmt.Errorf("[manifest]: subagent: name required")
		}
		if sa.Description == "" {
			return fmt.Errorf("[manifest]: subagent %q: description required", sa.Name)
		}
		if seen[sa.Name] {
			return fmt.Errorf("[manifest]: subagent %q: duplicate", sa.Name)
		}
		seen[sa.Name] = true
		for _, t := range sa.Tools {
			if !knownTool(t) && !declared[t] {
				return fmt.Errorf("[manifest]: subagent %q: tool unknown %q", sa.Name, t)
			}
		}
	}

	// --- policy ---
	for name, p := range s.Policy.Tools {
		switch p {
		case RiskPolicyAllow, RiskPolicyAsk, RiskPolicyDeny:
		default:
			return fmt.Errorf("[manifest]: policy.tools[%s]: invalid value %q (allow|ask|deny)", name, p)
		}
	}
	for i, r := range s.Policy.Rules {
		if r.Pattern == "" {
			return fmt.Errorf("[manifest]: policy.rules[%d]: pattern required", i)
		}
		if _, err := regexp.Compile(r.Pattern); err != nil {
			return fmt.Errorf("[manifest]: policy.rules[%d]: invalid regex: %w", i, err)
		}
		if r.Action != "" && r.Action != "deny" {
			return fmt.Errorf("[manifest]: policy.rules[%d]: action %q not supported (only \"deny\")", i, r.Action)
		}
	}
	for i, m := range s.Policy.Redact {
		if _, err := regexp.Compile(m.Pattern); err != nil {
			return fmt.Errorf("[manifest]: policy.redact[%d]: invalid regex: %w", i, err)
		}
	}

	// --- output ---
	if s.Output.Schema.Type != "" && s.Output.Schema.Type != "object" {
		return fmt.Errorf("[manifest]: output.schema.type must be \"object\", found %q", s.Output.Schema.Type)
	}

	// --- limits (le durate) ---
	for name, v := range map[string]string{
		"limits.max_duration": s.Limits.MaxDuration,
		"limits.tool_timeout": s.Limits.ToolTimeout,
	} {
		if v == "" {
			continue
		}
		if _, err := time.ParseDuration(v); err != nil {
			return fmt.Errorf("manifest: %s: %w", name, err)
		}
	}

	// --- run ---
	if s.Run.Scheduler.Concurrency < 0 {
		return fmt.Errorf("[manifest]: run.scheduler.concurrency must be >= 0")
	}
	if b := s.Run.Scheduler.Retry.Backoff; b != "" {
		if _, err := time.ParseDuration(b); err != nil {
			return fmt.Errorf("[manifest]: run.scheduler.retry.backoff: %w", err)
		}
	}
	seenTrigger := map[string]bool{}
	for i, t := range s.Run.Triggers {
		switch t.Type {
		case "every", "daily", "webhook":
		default:
			return fmt.Errorf("[manifest]: run.triggers[%d]: type %q not valid (every|daily|webhook)", i, t.Type)
		}
		if t.Memory != "" && t.Memory != "fresh" && t.Memory != "persistent" {
			return fmt.Errorf("[manifest]: run.triggers[%d]: memory must be fresh|persistent, found %q", i, t.Memory)
		}
		if t.Name != "" {
			if seenTrigger[t.Name] {
				return fmt.Errorf("[manifest]: run.triggers: name %q duplicate", t.Name)
			}
			seenTrigger[t.Name] = true
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

	known := map[string]bool{
		"name": true, "description": true, "command": true,
		"args": true, "env": true, "schema": true, "risk": true,
	}
	for i := 0; i < len(node.Content); i += 2 {
		if k := node.Content[i].Value; !known[k] {
			return fmt.Errorf("tool: campo sconosciuto %q (riga %d)", k, node.Content[i].Line)
		}
	}

	*t = ToolRef(r)
	return nil
}

const includeTag = "!include"

func (i *IdentitySpec) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("identity: expected a mapping (line %d)", node.Line)
	}

	known := map[string]bool{
		"name": true, "description": true, "provider": true, "model": true, "prompt": true,
	}

	for k := 0; k+1 < len(node.Content); k += 2 {
		key, val := node.Content[k], node.Content[k+1]

		if !known[key.Value] {
			return fmt.Errorf("identity: unknown field %q (line %d)", key.Value, key.Line)
		}

		if val.Tag == includeTag && key.Value != "prompt" {
			return fmt.Errorf("identity: !include is only allowed for prompt (line %d)", val.Line)
		}

		switch key.Value {
		case "name":
			i.Name = val.Value
		case "description":
			i.Description = val.Value
		case "provider":
			i.Provider = val.Value
		case "model":
			i.Model = val.Value
		case "prompt":
			if val.Tag == includeTag {
				i.promptInclude = val.Value
			} else {
				i.Prompt = val.Value
			}
		}
	}
	return nil
}

const macIncludeBytes = 256 << 10

func (s *RuntimeSpec) resolveIncludes(baseDir string) error {
	if s.Identity.promptInclude != "" {
		text, err := readInclude(baseDir, s.Identity.promptInclude)
		if err != nil {
			return fmt.Errorf("identity.prompt: %w", err)
		}
		s.Identity.Prompt = text
	}
	return nil
}

func readInclude(baseDir, rel string) (string, error) {
	if filepath.IsAbs(rel) || strings.HasPrefix(rel, "/") || strings.HasPrefix(rel, `\`) {
		return "", fmt.Errorf("!include %q: absolute paths are not supported", rel)
	}

	full := filepath.Join(baseDir, filepath.FromSlash(rel))

	info, err := os.Stat(full)
	if err != nil {
		return "", fmt.Errorf("!include %q: %w", rel, err)
	}

	if info.IsDir() {
		return "", fmt.Errorf("!include %q: directories are not supported", rel)
	}

	if info.Size() > macIncludeBytes {
		return "", fmt.Errorf("!include %q: file is too large", rel)
	}

	data, err := os.ReadFile(full)
	if err != nil {
		return "", fmt.Errorf("!include %q: %w", rel, err)
	}

	return string(data), nil
}

func (r RiskName) toCore() core.RiskLevel {
	switch r {
	case "network":
		return core.RiskNetwork
	case "write":
		return core.RiskWrite
	case "execute":
		return core.RiskExecute
	default:
		return core.RiskNone
	}
}
