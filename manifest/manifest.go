package manifest

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Federicoand98/mani/core"
	"github.com/Federicoand98/mani/tool"
	"gopkg.in/yaml.v3"
)

type Spec struct {
	// File is the path Load read the manifest from, as it was given. It is
	// not a key of the manifest: runs record it to say which agent ran.
	File string `yaml:"-"`

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
	Enabled   bool   `yaml:"enabled"`
	Keep      int    `yaml:"keep"`
	Mode      string `yaml:"mode,omitempty"`
	KeepTurns int    `yaml:"keep_turns,omitempty"`
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
	MaxTokens       int    `yaml:"max_tokens"`
	MaxOutputTokens int    `yaml:"max_output_tokens,omitempty"`
	MaxToolCalls    int    `yaml:"max_tool_calls"`
	MaxDuration     string `yaml:"max_duration"`
	MaxIterations   int    `yaml:"max_iterations"`
	ToolTimeout     string `yaml:"tool_timeout"`
	SubagentDepth   int    `yaml:"subagent_depth"`
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
	Path    string `yaml:"path"`  // default "/hook"
	Token   string `yaml:"token"` // fallback to MANI_WEBHOOK_TOKEN
	Prompt  string `yaml:"prompt"`
	Name    string `yaml:"name"`     // ← identità stabile (opzionale)
	Memory  string `yaml:"memory"`   // ← "" = fresh (default) | "persistent"
	CatchUp bool   `yaml:"catch_up"` // ← default false
}

// HookPath returns the webhook route, defaulting to /hook so a manifest written
// before 0.1.4 keeps working unchanged.
func (t TriggerSpec) HookPath() string {
	if t.Path == "" {
		return DefaultHookPath
	}
	return t.Path
}

// --- 8. observability ---

type ObservabilitySpec struct {
	Tracing bool        `yaml:"tracing"`
	Journal JournalSpec `yaml:"journal"`
}

type JournalSpec struct {
	Enabled   bool   `yaml:"enabled"`
	Backend   string `yaml:"backend"` // jsonl (default) | sqlite
	Path      string `yaml:"path"`
	Retention int    `yaml:"retention"` // ring buffer
}

func Default() Spec {
	return Spec{
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

func Load(path string) (Spec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Spec{}, fmt.Errorf("[manifest]: read %s: %w", path, err)
	}
	spec, _, err := parse(data, filepath.Dir(path), path)
	if err != nil {
		return Spec{}, err
	}
	spec.File = path
	return spec, spec.Validate()
}

// parse decodes a manifest: unknown fields, ${VAR}, defaults and !include. It
// returns the tree as written too, before ${VAR}, so a problem can be traced to
// its line. name labels the errors and may be empty.
func parse(data []byte, baseDir, name string) (Spec, *yaml.Node, error) {
	wrap := func(stage string, err error) error {
		if name == "" {
			return fmt.Errorf("[manifest]: %s: %w", stage, err)
		}
		return fmt.Errorf("[manifest]: %s %s: %w", stage, name, err)
	}

	{
		probe := Default()
		dec := yaml.NewDecoder(bytes.NewReader(data))
		dec.KnownFields(true)
		if err := dec.Decode(&probe); err != nil && !errors.Is(err, io.EOF) {
			return Spec{}, nil, wrap("decode", err)
		}
	}

	var written, root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return Spec{}, nil, wrap("unmarshal", err)
	}
	_ = yaml.Unmarshal(data, &written) // same bytes, cannot fail now

	if err := expandEnvNodes(&root); err != nil {
		return Spec{}, nil, wrap("expand env", err)
	}

	spec := Default()

	if len(root.Content) > 0 {
		if err := root.Decode(&spec); err != nil {
			return Spec{}, nil, wrap("decode", err)
		}
	}

	if err := spec.resolveIncludes(baseDir); err != nil {
		return Spec{}, nil, wrap("resolve includes", err)
	}

	return spec, &written, nil
}

// Validate reports the first problem of the manifest, or nil.
func (s Spec) Validate() error {
	if ps := s.Problems(); len(ps) > 0 {
		return ps[0]
	}
	return nil
}

