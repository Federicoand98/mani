package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Federicoand98/mani/core"
	"github.com/Federicoand98/mani/journal"
	"github.com/Federicoand98/mani/manifest"
)

// A run built from a manifest tells, in its journal, who ran, what it was
// asked, the limits it ran under, every answer to an "ask", what each tool
// returned and what it answered: the record of a triggered run has to make
// sense to someone who was not there.
func TestBuild_TheJournalTellsTheWholeRun(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, "bank.txt"), []byte("pay to IBAN IT60X0542811101000000123456 today"), 0o644); err != nil {
		t.Fatal(err)
	}

	spec := manifest.Default()
	spec.File = "agents/payer.yaml"
	spec.Identity = manifest.IdentitySpec{Name: "payer", Provider: "ollama", Model: "qwen3.5:9b", Prompt: "pay"}
	spec.Capabilities.Workspace = ws
	spec.Capabilities.Tools = []manifest.ToolRef{{Name: "read"}, {Name: "write"}}
	spec.Policy.Tools = map[string]manifest.RiskPolicy{"read": manifest.RiskPolicyAllow, "write": manifest.RiskPolicyAsk}
	spec.Policy.Redact = []manifest.RedactSpec{{Pattern: `IT\d{2}[A-Z0-9]{23}`, With: "[iban]"}}
	spec.Limits.MaxToolCalls = 6
	spec.Limits.MaxDuration = "2m"
	spec.Observability.Journal = manifest.JournalSpec{Enabled: true, Path: filepath.Join(t.TempDir(), "runs")}

	rt, err := Build(context.Background(), spec)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer rt.Close()
	rt.agent.Client = core.NewMock(
		core.RespToolCall("1", "read", map[string]any{"path": "bank.txt"}),
		core.RespToolCall("2", "write", map[string]any{"path": "out.txt", "content": "paid"}),
		core.RespText("I could not write the receipt."),
	)

	for ev := range rt.Execute(WithSource(context.Background(), "web"), "pay the bill in bank.txt") {
		if ev.Type == EventPermissionRequest {
			ev.Payload.(PermissionRequestPayload).Respond <- Deny
		}
	}

	j, ok := OpenJournalReader(spec)
	if !ok {
		t.Fatal("journal not readable")
	}
	metas, err := j.List(journal.ListFilter{})
	if err != nil || len(metas) != 1 {
		t.Fatalf("List = %+v, err=%v", metas, err)
	}
	rec, err := j.Get(metas[0].ID)
	if err != nil {
		t.Fatal(err)
	}

	if rec.Agent != (journal.Agent{Name: "payer", Manifest: "agents/payer.yaml", Provider: "ollama", Model: "qwen3.5:9b"}) {
		t.Errorf("agent = %+v", rec.Agent)
	}
	if rec.Task != "pay the bill in bank.txt" || rec.Answer != "I could not write the receipt." || rec.Status != "ok" {
		t.Errorf("task %q, answer %q, status %q", rec.Task, rec.Answer, rec.Status)
	}
	if rec.Limits.MaxToolCalls != 6 || rec.Limits.MaxDuration != "2m" || rec.Limits.MaxIterations == 0 {
		t.Errorf("limits = %+v", rec.Limits)
	}
	if rec.Summary.Blocked != 1 {
		t.Errorf("blocked = %d, want 1: the denied write", rec.Summary.Blocked)
	}

	var decisions []string
	preview := ""
	for _, ev := range rec.Events {
		switch ev.Kind {
		case journal.EvPermission:
			decisions = append(decisions, ev.Data["tool"].(string)+" "+ev.Data["decision"].(string))
		case journal.EvToolResult:
			if ev.Data["tool"] == "read" {
				preview, _ = ev.Data["preview"].(string)
			}
		}
	}
	if strings.Join(decisions, ",") != "write denied" {
		t.Errorf("decisions = %v, want the one on write", decisions)
	}
	// The preview is written after redaction: the journal must never keep
	// what the model itself was not shown.
	if !strings.Contains(preview, "[iban]") || strings.Contains(preview, "IT60X") {
		t.Errorf("read preview = %q, want the masked output", preview)
	}
}

func TestClip_CutsOnARuneAndSaysSo(t *testing.T) {
	if got := clip("short", 10); got != "short" {
		t.Errorf("clip(short) = %q", got)
	}
	got := clip("aaàbbb", 3) // "à" is two bytes from index 2
	if !strings.HasPrefix(got, "aa\n[... cut at 3 bytes]") {
		t.Errorf("clip = %q, want the cut before the split rune", got)
	}
}
