package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const paramsFlow = `
flow: letters
about: "rebuilds a busta"
params:
  busta: "The busta to rebuild"
  since: { about: "Earliest year", default: "1490" }
steps:
  - step: fetch
    does: "says what it was asked for"
    run: [{{BIN}}, __flow_script, echo, "busta=${params.busta}", "since=${params.since}"]
result: fetch
`

func TestFlow_ParamsReachTheCommand(t *testing.T) {
	dir := project(t, map[string]string{"f.flow.yaml": paramsFlow})

	res := runMani(t, t.TempDir(), "run", "--config", filepath.Join(dir, "f.flow.yaml"), "--set", "busta=12")
	if res.code != 0 {
		t.Fatalf("exit %d\n%s", res.code, res.stderr)
	}
	recs := jsonLines(t, res.stdout)
	if len(recs) != 1 || recs[0]["task"] != "busta=12 since=1490" {
		t.Errorf("stdout = %v, want the value from --set and the default", recs)
	}
}

func TestFlow_AMissingParamRefusesToRun(t *testing.T) {
	dir := project(t, map[string]string{"f.flow.yaml": paramsFlow})

	res := runMani(t, t.TempDir(), "run", "--config", filepath.Join(dir, "f.flow.yaml"))
	if res.code != exitUsage {
		t.Fatalf("exit %d, want %d\n%s", res.code, exitUsage, res.stderr)
	}
	for _, want := range []string{"missing parameters", "busta", "The busta to rebuild", "--set busta="} {
		if !strings.Contains(res.stderr, want) {
			t.Errorf("stderr lacks %q:\n%s", want, res.stderr)
		}
	}
	if scriptLog(t, dir) != "" {
		t.Error("the script ran without its parameter")
	}
}

func TestFlow_SetFlagErrors(t *testing.T) {
	dir := project(t, map[string]string{"f.flow.yaml": paramsFlow})
	flow := filepath.Join(dir, "f.flow.yaml")
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"undeclared", []string{"run", "--config", flow, "--set", "busta=12", "--set", "bust=1"}, "--set bust: the flow declares no such parameter"},
		{"no equals", []string{"run", "--config", flow, "--set", "busta"}, `expected name=value`},
		{"twice", []string{"run", "--config", flow, "--set", "busta=1", "--set", "busta=2"}, "--set busta: given twice"},
		{"on a manifest", []string{"run", "--config", filepath.Join(dir, "agent.yaml"), "--task", "x", "--set", "a=b"}, "--set does not apply to a manifest"},
		{"validate on a manifest", []string{"validate", "--config", filepath.Join(dir, "agent.yaml"), "--set", "a=b"}, "--set is for flows"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := runMani(t, t.TempDir(), tc.args...)
			if res.code != exitUsage {
				t.Errorf("exit %d, want %d", res.code, exitUsage)
			}
			if !strings.Contains(res.stderr, tc.want) {
				t.Errorf("stderr = %q, want %q", res.stderr, tc.want)
			}
		})
	}
}

// D4. Without the record, rerunning busta=13 in the directory of busta=12 finds
// every step "already done" and hands back busta 12's letters.
func TestFlow_TheStateDirectoryBelongsToItsParams(t *testing.T) {
	dir := project(t, map[string]string{"f.flow.yaml": paramsFlow})
	flow, out := filepath.Join(dir, "f.flow.yaml"), filepath.Join(dir, "out")

	if res := runMani(t, t.TempDir(), "run", "--config", flow, "--out", out, "--set", "busta=12"); res.code != 0 {
		t.Fatalf("first run: exit %d\n%s", res.code, res.stderr)
	}
	b, err := os.ReadFile(filepath.Join(out, ".params.json"))
	if err != nil {
		t.Fatalf("no .params.json: %v", err)
	}
	var recorded map[string]string
	if err := json.Unmarshal(b, &recorded); err != nil || recorded["busta"] != "12" || recorded["since"] != "1490" {
		t.Fatalf(".params.json = %s, want busta=12 and since=1490", b)
	}

	res := runMani(t, t.TempDir(), "run", "--config", flow, "--out", out, "--set", "busta=13")
	if res.code != exitUsage {
		t.Fatalf("rerun with another value: exit %d, want %d\n%s", res.code, exitUsage, res.stderr)
	}
	if !strings.Contains(res.stderr, "was built with busta=12, and this run has busta=13") {
		t.Errorf("stderr = %q, want both values named", res.stderr)
	}

	// The default counts too: changing it changes what the directory holds.
	res = runMani(t, t.TempDir(), "run", "--config", flow, "--out", out, "--set", "busta=12", "--set", "since=1501")
	if res.code != exitUsage || !strings.Contains(res.stderr, "since=1490, and this run has since=1501") {
		t.Errorf("a changed default must be refused too: exit %d\n%s", res.code, res.stderr)
	}

	log := scriptLog(t, dir)
	res = runMani(t, t.TempDir(), "run", "--config", flow, "--out", out, "--set", "busta=12")
	if res.code != 0 {
		t.Fatalf("rerun with the same values: exit %d\n%s", res.code, res.stderr)
	}
	if scriptLog(t, dir) != log {
		t.Error("the same values must resume, not redo")
	}
}

// Without --out there is no state to protect, so nothing is recorded.
func TestFlow_ParamsWithoutOutLeaveNoRecord(t *testing.T) {
	dir := project(t, map[string]string{"f.flow.yaml": paramsFlow})
	if res := runMani(t, t.TempDir(), "run", "--config", filepath.Join(dir, "f.flow.yaml"), "--set", "busta=12"); res.code != 0 {
		t.Fatalf("exit %d\n%s", res.code, res.stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, ".params.json")); err == nil {
		t.Error(".params.json written outside --out")
	}
}

func TestValidate_ListsTheParams(t *testing.T) {
	dir := project(t, map[string]string{"f.flow.yaml": paramsFlow})
	flow := filepath.Join(dir, "f.flow.yaml")

	res := runMani(t, t.TempDir(), "validate", "--config", flow)
	if res.code != 0 {
		t.Fatalf("validate must show a flow with a missing parameter: exit %d\n%s", res.code, res.stderr)
	}
	for _, want := range []string{"params:", "busta", "(not set)", "required", "The busta to rebuild", "since", "1490", "default"} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("validate lacks %q:\n%s", want, res.stdout)
		}
	}

	res = runMani(t, t.TempDir(), "validate", "--config", flow, "--set", "busta=12")
	if res.code != 0 || !strings.Contains(res.stdout, "--set") || !strings.Contains(res.stdout, "busta=12") {
		t.Errorf("validate --set must show the value and where it came from: exit %d\n%s", res.code, res.stdout)
	}
}
