package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Federicoand98/mani/core"
)

// writeManifest scrive un manifest temporaneo e ne ritorna il path.
func writeManifest(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "agent.yaml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("scrittura manifest: %v", err)
	}
	return p
}

// ---------------------------------------------------------------------------
// La grammatica: gli 8 blocchi
// ---------------------------------------------------------------------------

// Round-trip completo: ogni chiave finisce nel blocco che le compete.
// Questo test DOCUMENTA la grammatica: se cambia, si aggiorna qui.
func TestManifest_EightBlocksRoundTrip(t *testing.T) {
	path := writeManifest(t, `
identity:
  name: nightly
  description: "manutenzione notturna"
  provider: anthropic
  model: claude-sonnet-5
  prompt: "Sei un agente."

capabilities:
  workspace: /tmp
  tools: [read, bash, planning, delegate]
  mcp:
    - { name: deepwiki, url: "https://example.invalid/sse" }
  subagents:
    - name: researcher
      description: "sola lettura"
      prompt: "Non modifichi nulla."
      tools: [read]

context:
  window: 128000
  inject: false
  compaction: { enabled: false, keep: 5 }

output:
  schema:
    type: object
    properties:
      sentiment: { type: string }
    required: [sentiment]

policy:
  tools: { bash: ask, default: allow }
  rules:
    - { tool: bash, pattern: 'rm\s+-rf', action: deny, label: distruttivo }
  redact:
    - { pattern: 'sk-[A-Za-z0-9]{20,}', with: "***" }

limits:
  max_tokens: 50000
  max_tool_calls: 20
  max_duration: 2m
  max_iterations: 15
  tool_timeout: 15s
  subagent_depth: 3

run:
  triggers:
    - type: daily
      at: "02:00"
      name: nightly-report
      catch_up: true
      memory: persistent
      prompt: "report"
  scheduler:
    path: ./queue
    concurrency: 2
    max_pending: 128
    retry: { max_attempts: 5, backoff: 10s }

observability:
  tracing: false
  journal: { enabled: true, path: ./runs, retention: 200 }
`)

	s, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}

	// 1. identity
	if s.Identity.Name != "nightly" || s.Identity.Provider != "anthropic" ||
		s.Identity.Model != "claude-sonnet-5" || s.Identity.Prompt == "" ||
		s.Identity.Description == "" {
		t.Errorf("identity: %+v", s.Identity)
	}
	// 2. capabilities
	if s.Capabilities.Workspace != "/tmp" || len(s.Capabilities.Tools) != 4 ||
		len(s.Capabilities.MCP) != 1 || len(s.Capabilities.Subagents) != 1 {
		t.Errorf("capabilities: %+v", s.Capabilities)
	}
	if s.Capabilities.Subagents[0].Prompt == "" {
		t.Error("subagent.prompt non letto (era system_prompt)")
	}
	// 3. context
	if s.Context.Window != 128000 || s.Context.Inject || s.Context.Compaction.Enabled ||
		s.Context.Compaction.Keep != 5 {
		t.Errorf("context: %+v", s.Context)
	}
	// 4. output
	if s.Output.Schema.Type != "object" || len(s.Output.Schema.Required) != 1 {
		t.Errorf("output: %+v", s.Output)
	}
	// 5. policy
	if s.Policy.Tools["bash"] != RiskPolicyAsk || s.Policy.Tools["default"] != RiskPolicyAllow ||
		len(s.Policy.Rules) != 1 || len(s.Policy.Redact) != 1 {
		t.Errorf("policy: %+v", s.Policy)
	}
	if s.Policy.Rules[0].Action != "deny" || s.Policy.Rules[0].Label != "distruttivo" {
		t.Errorf("policy.rules: %+v", s.Policy.Rules[0])
	}
	// 6. limits
	l := s.Limits
	if l.MaxTokens != 50000 || l.MaxToolCalls != 20 || l.MaxDuration != "2m" ||
		l.MaxIterations != 15 || l.ToolTimeout != "15s" || l.SubagentDepth != 3 {
		t.Errorf("limits: %+v", l)
	}
	// 7. run
	if len(s.Run.Triggers) != 1 || s.Run.Scheduler.Path != "./queue" ||
		s.Run.Scheduler.Concurrency != 2 || s.Run.Scheduler.Retry.MaxAttempts != 5 {
		t.Errorf("run: %+v", s.Run)
	}
	tr := s.Run.Triggers[0]
	if tr.Name != "nightly-report" || !tr.CatchUp || tr.Memory != "persistent" {
		t.Errorf("run.triggers[0]: %+v", tr)
	}
	// 8. observability
	if s.Observability.Tracing || !s.Observability.Journal.Enabled ||
		s.Observability.Journal.Retention != 200 {
		t.Errorf("observability: %+v", s.Observability)
	}
}

