package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// validFlow is the smallest flow that touches every kind of step: code with no
// input, an agent per record, code over all records, an agent per record again.
func validFlow() FlowSpec {
	return FlowSpec{
		Flow:  "letters",
		About: "rebuilds a timeline",
		Steps: []StepSpec{
			{Step: "fetch", Does: "downloads", Run: Command{"python", "fetch.py"}},
			{Step: "extract", Does: "extracts", Agent: "extract.yaml", ForEach: "fetch", Jobs: 4},
			{Step: "timeline", Does: "merges", Run: Command{"python", "merge.py"}, FromAll: "extract"},
			{Step: "synth", Does: "writes", Agent: "synth.yaml", ForEach: "timeline"},
		},
		Result: "synth",
	}
}

func TestFlowValidate(t *testing.T) {
	if err := validFlow().Validate(); err != nil {
		t.Fatalf("the reference flow must be valid: %v", err)
	}

	cases := []struct {
		name   string
		mutate func(f *FlowSpec)
		want   string // substring of the error
	}{
		{"flow name with a space", func(f *FlowSpec) { f.Flow = "my flow" }, `flow: "my flow" is not a name`},
		{"flow name uppercase", func(f *FlowSpec) { f.Flow = "Letters" }, "is not a name"},
		{"about missing", func(f *FlowSpec) { f.About = "  " }, "about:"},
		{"no steps", func(f *FlowSpec) { f.Steps = nil }, "at least one step"},
		{"step name with a space", func(f *FlowSpec) { f.Steps[0].Step = "fetch letters" }, `"fetch letters" is not a name`},
		{"step named input", func(f *FlowSpec) { f.Steps[0].Step = "input"; f.Steps[1].ForEach = "input"; f.Input = "x" }, "reserved"},
		{"step defined twice", func(f *FlowSpec) { f.Steps[2].Step = "extract" }, "step extract: defined twice"},
		{"does missing", func(f *FlowSpec) { f.Steps[1].Does = "" }, "step extract: does:"},
		{"neither agent nor run", func(f *FlowSpec) { f.Steps[1].Agent = "" }, "step extract: needs exactly one of agent or run"},
		{"both agent and run", func(f *FlowSpec) { f.Steps[1].Run = Command{"x"} }, "step extract: needs exactly one of agent or run"},
		{"for_each and from_all", func(f *FlowSpec) { f.Steps[1].FromAll = "fetch" }, "step extract: reads with for_each or with from_all"},
		{"agent without input", func(f *FlowSpec) { f.Steps[1].ForEach = "" }, "step extract: an agent needs something to work on"},
		{"negative jobs", func(f *FlowSpec) { f.Steps[1].Jobs = -1 }, "step extract: jobs cannot be negative"},
		{"jobs on a code step", func(f *FlowSpec) { f.Steps[0].Jobs = 2 }, "step fetch: jobs:"},
		{"jobs with from_all", func(f *FlowSpec) {
			f.Steps[3].ForEach, f.Steps[3].FromAll, f.Steps[3].Jobs = "", "timeline", 2
		}, "step synth: jobs:"},
		{"reads input that is not declared", func(f *FlowSpec) { f.Steps[1].ForEach = "input" }, "step extract: reads \"input\", but the flow declares no input"},
		{"reads itself", func(f *FlowSpec) { f.Steps[1].ForEach = "extract" }, "step extract: reads itself"},
		{"reads a missing step", func(f *FlowSpec) { f.Steps[1].ForEach = "fecth" }, "step extract: reads fecth, which is not a step"},
		{"reads a step below it", func(f *FlowSpec) { f.Steps[1].ForEach = "synth" }, "step extract: reads synth, which comes later"},
		{"input declared but unread", func(f *FlowSpec) { f.Input = "letters" }, "input: declared, but no step reads it"},
		{"result not a step", func(f *FlowSpec) { f.Result = "report" }, `result: "report" is not a step`},
		{"negative budget", func(f *FlowSpec) { f.Limits.Tokens = -1 }, "limits.tokens"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := validFlow()
			tc.mutate(&f)
			err := f.Validate()
			if err == nil {
				t.Fatalf("Validate() = nil, want an error containing %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Validate() = %q, want it to contain %q", err, tc.want)
			}
		})
	}
}

