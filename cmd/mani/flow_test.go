package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// scriptArg makes the test binary act as the script of a run: step, so the
// flow tests need no Python. See TestMain.
const scriptArg = "__flow_script"

// flowScript is every script the tests use. Each call appends its name to
// script.log in the working directory — the flow's directory — so a test can
// tell whether a code step ran again.
func flowScript(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "flowScript: no script named")
		return 2
	}
	if f, err := os.OpenFile("script.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
		fmt.Fprintln(f, args[0])
		f.Close()
	}

	switch args[0] {
	case "cat": // prints a JSONL file: a first step that fetches
		b, err := os.ReadFile(args[1])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		os.Stdout.Write(b)
		return 0

	case "tally": // collects the labels of every record on stdin: a reduce
		var labels []string
		sc := bufio.NewScanner(os.Stdin)
		sc.Buffer(make([]byte, 0, 64*1024), 8<<20)
		for sc.Scan() {
			var rec struct {
				Result struct {
					Label string `json:"label"`
				} `json:"result"`
			}
			if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 1
			}
			labels = append(labels, rec.Result.Label)
		}
		sort.Strings(labels)
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"id": "all", "task": map[string]any{"labels": labels}})
		return 0

	case "fail":
		fmt.Fprintln(os.Stderr, "boom from the script")
		return 3
	}
	fmt.Fprintf(os.Stderr, "flowScript: unknown script %q\n", args[0])
	return 2
}

// fakeLLM is an Ollama that answers every task with the respond tool and
// label "L:<task>", so each result says which task produced it. It records the
// tasks it saw and how many requests were in flight at once.
type fakeLLM struct {
	srv   *httptest.Server
	calls atomic.Int64

	mu    sync.Mutex
	tasks []string

	inFlight, maxInFlight atomic.Int64
	delay                 time.Duration
	failing               atomic.Bool
	fail                  func(task string) bool // HTTP 500 when failing is set and this returns true
}

func newFakeLLM(t *testing.T) *fakeLLM {
	t.Helper()
	f := &fakeLLM{}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeLLM) serve(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/chat" {
		http.NotFound(w, r)
		return
	}
	f.calls.Add(1)
	n := f.inFlight.Add(1)
	defer f.inFlight.Add(-1)
	for {
		m := f.maxInFlight.Load()
		if n <= m || f.maxInFlight.CompareAndSwap(m, n) {
			break
		}
	}
	time.Sleep(f.delay)

	var req struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	task := ""
	for _, m := range req.Messages {
		if m.Role == "user" {
			task = m.Content
		}
	}
	f.mu.Lock()
	f.tasks = append(f.tasks, task)
	f.mu.Unlock()

	if f.failing.Load() && f.fail != nil && f.fail(task) {
		http.Error(w, "scripted failure", http.StatusInternalServerError)
		return
	}

	msg := map[string]any{"role": "assistant", "content": "", "tool_calls": []map[string]any{
		{"function": map[string]any{"name": "respond", "arguments": map[string]any{"label": "L:" + task}}},
	}}
	w.Header().Set("Content-Type", "application/x-ndjson")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"message": msg, "done": true, "done_reason": "stop",
		"prompt_eval_count": 11, "eval_count": 7,
	})
}

func (f *fakeLLM) seen() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.tasks...)
}

// maniResult is one run of the real binary.
type maniResult struct {
	stdout, stderr string
	code           int
}

// maniIn runs the binary with a stdin and extra environment, and never fails
// the test: exit codes are what these tests are about.
func maniIn(t *testing.T, home, stdin string, env []string, args ...string) maniResult {
	t.Helper()
	cmd := exec.Command(os.Args[0], args...)
	cmd.Env = append(append(os.Environ(), "MANI_TEST_RUN_MAIN=1", "XDG_CONFIG_HOME="+home), env...)
	cmd.Stdin = strings.NewReader(stdin)
	var out, errb strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &errb

	res := maniResult{}
	err := cmd.Run()
	var exit *exec.ExitError
	switch {
	case errors.As(err, &exit):
		res.code = exit.ExitCode()
	case err != nil:
		t.Fatalf("mani %v: %v", args, err)
	}
	res.stdout, res.stderr = out.String(), errb.String()
	return res
}