// Il manifest minimo: solo identity + capabilities, tutto il resto ai default.
func TestManifest_MinimalLoads(t *testing.T) {
	path := writeManifest(t, `
identity:
  provider: ollama
  model: m
capabilities:
  tools: [read]
`)
	s, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if len(s.Capabilities.Tools) != 1 {
		t.Errorf("tools: %+v", s.Capabilities.Tools)
	}
}

// ---------------------------------------------------------------------------
// Rottura netta: le chiavi della vecchia grammatica devono FALLIRE
// ---------------------------------------------------------------------------

// KnownFields(true) è la rete di sicurezza della fase 31: un manifest vecchio
// deve dare un errore che NOMINA la chiave, non partire in silenzio.
func TestManifest_LegacyKeysRejected(t *testing.T) {
	legacy := map[string]string{
		"permissions":    "permissions:\n  bash: deny\n",
		"guardrails":     "guardrails:\n  deny: []\n",
		"budget":         "budget:\n  max_tokens: 10\n",
		"queue":          "queue:\n  concurrency: 2\n",
		"features":       "features:\n  planning: true\n",
		"triggers":       "triggers: []\n",
		"tools":          "tools: [read]\n",
		"system_prompt":  "system_prompt: ciao\n",
		"provider":       "provider: ollama\n",
		"output_schema":  "output_schema:\n  type: object\n",
		"context_window": "context_window: 1000\n",
		"mcpservers":     "mcpservers: []\n",
	}

	for key, body := range legacy {
		t.Run(key, func(t *testing.T) {
			_, err := LoadManifest(writeManifest(t, body))
			if err == nil {
				t.Fatalf("la chiave legacy %q doveva essere rifiutata", key)
			}
			if !strings.Contains(err.Error(), key) {
				t.Errorf("l'errore deve nominare %q, ottenuto: %v", key, err)
			}
		})
	}
}

// Una chiave sconosciuta DENTRO un tool: KnownFields non ci arriva (UnmarshalYAML
// custom apre un decoder nuovo), quindi la verifica è manuale. È la stessa classe
// di bug di `mcp:` invece di `mcpservers:`.
func TestManifest_UnknownFieldInsideToolRejected(t *testing.T) {
	_, err := LoadManifest(writeManifest(t, `
identity: { provider: ollama, model: m }
capabilities:
  tools:
    - name: custom
      command: ./x.sh
      scheme: { type: object }
`))
	if err == nil {
		t.Fatal("un campo sconosciuto dentro un tool deve essere rifiutato")
	}
	if !strings.Contains(err.Error(), "scheme") {
		t.Errorf("l'errore deve nominare 'scheme', ottenuto: %v", err)
	}
}

// ---------------------------------------------------------------------------
// ToolRef eterogeneo (scalare + oggetto) con KnownFields attivo
// ---------------------------------------------------------------------------