// The input is the one name a step may read without it being written above.
func TestFlowValidate_InputIsReadableByAnyStep(t *testing.T) {
	f := FlowSpec{
		Flow: "batch", About: "x", Input: "tasks",
		Steps: []StepSpec{
			{Step: "a", Does: "x", Agent: "a.yaml", ForEach: FlowInput},
			{Step: "b", Does: "x", Run: Command{"cat"}, FromAll: FlowInput},
		},
		Result: "a",
	}
	if err := f.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestCommand_StringAndList(t *testing.T) {
	var st StepSpec
	if err := yaml.Unmarshal([]byte("run: python  merge.py --busta 12\n"), &st); err != nil {
		t.Fatal(err)
	}
	if want := []string{"python", "merge.py", "--busta", "12"}; strings.Join(st.Run, "|") != strings.Join(want, "|") {
		t.Errorf("string form = %q, want %q", st.Run, want)
	}

	st = StepSpec{}
	if err := yaml.Unmarshal([]byte(`run: [python, "my script.py"]`+"\n"), &st); err != nil {
		t.Fatal(err)
	}
	if len(st.Run) != 2 || st.Run[1] != "my script.py" {
		t.Errorf("list form = %q, want the argument with a space kept whole", st.Run)
	}

	st = StepSpec{}
	if err := yaml.Unmarshal([]byte("run: {cmd: x}\n"), &st); err == nil {
		t.Error("a mapping must be refused")
	}
}

func TestIsFlowFile(t *testing.T) {
	dir := t.TempDir()
	cases := map[string]struct {
		body string
		want bool
	}{
		"flow.yaml":  {"flow: letters\nabout: x\n", true},
		"agent.yaml": {"identity:\n  name: a\n  prompt: !include ./p.md\n", false},
		"empty.yaml": {"", false},
	}
	for name, tc := range cases {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(tc.body), 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := IsFlowFile(path)
		if err != nil {
			t.Errorf("%s: %v", name, err)
		}
		if got != tc.want {
			t.Errorf("%s: IsFlowFile = %v, want %v", name, got, tc.want)
		}
	}

	if _, err := IsFlowFile(filepath.Join(dir, "missing.yaml")); err == nil {
		t.Error("a missing file must be an error, not an agent")
	}
}

const flowAgent = `
identity:
  name: extractor
  provider: ollama
  model: test-model
  prompt: "extract"
`

// writeFlowDir writes files into a fresh directory and returns it.
func writeFlowDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

const twoStepFlow = `
flow: letters
about: "extracts the letters"
input: "one letter per line"
steps:
  - step: extract
    does: "extracts one letter"
    agent: agent.yaml
    for_each: input
result: extract
`

// The test runs from the package directory, not the flow's: agent: must be
// resolved against the flow file, or a flow would only run from its own folder.
func TestLoadFlow_LoadsItsAgentsRelativeToTheFlow(t *testing.T) {
	dir := writeFlowDir(t, map[string]string{"f.yaml": twoStepFlow, "agent.yaml": flowAgent})

	f, err := LoadFlow(filepath.Join(dir, "f.yaml"))
	if err != nil {
		t.Fatalf("LoadFlow: %v", err)
	}
	if f.Steps[0].Agent != "agent.yaml" {
		t.Errorf("agent = %q: the spec keeps the path as written", f.Steps[0].Agent)
	}
}

func TestLoadFlow_ABrokenAgentNamesTheStep(t *testing.T) {
	dir := writeFlowDir(t, map[string]string{
		"f.yaml":     twoStepFlow,
		"agent.yaml": flowAgent + "surprise: true\n",
	})

	_, err := LoadFlow(filepath.Join(dir, "f.yaml"))
	if err == nil || !strings.Contains(err.Error(), "step extract") || !strings.Contains(err.Error(), "surprise") {
		t.Errorf("err = %v, want it to name the step and the bad field", err)
	}
}

func TestLoadFlow_MissingAgentFileNamesTheStep(t *testing.T) {
	dir := writeFlowDir(t, map[string]string{"f.yaml": twoStepFlow})

	_, err := LoadFlow(filepath.Join(dir, "f.yaml"))
	if err == nil || !strings.Contains(err.Error(), "step extract") {
		t.Errorf("err = %v, want it to name the step", err)
	}
}

func TestLoadFlow_ExpandsEnvInValues(t *testing.T) {
	t.Setenv("BUSTA", "12")
	dir := writeFlowDir(t, map[string]string{"f.yaml": `
flow: letters
about: "downloads"
steps:
  - step: fetch
    does: "downloads a busta"
    run: python fetch.py --busta ${BUSTA}
result: fetch
`})

	f, err := LoadFlow(filepath.Join(dir, "f.yaml"))
	if err != nil {
		t.Fatalf("LoadFlow: %v", err)
	}
	if got := f.Steps[0].Run; len(got) != 4 || got[3] != "12" {
		t.Errorf("run = %q, want ${BUSTA} expanded to 12", got)
	}
}

// Same rule as the manifest: an unset variable is an error, never "".
func TestLoadFlow_UnsetVariableIsAnError(t *testing.T) {
	dir := writeFlowDir(t, map[string]string{"f.yaml": `
flow: letters
about: "downloads"
steps:
  - step: fetch
    does: "downloads a busta"
    run: python fetch.py --busta ${MANI_TEST_SURELY_UNSET}
result: fetch
`})

	_, err := LoadFlow(filepath.Join(dir, "f.yaml"))
	if err == nil || !strings.Contains(err.Error(), "MANI_TEST_SURELY_UNSET") {
		t.Errorf("err = %v, want it to name the variable", err)
	}
}

// A typo in a key must fail with the key and its line, not be ignored: a
// misspelled from_all would silently turn a reduce into a first step.
func TestLoadFlow_UnknownFieldNamesItsLine(t *testing.T) {
	dir := writeFlowDir(t, map[string]string{"f.yaml": `flow: letters
about: "downloads"
steps:
  - step: fetch
    does: "downloads"
    run: cat
  - step: merge
    does: "merges"
    run: cat
    form_all: fetch
result: merge
`})

	_, err := LoadFlow(filepath.Join(dir, "f.yaml"))
	if err == nil || !strings.Contains(err.Error(), "form_all") || !strings.Contains(err.Error(), "line 10") {
		t.Errorf("err = %v, want the unknown field and line 10", err)
	}
}

func TestLoadFlow_InvalidFlowNamesTheFile(t *testing.T) {
	dir := writeFlowDir(t, map[string]string{"f.yaml": "flow: letters\nabout: x\nsteps: []\nresult: x\n"})
	path := filepath.Join(dir, "f.yaml")

	_, err := LoadFlow(path)
	if err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "at least one step") {
		t.Errorf("err = %v, want the file and the reason", err)
	}
}