func (t ToolRef) IsSubprocess() bool {
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

func (s *Spec) resolveIncludes(baseDir string) error {
	if s.Identity.promptInclude != "" {
		text, err := readInclude(baseDir, s.Identity.promptInclude)
		if err != nil {
			return fmt.Errorf("identity.prompt: %w", err)
		}
		s.Identity.Prompt = text
	}

	if err := resolveSchemaIncludes(baseDir, "output.schema", &s.Output.Schema); err != nil {
		return err
	}
	for i := range s.Capabilities.Tools {
		where := fmt.Sprintf("capabilities.tools[%d].schema", i)
		if err := resolveSchemaIncludes(baseDir, where, &s.Capabilities.Tools[i].Schema); err != nil {
			return err
		}
	}
	return nil
}

func resolveSchemaIncludes(baseDir, where string, schema *tool.InputSchema) error {
	for name, prop := range schema.Properties {
		// map values are copies: resolve, then put it back
		if err := resolvePropIncludes(baseDir, where+"."+name, &prop); err != nil {
			return err
		}
		schema.Properties[name] = prop
	}
	return nil
}

func resolvePropIncludes(baseDir, where string, p *tool.PropertySchema) error {
	if p.Enum != nil && p.Enum.Include != "" {
		content, err := readInclude(baseDir, p.Enum.Include)
		if err != nil {
			return fmt.Errorf("%s.enum: %w", where, err)
		}
		values, err := parseEnumFile(p.Enum.Include, content)
		if err != nil {
			return fmt.Errorf("%s.enum: %w", where, err)
		}
		p.Enum = &tool.EnumValues{Values: values}
	}

	if p.Items != nil {
		if err := resolvePropIncludes(baseDir, where+"[]", p.Items); err != nil {
			return err
		}
	}
	for name, sub := range p.Properties {
		if err := resolvePropIncludes(baseDir, where+"."+name, &sub); err != nil {
			return err
		}
		p.Properties[name] = sub
	}
	return nil
}

func parseEnumFile(name, content string) ([]string, error) {
	var values []string

	switch strings.ToLower(filepath.Ext(name)) {
	case ".json", ".yaml", ".yml":
		// YAML is a superset of JSON: one parser covers both.
		if err := yaml.Unmarshal([]byte(content), &values); err != nil {
			return nil, fmt.Errorf("!include %q: expected a list of strings: %w", name, err)
		}
	default:
		for _, line := range strings.Split(content, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			values = append(values, line)
		}
	}

	if len(values) == 0 {
		return nil, fmt.Errorf("!include %q: no values", name)
	}
	seen := make(map[string]bool, len(values))
	for _, v := range values {
		if v == "" {
			return nil, fmt.Errorf("!include %q: contains an empty value", name)
		}
		if seen[v] {
			return nil, fmt.Errorf("!include %q: duplicate value %q", name, v)
		}
		seen[v] = true
	}
	return values, nil
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

// expandEnvNodes replaces ${VAR} in the scalar VALUES of the tree, never in
// keys. It skips block scalars (| and >): these are human-written prose —
// typically identity.prompt — and stripping their ${} would be a bug that
// surfaces much later, in production.
// An undefined variable is an ERROR, not an empty string: token:
// ${HOOK_TOKEN} with a missing variable would become an empty token, which for
// BearerAuth means "authentication disabled". The most dangerous silent failure
// possible.
func expandEnvNodes(n *yaml.Node) error {
	switch n.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			if err := expandEnvNodes(n.Content[i+1]); err != nil {
				return err
			}
		}
	case yaml.DocumentNode, yaml.SequenceNode:
		for _, c := range n.Content {
			if err := expandEnvNodes(c); err != nil {
				return err
			}
		}
	case yaml.ScalarNode:
		if n.Style == yaml.LiteralStyle || n.Style == yaml.FoldedStyle {
			return nil
		}
		v, err := expandEnvString(n.Value, n.Line)
		if err != nil {
			return err
		}
		n.Value = v
	}
	return nil
}

var envRef = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

func expandEnvString(s string, line int) (string, error) {
	var missing []string

	out := envRef.ReplaceAllStringFunc(s, func(m string) string {
		name := envRef.FindStringSubmatch(m)[1]
		val, ok := os.LookupEnv(name)
		if !ok {
			missing = append(missing, name)
			return m
		}
		return val
	})

	if len(missing) > 0 {
		return "", fmt.Errorf("line %d: environment variable %s is not set", line, strings.Join(missing, ", "))
	}
	return out, nil
}

func (r RiskName) Core() core.RiskLevel {
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

const enumWarnThreshold = 200

// Warnings reports what is legal but probably a mistake. It never fails a load.
func (s Spec) Warnings() []string {
	var out []string
	walkSchema("output.schema", s.Output.Schema, &out)
	for i, ref := range s.Capabilities.Tools {
		walkSchema(fmt.Sprintf("capabilities.tools[%d].schema", i), ref.Schema, &out)
	}
	return out
}

func walkSchema(where string, schema tool.InputSchema, out *[]string) {
	for name, prop := range schema.Properties {
		walkProp(where+"."+name, prop, out)
	}
}

func walkProp(where string, p tool.PropertySchema, out *[]string) {
	if p.Enum != nil && len(p.Enum.Values) > enumWarnThreshold {
		*out = append(*out, fmt.Sprintf(
			"%s.enum has %d values: the schema is sent on every call, consider a lookup tool instead",
			where, len(p.Enum.Values)))
	}
	if p.Items != nil {
		walkProp(where+"[]", *p.Items, out)
	}
	for name, sub := range p.Properties {
		walkProp(where+"."+name, sub, out)
	}
}