func TestToolRef_ScalarAndObjectCoexist(t *testing.T) {
	s, err := LoadManifest(writeManifest(t, `
identity: { provider: ollama, model: m }
capabilities:
  tools:
    - read
    - name: fetch
      description: "scarica"
      command: ./x.sh
      risk: none
      schema:
        type: object
        properties:
          symbol: { type: string }
        required: [symbol]
`))
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}

	tools := s.Capabilities.Tools
	if len(tools) != 2 {
		t.Fatalf("attesi 2 tool, ottenuti %d", len(tools))
	}
	if tools[0].Name != "read" || tools[0].isSubprocess() {
		t.Errorf("tool[0] atteso built-in 'read': %+v", tools[0])
	}
	if tools[1].Name != "fetch" || !tools[1].isSubprocess() {
		t.Errorf("tool[1] atteso subprocess 'fetch': %+v", tools[1])
	}
	if tools[1].Schema.Type != "object" || tools[1].Risk != "none" {
		t.Errorf("tool[1] schema/risk non letti: %+v", tools[1])
	}
}

// ---------------------------------------------------------------------------
// Default
// ---------------------------------------------------------------------------

func TestDefaultSpec_Values(t *testing.T) {
	s := DefaultSpec()
	if !s.Context.Inject {
		t.Error("context.inject deve essere true di default")
	}
	if !s.Context.Compaction.Enabled || s.Context.Compaction.Keep != 20 {
		t.Errorf("compaction default: %+v", s.Context.Compaction)
	}
	if !s.Observability.Tracing {
		t.Error("observability.tracing deve essere true di default")
	}
	if s.Limits.SubagentDepth != 5 {
		t.Errorf("subagent_depth default = %d, atteso 5", s.Limits.SubagentDepth)
	}
	// i tool NON hanno default impliciti: planning/delegate vanno dichiarati (fase 31, G2)
	if len(s.Capabilities.Tools) != 0 {
		t.Errorf("nessun tool implicito atteso, ottenuti %+v", s.Capabilities.Tools)
	}
}

// Un valore esplicito deve vincere sul default (LoadManifest parte da DefaultSpec).
func TestManifest_ExplicitFalseOverridesDefault(t *testing.T) {
	s, err := LoadManifest(writeManifest(t, `
identity: { provider: ollama, model: m }
context:
  inject: false
observability:
  tracing: false
`))
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if s.Context.Inject {
		t.Error("inject: false esplicito deve vincere sul default true")
	}
	if s.Observability.Tracing {
		t.Error("tracing: false esplicito deve vincere sul default true")
	}
}

// ---------------------------------------------------------------------------
// Validate: i casi negativi
// ---------------------------------------------------------------------------

func TestValidate_Rejects(t *testing.T) {
	base := func() RuntimeSpec {
		s := DefaultSpec()
		s.Identity.Provider = "ollama"
		return s
	}

	cases := map[string]func(*RuntimeSpec){
		"tool sconosciuto": func(s *RuntimeSpec) {
			s.Capabilities.Tools = []ToolRef{{Name: "nonexistent"}}
		},
		"tool duplicato": func(s *RuntimeSpec) {
			s.Capabilities.Tools = []ToolRef{{Name: "read"}, {Name: "read"}}
		},
		"subprocess senza schema": func(s *RuntimeSpec) {
			s.Capabilities.Tools = []ToolRef{{Name: "x", Command: "./x"}}
		},
		"policy.tools valore invalido": func(s *RuntimeSpec) {
			s.Policy.Tools = map[string]RiskPolicy{"bash": "maybe"}
		},
		"policy.rules regex invalida": func(s *RuntimeSpec) {
			s.Policy.Rules = []RuleSpec{{Tool: "bash", Pattern: "["}}
		},
		"policy.redact regex invalida": func(s *RuntimeSpec) {
			s.Policy.Redact = []RedactSpec{{Pattern: "[", With: "x"}}
		},
		"output.schema non object": func(s *RuntimeSpec) {
			s.Output.Schema.Type = "string"
		},
		"trigger memory invalida": func(s *RuntimeSpec) {
			s.Run.Triggers = []TriggerSpec{{Type: "daily", At: "02:00", Memory: "boh"}}
		},
		"trigger type invalido": func(s *RuntimeSpec) {
			s.Run.Triggers = []TriggerSpec{{Type: "sometimes"}}
		},
	}

	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			s := base()
			mutate(&s)
			if err := s.Validate(); err == nil {
				t.Errorf("Validate doveva rifiutare: %s", name)
			}
		})
	}
}