func runMani(t *testing.T, home string, args ...string) maniResult {
	t.Helper()
	return maniIn(t, home, "", nil, args...)
}

// project writes a flow directory: agent.yaml (schema manifest, journal in
// ./runs) plus the given files. {{BIN}} in a file becomes the test binary,
// quoted for YAML.
func project(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	all := map[string]string{"agent.yaml": fmt.Sprintf(schemaManifest, filepath.Join(dir, "runs"))}
	for name, body := range files {
		all[name] = strings.ReplaceAll(body, "{{BIN}}", fmt.Sprintf("%q", os.Args[0]))
	}
	for name, body := range all {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// readRecord reads one record file.
func readRecord(t *testing.T, path string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var rec map[string]any
	if err := json.Unmarshal(b, &rec); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return rec
}

// jsonLines parses stdout as JSONL, failing on anything that is not a record.
func jsonLines(t *testing.T, stdout string) []map[string]any {
	t.Helper()
	var recs []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("stdout line is not a record: %q", line)
		}
		recs = append(recs, rec)
	}
	return recs
}

func label(rec map[string]any) string {
	res, _ := rec["result"].(map[string]any)
	s, _ := res["label"].(string)
	return s
}

func scriptLog(t *testing.T, dir string) string {
	t.Helper()
	b, _ := os.ReadFile(filepath.Join(dir, "script.log"))
	return string(b)
}

const lettersJSONL = `{"id": "a", "task": "one", "shelf": "s1"}
{"id": "b", "task": "two", "shelf": "s2"}
`

// lettersFlow is idea-letters in miniature: fetch (code) → extract (agent per
// record) → timeline (code over all) → synth (agent per record).
const lettersFlow = `
flow: letters
about: "rebuilds the timeline of the letters"
steps:
  - step: fetch
    does: "emits the letters"
    run: [{{BIN}}, __flow_script, cat, letters.jsonl]
  - step: extract
    does: "labels each letter"
    agent: agent.yaml
    for_each: fetch
    jobs: 2
  - step: timeline
    does: "collects the labels"
    run: [{{BIN}}, __flow_script, tally]
    from_all: extract
  - step: synth
    does: "writes the synthesis"
    agent: agent.yaml
    for_each: timeline
result: synth
`

func lettersProject(t *testing.T) string {
	return project(t, map[string]string{"f.flow.yaml": lettersFlow, "letters.jsonl": lettersJSONL})
}

const wantSynth = `L:{"labels":["L:one","L:two"]}`

func TestFlow_RunsStepsInOrderAndPrintsTheResult(t *testing.T) {
	llm := newFakeLLM(t)
	home := cliHome(t, llm.srv.URL)
	dir := lettersProject(t)
	out := filepath.Join(dir, "out")

	res := runMani(t, home, "run", "--config", filepath.Join(dir, "f.flow.yaml"), "--out", out)
	if res.code != 0 {
		t.Fatalf("exit %d\n%s", res.code, res.stderr)
	}

	recs := jsonLines(t, res.stdout)
	if len(recs) != 1 || recs[0]["id"] != "all" || label(recs[0]) != wantSynth {
		t.Fatalf("stdout = %v, want only the synth record with label %s", recs, wantSynth)
	}
	if got := llm.calls.Load(); got != 3 {
		t.Errorf("model calls = %d, want 3 (two extractions, one synthesis)", got)
	}
	if got := scriptLog(t, dir); got != "cat\ntally\n" {
		t.Errorf("scripts ran %q, want cat then tally", got)
	}
	for _, step := range []string{"fetch", "extract", "timeline", "synth"} {
		if _, err := os.Stat(filepath.Join(out, step)); err != nil {
			t.Errorf("no directory for step %s: %v", step, err)
		}
	}
	if !strings.Contains(res.stderr, "tokens: 33 in, 21 out") {
		t.Errorf("stderr has no token total:\n%s", res.stderr)
	}
}

