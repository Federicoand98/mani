package app

import (
	"strings"
	"testing"

	"github.com/Federicoand98/mani/core"
)

type promptCall struct {
	toolName  string
	riskLevel string
	input     map[string]any
}

// recordingPrompter simula la UI: registra ogni richiesta di permesso e risponde
// sul canale Respond del payload (il manager è event-based, non più Prompter iniettato).
type recordingPrompter struct {
	calls  []promptCall
	decide func(call promptCall) Decision
}

func (r *recordingPrompter) emit() func(PermissionRequestPayload) {
	return func(p PermissionRequestPayload) {
		c := promptCall{toolName: p.ToolName, riskLevel: p.RiskLevel, input: p.Input}
		r.calls = append(r.calls, c)
		p.Respond <- r.decide(c)
	}
}

func newManager(rp *recordingPrompter) *PermissionManager {
	pm := NewPermissionManager()
	pm.setEmit(rp.emit())
	return pm
}

func TestPermissionManager_RiskNone_SkipsPrompter(t *testing.T) {
	rp := &recordingPrompter{decide: func(promptCall) Decision { return Deny }}
	pm := newManager(rp)

	err := pm.check("anything", core.RiskNone, nil)
	if err != nil {
		t.Errorf("RiskNone non deve errare, ottenuto %v", err)
	}
	if len(rp.calls) != 0 {
		t.Errorf("prompter non deve essere invocato per RiskNone, calls=%d", len(rp.calls))
	}
}

func TestPermissionManager_AllowOnce_DoesNotCache(t *testing.T) {
	rp := &recordingPrompter{decide: func(promptCall) Decision { return AllowOnce }}
	pm := newManager(rp)

	if err := pm.check("bash", core.RiskExecute, nil); err != nil {
		t.Fatalf("prima check inattesa: %v", err)
	}
	if err := pm.check("bash", core.RiskExecute, nil); err != nil {
		t.Fatalf("seconda check inattesa: %v", err)
	}
	if len(rp.calls) != 2 {
		t.Errorf("AllowOnce non deve cachare, calls attesi 2, ottenuti %d", len(rp.calls))
	}
}

func TestPermissionManager_AllowAlways_Caches(t *testing.T) {
	rp := &recordingPrompter{decide: func(promptCall) Decision { return AllowAlways }}
	pm := newManager(rp)

	for i := 0; i < 3; i++ {
		if err := pm.check("bash", core.RiskExecute, nil); err != nil {
			t.Fatalf("check #%d inattesa: %v", i, err)
		}
	}
	if len(rp.calls) != 1 {
		t.Errorf("AllowAlways deve cachare, prompter chiamato 1 volta, ottenuto %d", len(rp.calls))
	}
}

func TestPermissionManager_Deny_ReturnsError(t *testing.T) {
	rp := &recordingPrompter{decide: func(promptCall) Decision { return Deny }}
	pm := newManager(rp)

	err := pm.check("bash", core.RiskExecute, nil)
	if err == nil {
		t.Fatal("Deny deve ritornare errore")
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("messaggio errore inatteso: %q", err.Error())
	}
}

func TestPermissionManager_PrompterReceivesArgs(t *testing.T) {
	rp := &recordingPrompter{decide: func(promptCall) Decision { return AllowOnce }}
	pm := newManager(rp)

	input := map[string]any{"command": "ls"}
	pm.check("bash", core.RiskExecute, input)

	if len(rp.calls) != 1 {
		t.Fatalf("attesa 1 chiamata, ottenute %d", len(rp.calls))
	}
	c := rp.calls[0]
	if c.toolName != "bash" {
		t.Errorf("toolName atteso 'bash', ottenuto %q", c.toolName)
	}
	if c.riskLevel != "execute" {
		t.Errorf("riskLevel atteso 'execute', ottenuto %q", c.riskLevel)
	}
	if c.input["command"] != "ls" {
		t.Errorf("input non propagato: %+v", c.input)
	}
}

func TestPermissionManager_DifferentTools_NotShared(t *testing.T) {
	rp := &recordingPrompter{decide: func(c promptCall) Decision {
		if c.toolName == "bash" {
			return AllowAlways
		}
		return Deny
	}}
	pm := newManager(rp)

	if err := pm.check("bash", core.RiskExecute, nil); err != nil {
		t.Fatalf("bash AllowAlways: %v", err)
	}
	if err := pm.check("edit_file", core.RiskWrite, nil); err == nil {
		t.Error("edit_file deve essere negato, non condiviso con bash")
	}
}

func TestPermissionManager_Hook_DelegatesToCheck(t *testing.T) {
	rp := &recordingPrompter{decide: func(promptCall) Decision { return AllowOnce }}
	pm := newManager(rp)

	hook := pm.Hook()
	input := map[string]any{"x": 1}
	err := hook("edit_file", core.RiskWrite, input)
	if err != nil {
		t.Fatalf("hook error inatteso: %v", err)
	}
	if len(rp.calls) != 1 {
		t.Fatalf("hook deve invocare il prompter una volta, ottenuto %d", len(rp.calls))
	}
	c := rp.calls[0]
	if c.toolName != "edit_file" || c.riskLevel != "write" {
		t.Errorf("hook args non propagati: %+v", c)
	}
}
