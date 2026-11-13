package wire

import (
	"encoding/json"
	"testing"

	"github.com/Federicoand98/mani/app"
	"github.com/Federicoand98/mani/core"
)

func TestContextEventIsSeparateFromUsage(t *testing.T) {
	last := 123
	ev := app.Event{Type: app.EventContext, Payload: core.ContextUsage{
		EstimatedInput: 150, InputBudget: 896, OutputLimit: 128, ContextWindow: 1024,
		LimitSource: "manual", LastInput: &last, State: "ready", Compacted: true,
	}}
	msg, ok := FromEvent(ev)
	if !ok || msg.Type != "context" {
		t.Fatal("context event dropped")
	}
	raw, err := json.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Payload Context `json:"payload"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	p := decoded.Payload
	if p.InputBudget != 896 || p.OutputLimit != 128 || p.LastInput == nil || *p.LastInput != 123 || !p.Compacted {
		t.Fatalf("payload=%+v", p)
	}
}