func TestFlow_SecondRunDoesNothing(t *testing.T) {
	llm := newFakeLLM(t)
	home := cliHome(t, llm.srv.URL)
	dir := lettersProject(t)
	args := []string{"run", "--config", filepath.Join(dir, "f.flow.yaml"), "--out", filepath.Join(dir, "out")}

	if res := runMani(t, home, args...); res.code != 0 {
		t.Fatalf("first run: exit %d\n%s", res.code, res.stderr)
	}
	calls, log := llm.calls.Load(), scriptLog(t, dir)

	res := runMani(t, home, args...)
	if res.code != 0 {
		t.Fatalf("second run: exit %d\n%s", res.code, res.stderr)
	}
	if got := llm.calls.Load() - calls; got != 0 {
		t.Errorf("second run made %d model calls, want 0", got)
	}
	if got := scriptLog(t, dir); got != log {
		t.Errorf("second run ran scripts again: %q", strings.TrimPrefix(got, log))
	}
	// The result is the whole step, whether made now or before.
	if recs := jsonLines(t, res.stdout); len(recs) != 1 || label(recs[0]) != wantSynth {
		t.Errorf("second run stdout = %v, want the same result", recs)
	}
}

// Fetching again with one more letter must cost one extraction, not all of
// them: files that come out identical keep their time.
func TestFlow_NewRecordRedoesOnlyWhatDependsOnIt(t *testing.T) {
	llm := newFakeLLM(t)
	home := cliHome(t, llm.srv.URL)
	dir := lettersProject(t)
	out := filepath.Join(dir, "out")
	args := []string{"run", "--config", filepath.Join(dir, "f.flow.yaml"), "--out", out}

	if res := runMani(t, home, args...); res.code != 0 {
		t.Fatalf("first run: exit %d\n%s", res.code, res.stderr)
	}
	calls := llm.calls.Load()

	more := lettersJSONL + `{"id": "c", "task": "three"}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "letters.jsonl"), []byte(more), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(out, "fetch", ".done")); err != nil {
		t.Fatal(err)
	}

	res := runMani(t, home, args...)
	if res.code != 0 {
		t.Fatalf("second run: exit %d\n%s", res.code, res.stderr)
	}
	newTasks := llm.seen()[calls:]
	if len(newTasks) != 2 || newTasks[0] != "three" {
		t.Errorf("second run sent %q, want the new letter and then the synthesis", newTasks)
	}
	if recs := jsonLines(t, res.stdout); len(recs) != 1 || !strings.Contains(label(recs[0]), "L:three") {
		t.Errorf("stdout = %v, want a synthesis that includes the new letter", recs)
	}
}

// A code step that runs again and prints the same records must not make the
// steps after it run again.
func TestFlow_IdenticalScriptOutputDoesNotCascade(t *testing.T) {
	llm := newFakeLLM(t)
	home := cliHome(t, llm.srv.URL)
	dir := lettersProject(t)
	out := filepath.Join(dir, "out")
	args := []string{"run", "--config", filepath.Join(dir, "f.flow.yaml"), "--out", out}

	if res := runMani(t, home, args...); res.code != 0 {
		t.Fatalf("first run: exit %d\n%s", res.code, res.stderr)
	}
	calls := llm.calls.Load()
	synth := filepath.Join(out, "synth", "all.json")
	before, err := os.Stat(synth)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.Remove(filepath.Join(out, "timeline", ".done")); err != nil {
		t.Fatal(err)
	}
	if res := runMani(t, home, args...); res.code != 0 {
		t.Fatalf("second run: exit %d\n%s", res.code, res.stderr)
	}

	if !strings.HasSuffix(scriptLog(t, dir), "tally\ntally\n") {
		t.Errorf("the timeline step did not run again: %q", scriptLog(t, dir))
	}
	if got := llm.calls.Load() - calls; got != 0 {
		t.Errorf("an identical timeline cost %d model calls, want 0", got)
	}
	after, _ := os.Stat(synth)
	if !after.ModTime().Equal(before.ModTime()) {
		t.Error("the synthesis file was rewritten")
	}
}

func TestFlow_FailedRecordStopsBeforeTheNextStep(t *testing.T) {
	llm := newFakeLLM(t)
	llm.failing.Store(true)
	llm.fail = func(task string) bool { return task == "two" }
	home := cliHome(t, llm.srv.URL)
	dir := lettersProject(t)
	out := filepath.Join(dir, "out")
	args := []string{"run", "--config", filepath.Join(dir, "f.flow.yaml"), "--out", out}

	res := runMani(t, home, args...)
	if res.code != 1 {
		t.Fatalf("exit %d, want 1\n%s", res.code, res.stderr)
	}
	if got := scriptLog(t, dir); got != "cat\n" {
		t.Errorf("scripts ran %q: the timeline must not run on half the letters", got)
	}
	if !strings.Contains(res.stderr, filepath.Join(out, "extract", "errors.jsonl")) {
		t.Errorf("stderr does not point at errors.jsonl:\n%s", res.stderr)
	}
	if _, err := os.Stat(filepath.Join(out, "extract", "b.json")); err == nil {
		t.Error("the failed record left a result file: the next run would skip it")
	}
	if strings.TrimSpace(res.stdout) != "" {
		t.Errorf("stdout = %q, want nothing", res.stdout)
	}

	llm.failing.Store(false)
	calls := llm.calls.Load()
	if res := runMani(t, home, args...); res.code != 0 {
		t.Fatalf("retry: exit %d\n%s", res.code, res.stderr)
	}
	if got := llm.seen()[calls:]; len(got) != 2 || got[0] != "two" {
		t.Errorf("retry sent %q, want only the failed letter and then the synthesis", got)
	}
}

func TestFlow_ScriptFailureStopsTheFlow(t *testing.T) {
	llm := newFakeLLM(t)
	home := cliHome(t, llm.srv.URL)
	dir := project(t, map[string]string{"f.flow.yaml": `
flow: broken
about: "a script that fails"
steps:
  - step: fetch
    does: "fails"
    run: [{{BIN}}, __flow_script, fail]
  - step: extract
    does: "never runs"
    agent: agent.yaml
    for_each: fetch
result: extract
`})

	res := runMani(t, home, "run", "--config", filepath.Join(dir, "f.flow.yaml"))
	if res.code != 1 {
		t.Fatalf("exit %d, want 1\n%s", res.code, res.stderr)
	}
	if !strings.Contains(res.stderr, "boom from the script") || !strings.Contains(res.stderr, "step fetch") {
		t.Errorf("stderr must carry the script's own message and the step:\n%s", res.stderr)
	}
	if llm.calls.Load() != 0 {
		t.Error("the agent ran after its input failed")
	}
}

// --limit is for trying a flow on a sample: the steps after a limited one run
// on what there is, and the full run later redoes only what the sample lacked.
func TestFlow_LimitLetsTheSampleThrough(t *testing.T) {
	llm := newFakeLLM(t)
	home := cliHome(t, llm.srv.URL)
	dir := lettersProject(t)
	args := []string{"run", "--config", filepath.Join(dir, "f.flow.yaml"), "--out", filepath.Join(dir, "out")}

	res := runMani(t, home, append(args, "--limit", "1")...)
	if res.code != 0 {
		t.Fatalf("sample: exit %d\n%s", res.code, res.stderr)
	}
	recs := jsonLines(t, res.stdout)
	if len(recs) != 1 || label(recs[0]) != `L:{"labels":["L:one"]}` {
		t.Errorf("sample result = %v, want a synthesis of one letter", recs)
	}
	if !strings.Contains(res.stderr, "1 left for later") {
		t.Errorf("stderr does not say what was left:\n%s", res.stderr)
	}

	calls := llm.calls.Load()
	res = runMani(t, home, args...)
	if res.code != 0 {
		t.Fatalf("full: exit %d\n%s", res.code, res.stderr)
	}
	if got := llm.seen()[calls:]; len(got) != 2 || got[0] != "two" {
		t.Errorf("full run sent %q, want the missing letter and then the synthesis", got)
	}
	if recs := jsonLines(t, res.stdout); len(recs) != 1 || label(recs[0]) != wantSynth {
		t.Errorf("full result = %v", recs)
	}
}

// Each fake run costs 18 tokens and runs one at a time: with a cap of 20 the
// second run starts (18 < 20) and the third does not.
func TestFlow_TokenBudgetStopsNewRuns(t *testing.T) {
	llm := newFakeLLM(t)
	home := cliHome(t, llm.srv.URL)
	dir := project(t, map[string]string{"f.flow.yaml": `
flow: capped
about: "stops at the budget"
input: "one task per line"
steps:
  - step: label
    does: "labels"
    agent: agent.yaml
    for_each: input
result: label
limits: { tokens: 20 }
`, "in.jsonl": `{"id":"a","task":"one"}
{"id":"b","task":"two"}
{"id":"c","task":"three"}
`})

	res := runMani(t, home, "run", "--config", filepath.Join(dir, "f.flow.yaml"),
		"--in", filepath.Join(dir, "in.jsonl"), "--out", filepath.Join(dir, "out"))
	if res.code != 1 {
		t.Fatalf("exit %d, want 1\n%s", res.code, res.stderr)
	}
	if got := llm.calls.Load(); got != 2 {
		t.Errorf("model calls = %d, want 2", got)
	}
	if !strings.Contains(res.stderr, "token budget") {
		t.Errorf("stderr does not name the budget:\n%s", res.stderr)
	}
	if recs := jsonLines(t, res.stdout); len(recs) != 2 {
		t.Errorf("stdout has %d records, want the 2 that were made", len(recs))
	}
}

// One agent's structured result is the next agent's task, as JSON; the
// caller's own fields survive both hops.
func TestFlow_StructuredResultBecomesTheNextTaskAndExtrasTravel(t *testing.T) {
	llm := newFakeLLM(t)
	home := cliHome(t, llm.srv.URL)
	dir := project(t, map[string]string{"f.flow.yaml": `
flow: chain
about: "two agents in a row"
input: "one task per line"
steps:
  - step: first
    does: "labels"
    agent: agent.yaml
    for_each: input
  - step: second
    does: "labels the label"
    agent: agent.yaml
    for_each: first
result: second
`, "in.jsonl": `{"id":"a","task":"one","shelf":"s1"}` + "\n"})

	res := runMani(t, home, "run", "--config", filepath.Join(dir, "f.flow.yaml"), "--in", filepath.Join(dir, "in.jsonl"))
	if res.code != 0 {
		t.Fatalf("exit %d\n%s", res.code, res.stderr)
	}
	if got := llm.seen(); len(got) != 2 || got[1] != `{"label":"L:one"}` {
		t.Errorf("tasks = %q, want the first result as JSON for the second agent", got)
	}
	recs := jsonLines(t, res.stdout)
	if len(recs) != 1 || recs[0]["shelf"] != "s1" || recs[0]["id"] != "a" {
		t.Errorf("result = %v, want id and shelf carried through", recs)
	}
}

// from_all on an agent: one run, over the list of records, without the run
// bookkeeping.
func TestFlow_AgentFromAllGetsTheRecordsWithoutRun(t *testing.T) {
	llm := newFakeLLM(t)
	home := cliHome(t, llm.srv.URL)
	dir := project(t, map[string]string{"f.flow.yaml": `
flow: reduce
about: "an agent over all records"
input: "one task per line"
steps:
  - step: first
    does: "labels"
    agent: agent.yaml
    for_each: input
  - step: summary
    does: "summarizes"
    agent: agent.yaml
    from_all: first
result: summary
`, "in.jsonl": `{"id":"a","task":"one"}` + "\n" + `{"id":"b","task":"two"}` + "\n"})

	res := runMani(t, home, "run", "--config", filepath.Join(dir, "f.flow.yaml"), "--in", filepath.Join(dir, "in.jsonl"))
	if res.code != 0 {
		t.Fatalf("exit %d\n%s", res.code, res.stderr)
	}
	tasks := llm.seen()
	if len(tasks) != 3 {
		t.Fatalf("tasks = %q, want two labels and one summary", tasks)
	}
	var list []map[string]any
	if err := json.Unmarshal([]byte(tasks[2]), &list); err != nil {
		t.Fatalf("summary task is not a JSON list: %q", tasks[2])
	}
	if len(list) != 2 || list[0]["id"] != "a" || list[1]["id"] != "b" {
		t.Errorf("summary got %v, want both records by id", list)
	}
	for _, rec := range list {
		if _, ok := rec["run"]; ok {
			t.Errorf("record %v still carries run", rec["id"])
		}
	}
	if recs := jsonLines(t, res.stdout); len(recs) != 1 || recs[0]["id"] != "summary" {
		t.Errorf("result = %v, want one record named after the step", recs)
	}
}

// Without --out the flow still runs, and leaves nothing behind.
func TestFlow_WithoutOutLeavesNothingBehind(t *testing.T) {
	llm := newFakeLLM(t)
	home := cliHome(t, llm.srv.URL)
	dir := lettersProject(t)
	tmp := t.TempDir()

	res := maniIn(t, home, "", []string{"TMPDIR=" + tmp}, "run", "--config", filepath.Join(dir, "f.flow.yaml"))
	if res.code != 0 {
		t.Fatalf("exit %d\n%s", res.code, res.stderr)
	}
	if recs := jsonLines(t, res.stdout); len(recs) != 1 || label(recs[0]) != wantSynth {
		t.Errorf("stdout = %v", recs)
	}
	if !strings.Contains(res.stderr, "no --out") {
		t.Errorf("stderr does not warn that nothing is kept:\n%s", res.stderr)
	}
	left, _ := os.ReadDir(tmp)
	if len(left) != 0 {
		t.Errorf("left %d entries in the temporary directory", len(left))
	}
}

func TestRun_FlowAndAgentFlagsDoNotMix(t *testing.T) {
	llm := newFakeLLM(t)
	home := cliHome(t, llm.srv.URL)
	dir := project(t, map[string]string{
		"f.flow.yaml": lettersFlow,
		"in.flow.yaml": `
flow: withinput
about: "reads --in"
input: "tasks"
steps:
  - step: label
    does: "labels"
    agent: agent.yaml
    for_each: input
result: label
`,
		"letters.jsonl": lettersJSONL,
	})
	flow, inFlow, agent := filepath.Join(dir, "f.flow.yaml"), filepath.Join(dir, "in.flow.yaml"), filepath.Join(dir, "agent.yaml")
	in := filepath.Join(dir, "letters.jsonl")

	cases := []struct {
		name string
		args []string
		want string
	}{
		{"task on a flow", []string{"--config", flow, "--task", "x"}, "--task does not apply to a flow"},
		{"provenance on a flow", []string{"--config", flow, "--provenance"}, "--provenance does not apply to a flow"},
		{"in on an agent", []string{"--config", agent, "--task", "x", "--in", in}, "--in"},
		{"out on an agent", []string{"--config", agent, "--task", "x", "--out", dir}, "--out"},
		{"flow with input but no --in", []string{"--config", inFlow}, "reads its records from --in"},
		{"flow without input given --in", []string{"--config", flow, "--in", in}, "declares no input"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := runMani(t, home, append([]string{"run"}, tc.args...)...)
			if res.code != exitUsage {
				t.Errorf("exit %d, want %d\n%s", res.code, exitUsage, res.stderr)
			}
			if !strings.Contains(res.stderr, tc.want) {
				t.Errorf("stderr = %q, want it to contain %q", res.stderr, tc.want)
			}
		})
	}
	if llm.calls.Load() != 0 {
		t.Error("a usage error reached the model")
	}
}

func TestValidate_ReadsTheFlowAloud(t *testing.T) {
	dir := lettersProject(t)

	res := runMani(t, t.TempDir(), "validate", "--config", filepath.Join(dir, "f.flow.yaml"))
	if res.code != 0 {
		t.Fatalf("exit %d\n%s", res.code, res.stderr)
	}
	for _, want := range []string{
		"letters — rebuilds the timeline of the letters",
		"1. fetch ",
		"2. extract ",
		"agent agent.yaml, for each record of fetch (2 at a time)",
		", on all the records of extract",
		"labels each letter",
		"result: synth",
	} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("validate output lacks %q:\n%s", want, res.stdout)
		}
	}
}

func TestValidate_BrokenFlowIsAUsageError(t *testing.T) {
	dir := project(t, map[string]string{"f.flow.yaml": strings.Replace(lettersFlow, "for_each: fetch", "for_each: synth", 1)})

	res := runMani(t, t.TempDir(), "validate", "--config", filepath.Join(dir, "f.flow.yaml"))
	if res.code != exitUsage || !strings.Contains(res.stderr, "which comes later") {
		t.Errorf("exit %d, stderr %q", res.code, res.stderr)
	}
}