func TestValidate_AcceptsKnownTools(t *testing.T) {
	s := DefaultSpec()
	s.Capabilities.Tools = []ToolRef{
		{Name: "read"},
		{Name: "edit"},
		{Name: "write"},
		{Name: "bash"},
		{Name: "planning"},
		{Name: "delegate"},
	}
	if err := s.Validate(); err != nil {
		t.Fatalf("i sei built-in devono passare: %v", err)
	}
}

// ---------------------------------------------------------------------------
// planning e delegate sono TOOL (fase 31, G2)
// ---------------------------------------------------------------------------

func TestBuild_PlanningAndDelegateAreTools(t *testing.T) {
	s := DefaultSpec()
	s.Identity.Provider = "ollama"
	s.Capabilities.Tools = []ToolRef{{Name: "read"}, {Name: "planning"}, {Name: "delegate"}}

	rt, err := Build(t.Context(), s)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer rt.Close()

	got := map[string]bool{}
	for _, tl := range rt.tools {
		got[tl.Name()] = true
	}
	for _, want := range []string{"read", "planning", "delegate"} {
		if !got[want] {
			t.Errorf("tool %q non registrato; presenti: %v", want, got)
		}
	}
}

// Senza dichiararli, planning e delegate NON esistono: è il punto di G2.
func TestBuild_NoImplicitTools(t *testing.T) {
	s := DefaultSpec()
	s.Identity.Provider = "ollama"
	s.Capabilities.Tools = []ToolRef{{Name: "read"}}

	rt, err := Build(t.Context(), s)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer rt.Close()

	for _, tl := range rt.tools {
		if tl.Name() == "planning" || tl.Name() == "delegate" {
			t.Errorf("tool %q non dichiarato ma registrato", tl.Name())
		}
	}
}

// Un solo vocabolario: la chiave del manifest DEVE essere il nome del tool a runtime.
// Se divergono, `policy.tools` non blocca nulla (cade sul default) e i subagent che
// referenziano il tool falliscono a runtime — entrambi in silenzio.
func TestBuiltinTools_ManifestKeyMatchesRuntimeName(t *testing.T) {
	s := DefaultSpec()
	s.Identity.Provider = "ollama"

	builtins := []string{"read", "edit", "write", "bash", "planning", "delegate"}
	for _, name := range builtins {
		s.Capabilities.Tools = append(s.Capabilities.Tools, ToolRef{Name: name})
	}

	rt, err := Build(t.Context(), s)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer rt.Close()

	runtimeNames := map[string]bool{}
	for _, tl := range rt.tools {
		runtimeNames[tl.Name()] = true
	}
	for _, key := range builtins {
		if !runtimeNames[key] {
			t.Errorf("la chiave di manifest %q non corrisponde a nessun tool a runtime; presenti: %v",
				key, runtimeNames)
		}
	}
}

// ---------------------------------------------------------------------------
// Gli esempi del repo devono restare validi (in fase 30 ne erano rotti tre)
// ---------------------------------------------------------------------------

func TestExamples_AllLoad(t *testing.T) {
	paths, err := filepath.Glob("../_examples/*.yaml")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("nessun esempio trovato in ../_examples")
	}

	for _, p := range paths {
		t.Run(filepath.Base(p), func(t *testing.T) {
			if _, err := LoadManifest(p); err != nil {
				t.Errorf("%s non carica: %v", filepath.Base(p), err)
			}
		})
	}
}

func TestRiskLevel_String_CoversAllLevels(t *testing.T) {
	cases := map[core.RiskLevel]string{
		core.RiskNone: "none", core.RiskNetwork: "network",
		core.RiskWrite: "write", core.RiskExecute: "execute",
	}
	for level, want := range cases {
		if got := level.String(); got != want {
			t.Errorf("RiskLevel(%d).String() = %q, want %q", level, got, want)
		}
	}
}

func TestRiskNetwork_IsNotRiskNone(t *testing.T) {
	if core.RiskNetwork == core.RiskNone {
		t.Fatal("RiskNetwork must differ from RiskNone")
	}
}
