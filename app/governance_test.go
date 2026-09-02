package app

import (
	"context"
	"testing"

	"github.com/Federicoand98/mani/config"
	"github.com/Federicoand98/mani/core"
	"github.com/Federicoand98/mani/tool"
)

// testRuntime costruisce un Runtime offline con un client mock iniettato.
func testRuntime(t *testing.T, client core.LLMClient) *Runtime {
	t.Helper()
	cfg := config.Config{
		Provider:      "ollama",
		Providers:     map[string]config.ProviderConfig{"ollama": {BaseURL: "http://x", Model: "m"}},
		MaxIterations: 10,
	}
	rt := NewFromConfig(cfg)
	rt.agent.Client = client
	return rt
}

func drain(ch <-chan Event) (gotError bool) {
	for ev := range ch {
		if ev.Type == EventError {
			gotError = true
		}
	}
	return
}

func fakeTool(name string, risk core.RiskLevel, ran *bool) tool.Tool {
	return tool.New(
		tool.ToolSchema{Name: name, InputSchema: tool.InputSchema{Type: "object"}},
		risk,
		func(context.Context, map[string]any) (string, error) {
			if ran != nil {
				*ran = true
			}
			return "ok", nil
		},
	)
}

// deny: un pattern che matcha l'input deve bloccare il tool (executor mai chiamato).
func TestPolicyRules_DenyBlocksTool(t *testing.T) {
	ran := false
	client := core.NewMock(
		core.RespToolCall("1", "bash", map[string]any{"command": "rm -rf /tmp"}),
		core.RespText("fatto"),
	)
	rt := testRuntime(t, client)
	rt.WithTool(fakeTool("bash", core.RiskExecute, &ran))
	RegisterPolicyRules(rt, PolicySpec{
		Rules: []RuleSpec{{Tool: "bash", Pattern: `rm\s+-rf`, Label: "rm ricorsivo"}},
	})

	drain(rt.Execute(context.Background(), "cancella /tmp"))

	if ran {
		t.Error("il guardrail doveva bloccare bash, invece è stato eseguito")
	}
}

// deny: un pattern che NON matcha lascia passare il tool.
func TestPolicyRules_DenyAllowsUnmatched(t *testing.T) {
	ran := false
	client := core.NewMock(
		core.RespToolCall("1", "bash", map[string]any{"command": "ls -la"}),
		core.RespText("fatto"),
	)
	rt := testRuntime(t, client)
	rt.WithTool(fakeTool("bash", core.RiskExecute, &ran))
	RegisterPolicyRules(rt, PolicySpec{
		Rules: []RuleSpec{{Tool: "bash", Pattern: `rm\s+-rf`}},
	})

	drain(rt.Execute(context.Background(), "elenca"))

	if !ran {
		t.Error("un comando non vietato doveva essere eseguito")
	}
}

// budget: sforare max_tokens ferma la run con un errore.
func TestBudget_MaxTokensAborts(t *testing.T) {
	client := core.NewMock(core.WithUsage(core.RespText("done"), 120000, 0))
	rt := testRuntime(t, client)
	RegisterBudget(rt, LimitsSpec{MaxTokens: 100000})

	if !drain(rt.Execute(context.Background(), "x")) {
		t.Error("atteso EventError per max_tokens superato")
	}
}

// budget PER-RUN: due Execute di fila, ognuna sotto il limite → nessun errore (reset via context).
func TestBudget_PerRunReset(t *testing.T) {
	client := core.NewMockFunc(func([]core.Message, []core.ToolDefinition) core.LLMResponse {
		return core.WithUsage(core.RespText("done"), 60000, 0)
	})
	rt := testRuntime(t, client)
	RegisterBudget(rt, LimitsSpec{MaxTokens: 100000})

	for i := 0; i < 2; i++ {
		if drain(rt.Execute(context.Background(), "x")) {
			t.Fatalf("run %d: budget non resettato (EventError con 60k < 100k)", i)
		}
	}
}

// ---------------------------------------------------------------------------
// Journal: the guardrail vocabulary is a contract between producer and counter
// ---------------------------------------------------------------------------

// Every producer of a guardrail event must use a word RunRecord.apply counts.
// manifestPolicyHook used to write "denied" while apply counts "deny", so a tool
// blocked by policy.tools was recorded but never showed up in Summary.Blocked —
// the audit trail said the run was clean.
func TestManifestPolicy_DenyIsCountedAsBlocked(t *testing.T) {
	client := core.NewMock(
		core.RespToolCall("1", "bash", map[string]any{"command": "ls"}),
		core.RespText("done"),
	)
	rt := testRuntime(t, client)

	ran := false
	rt.WithTool(fakeTool("bash", core.RiskExecute, &ran))

	j := NewInMemoryJournal(10)
	RegisterJournal(rt, j)

	rt.permission = NewPermissionManager()
	rt.agent.AddPreToolUseHook(manifestPolicyHook(
		map[string]RiskPolicy{"bash": RiskPolicyDeny}, rt.permission, rt,
	))

	drain(rt.Execute(context.Background(), "list the files"))

	if ran {
		t.Error("policy.tools: deny did not block the tool")
	}

	metas, err := j.List(ListFilter{Limit: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(metas) != 1 {
		t.Fatalf("got %d runs in the journal, want 1", len(metas))
	}

	rec, err := j.Get(metas[0].ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if rec.Summary.Blocked != 1 {
		t.Errorf("Summary.Blocked = %d, want 1: a denied tool must be counted", rec.Summary.Blocked)
	}
}
