package manifest

import (
	"strings"
	"testing"
)

const threeProblems = `identity:
  name: demo
  provider: ollama
  model: qwen3.5:9b
capabilities:
  tools:
    - name: read
    - name: teleport
policy:
  rules:
    - { tool: bash, pattern: "rm\\s+-rf", action: deny }
    - { tool: bash, pattern: "(unclosed", action: deny }
limits:
  max_duration: 2 minutes
`

func TestCheck_ReportsEveryProblemWithItsLine(t *testing.T) {
	_, ps := Check([]byte(threeProblems), t.TempDir())
	want := []struct {
		path string
		line int
		text string
	}{
		{"capabilities.tools[1]", 8, `tool unknown "teleport"`},
		{"policy.rules[1].pattern", 12, "invalid regex"},
		{"limits.max_duration", 14, "limits.max_duration"},
	}
	if len(ps) != len(want) {
		t.Fatalf("got %d problems, want %d: %+v", len(ps), len(want), ps)
	}
	for i, w := range want {
		if ps[i].Path != w.path || ps[i].Line != w.line || !strings.Contains(ps[i].Message, w.text) {
			t.Errorf("problem %d = %+v, want %s at line %d containing %q", i, ps[i], w.path, w.line, w.text)
		}
	}
}

// Validate keeps its old contract: the first problem, with the old wording.
func TestValidate_IsTheFirstProblem(t *testing.T) {
	spec, ps := Check([]byte(threeProblems), t.TempDir())
	err := spec.Validate()
	if err == nil || err.Error() != "[manifest]: "+ps[0].Message {
		t.Errorf("Validate = %v, want the first problem %q", err, ps[0].Message)
	}
}

func TestCheck_UnknownFieldsAllAtOnce(t *testing.T) {
	src := "context:\n  windw: 10\nlimits:\n  max_tokns: 10\n"
	_, ps := Check([]byte(src), t.TempDir())
	if len(ps) != 2 {
		t.Fatalf("got %+v, want two problems", ps)
	}
	if ps[0].Line != 2 || !strings.Contains(ps[0].Message, "windw") || ps[1].Line != 4 || !strings.Contains(ps[1].Message, "max_tokns") {
		t.Errorf("problems = %+v", ps)
	}
}

// identity has its own decoder, which stops at its first unknown field: one
// problem, still with its line.
func TestCheck_UnknownFieldInIdentity(t *testing.T) {
	_, ps := Check([]byte("identity:\n  name: demo\n  colour: red\n"), t.TempDir())
	if len(ps) != 1 || ps[0].Line != 3 || !strings.Contains(ps[0].Message, "colour") {
		t.Errorf("problems = %+v", ps)
	}
}

func TestCheck_SyntaxError(t *testing.T) {
	_, ps := Check([]byte("identity:\n  name: [unclosed\n"), t.TempDir())
	if len(ps) != 1 || ps[0].Line == 0 {
		t.Errorf("syntax error: %+v", ps)
	}
}

func TestCheck_AGoodManifestHasNoProblems(t *testing.T) {
	src := "identity:\n  name: demo\n  provider: ollama\n  model: m\ncapabilities:\n  tools: [read]\n"
	spec, ps := Check([]byte(src), t.TempDir())
	if len(ps) != 0 || spec.Identity.Name != "demo" {
		t.Errorf("problems = %+v, name = %q", ps, spec.Identity.Name)
	}
}

func TestLineAt_FallsBackToTheEnclosingField(t *testing.T) {
	_, written, err := parse([]byte("policy:\n  rules: []\n"), t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	if got := lineAt(written, "policy.rules[3].pattern"); got != 2 {
		t.Errorf("line of a missing rule = %d, want 2 (the rules key)", got)
	}
}
