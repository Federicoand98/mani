package app

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Federicoand98/mani/core"
	"github.com/Federicoand98/mani/manifest"
	"github.com/Federicoand98/mani/tool"
)

func TestTruncateOutput(t *testing.T) {
	if got := truncateOutput("short", 10); got != "short" {
		t.Errorf("under the limit: got %q, want it unchanged", got)
	}
	exact := strings.Repeat("x", 10)
	if got := truncateOutput(exact, 10); got != exact {
		t.Errorf("at the limit: got %q, want it unchanged", got)
	}

	long := "HEAD" + strings.Repeat("m", 1000) + "TAIL"
	got := truncateOutput(long, 100)
	if !strings.HasPrefix(got, "HEAD") || !strings.HasSuffix(got, "TAIL") {
		t.Errorf("the head and the tail must survive, got %q", got)
	}
	if !strings.Contains(got, "output truncated: 908 of 1008 bytes cut") {
		t.Errorf("the marker must say how much was cut, got %q", got)
	}
	if kept := strings.Count(got, "m"); kept > 100 {
		t.Errorf("kept %d bytes of the middle, want at most the limit", kept)
	}
}

// A cut never splits a multi-byte rune: the result stays valid UTF-8.
func TestTruncateOutput_RuneBoundaries(t *testing.T) {
	s := strings.Repeat("è€😀", 200)
	for _, limit := range []int{7, 8, 9, 10, 11, 100, 101} {
		if got := truncateOutput(s, limit); !utf8.ValidString(got) {
			t.Errorf("limit %d: the result is not valid UTF-8", limit)
		}
	}
}

// The tool result that reaches the model is the clipped one, and a secret
// standing across the cut is redacted before the cut, so no half of it leaks.
func TestToolOutputTruncation_AfterRedaction(t *testing.T) {
	out := strings.Repeat("a", 15) + "tok-ABCDEFGHIJ" + strings.Repeat("b", 100)
	big := tool.New(tool.ToolSchema{Name: "read", InputSchema: tool.InputSchema{Type: "object"}}, core.RiskNone,
		func(context.Context, map[string]any) (string, error) { return out, nil })

	client := core.NewMock(core.RespToolCall("1", "read", nil), core.RespText("done"))
	rt := testRuntime(t, client)
	rt.WithTool(big)
	RegisterPolicyRules(rt, manifest.PolicySpec{Redact: []manifest.RedactSpec{{Pattern: `tok-[A-Z]+`, With: "[x]"}}})
	RegisterToolOutputTruncation(rt, 40)

	drain(rt.Execute(context.Background(), "read it"))

	if len(client.Sent) < 2 {
		t.Fatalf("want a second call carrying the tool result, got %d calls", len(client.Sent))
	}
	result := lastToolResult(t, client.Sent[1])
	if !strings.Contains(result, "output truncated") {
		t.Errorf("the result must be clipped, got %q", result)
	}
	if strings.Contains(result, "tok-") {
		t.Errorf("part of the secret reached the model: %q", result)
	}
}

func TestToolOutputTruncation_SubagentsInherit(t *testing.T) {
	rt := testRuntime(t, core.NewMock())
	RegisterToolOutputTruncation(rt, 40)

	p := &core.PostToolUsePayload{ToolName: "read", Result: strings.Repeat("z", 500)}
	child := rt.spawnChild()
	if err := child.Hooks().Fire(context.Background(), core.HookEvent{Type: core.HookPostToolUse, Payload: p}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(p.Result, "output truncated") {
		t.Errorf("a subagent's tool result must be clipped too, got %d bytes", len(p.Result))
	}
}

func TestToolOutputTruncation_ZeroDisables(t *testing.T) {
	rt := testRuntime(t, core.NewMock())
	RegisterToolOutputTruncation(rt, 0)
	if rt.toolOutputLimit != 0 {
		t.Errorf("limit 0 must register nothing, got %d", rt.toolOutputLimit)
	}
}

func lastToolResult(t *testing.T, msgs []core.Message) string {
	t.Helper()
	for i := len(msgs) - 1; i >= 0; i-- {
		for _, b := range msgs[i].Content {
			if tr, ok := b.(core.ToolResultBlock); ok {
				return tr.Content
			}
		}
	}
	t.Fatal("no tool result in the messages sent")
	return ""
}
