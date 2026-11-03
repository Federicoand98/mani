package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const threeInputs = `{"id":"a","task":"one"}
{"id":"b","task":"two"}
{"id":"c","task":"three"}
`

// The labeller answers L:<task>, so a where on label picks a known record.
const whereFlow = `
flow: review
about: "labels every task, then reviews one"
input: "tasks"
steps:
  - step: label
    does: "labels"
    agent: agent.yaml
    for_each: input
  - step: review
    does: "reviews the matching one"
    agent: agent.yaml
    for_each: label
    where: { label: "L:two" }
result: review
`

func TestFlow_WhereRunsOnlyTheMatchingRecords(t *testing.T) {
	llm := newFakeLLM(t)
	home := cliHome(t, llm.srv.URL)
	dir := project(t, map[string]string{"f.flow.yaml": whereFlow, "in.jsonl": threeInputs})
	args := []string{"run", "--config", filepath.Join(dir, "f.flow.yaml"), "--in", filepath.Join(dir, "in.jsonl"), "--out", filepath.Join(dir, "out")}

	res := runMani(t, home, args...)
	if res.code != 0 {
		t.Fatalf("exit %d\n%s", res.code, res.stderr)
	}
	tasks := llm.seen()
	if len(tasks) != 4 || tasks[3] != `{"label":"L:two"}` {
		t.Fatalf("model saw %q, want three labels and one review of b", tasks)
	}
	recs := jsonLines(t, res.stdout)
	if len(recs) != 1 || recs[0]["id"] != "b" {
		t.Errorf("stdout = %v, want only b", recs)
	}
	if !strings.Contains(res.stderr, "review: 1 to run, 0 already done, 2 not matching where") {
		t.Errorf("stderr does not count the filtered records:\n%s", res.stderr)
	}

	// A record left out is not missing: the next run does not retry it.
	calls := llm.calls.Load()
	if res := runMani(t, home, args...); res.code != 0 {
		t.Fatalf("rerun: exit %d\n%s", res.code, res.stderr)
	}
	if got := llm.calls.Load() - calls; got != 0 {
		t.Errorf("rerun made %d model calls, want 0: filtered records are not pending", got)
	}
}

// A record that matched before and no longer does: its old result must leave
// the step's directory, or the directory stops meaning "this step's records".
func TestFlow_WhereRemovesAResultThatNoLongerMatches(t *testing.T) {
	llm := newFakeLLM(t)
	home := cliHome(t, llm.srv.URL)
	dir := project(t, map[string]string{"f.flow.yaml": whereFlow, "in.jsonl": threeInputs})
	out := filepath.Join(dir, "out")
	args := []string{"run", "--config", filepath.Join(dir, "f.flow.yaml"), "--in", filepath.Join(dir, "in.jsonl"), "--out", out}

	if res := runMani(t, home, args...); res.code != 0 {
		t.Fatalf("first run: exit %d\n%s", res.code, res.stderr)
	}
	reviewed := filepath.Join(out, "review", "b.json")
	if _, err := os.Stat(reviewed); err != nil {
		t.Fatalf("no review of b after the first run: %v", err)
	}

	// b changes, and its label with it: L:zwei no longer matches.
	if err := os.WriteFile(filepath.Join(dir, "in.jsonl"), []byte(strings.Replace(threeInputs, `"two"`, `"zwei"`, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(out, "label", "b.json")); err != nil {
		t.Fatal(err)
	}

	res := runMani(t, home, args...)
	if res.code != 0 {
		t.Fatalf("second run: exit %d\n%s", res.code, res.stderr)
	}
	if _, err := os.Stat(reviewed); err == nil {
		t.Error("the review of b is still there after b stopped matching")
	}
	if !strings.Contains(res.stderr, "removed 1 results that no longer match where") {
		t.Errorf("stderr does not say what it removed:\n%s", res.stderr)
	}
	if recs := jsonLines(t, res.stdout); len(recs) != 0 {
		t.Errorf("stdout = %v, want nothing left to review", recs)
	}
}

// A payload with no fields cannot be filtered on a field: that is an error,
// not a silent "nothing matched".
func TestFlow_WhereOnATextPayloadIsAnError(t *testing.T) {
	llm := newFakeLLM(t)
	home := cliHome(t, llm.srv.URL)
	dir := project(t, map[string]string{"f.flow.yaml": `
flow: review
about: "filters the input directly"
input: "tasks"
steps:
  - step: review
    does: "reviews some"
    agent: agent.yaml
    for_each: input
    where: { kind: letter }
result: review
`, "in.jsonl": threeInputs})

	res := runMani(t, home, "run", "--config", filepath.Join(dir, "f.flow.yaml"), "--in", filepath.Join(dir, "in.jsonl"))
	if res.code != exitRuntime {
		t.Fatalf("exit %d, want %d\n%s", res.code, exitRuntime, res.stderr)
	}
	if !strings.Contains(res.stderr, "record a: task: where needs an object") {
		t.Errorf("stderr does not name the record and the field:\n%s", res.stderr)
	}
	if llm.calls.Load() != 0 {
		t.Error("the model was called before the filter failed")
	}
}

// A where on an object task — what a script or --in can carry — works on the
// task's fields.
func TestFlow_WhereOnObjectTasks(t *testing.T) {
	llm := newFakeLLM(t)
	home := cliHome(t, llm.srv.URL)
	dir := project(t, map[string]string{"f.flow.yaml": `
flow: review
about: "filters the input directly"
input: "tasks"
steps:
  - step: review
    does: "reviews the letters only"
    agent: agent.yaml
    for_each: input
    where: { kind: letter }
result: review
`, "in.jsonl": `{"id":"a","task":{"kind":"letter","text":"one"}}
{"id":"b","task":{"kind":"note","text":"two"}}
`})

	res := runMani(t, home, "run", "--config", filepath.Join(dir, "f.flow.yaml"), "--in", filepath.Join(dir, "in.jsonl"))
	if res.code != 0 {
		t.Fatalf("exit %d\n%s", res.code, res.stderr)
	}
	if got := llm.seen(); len(got) != 1 || !strings.Contains(got[0], `"text":"one"`) {
		t.Errorf("model saw %q, want only the letter", got)
	}
}

const fanInFlow = `
flow: join
about: "joins letters and people"
steps:
  - step: letters
    does: "emits the letters"
    run: [{{BIN}}, __flow_script, cat, letters.jsonl]
  - step: people
    does: "emits the people"
    run: [{{BIN}}, __flow_script, cat, people.jsonl]
  - step: joined
    does: "receives both"
    run: [{{BIN}}, __flow_script, sources]
    from_all: [letters, people]
result: joined
`

// The same id can exist in two sources — doc a among the letters, person a in
// the register — and the fan-in keeps them apart by where they came from.
func TestFlow_FanInTagsEveryRecordWithItsSource(t *testing.T) {
	dir := project(t, map[string]string{
		"f.flow.yaml":   fanInFlow,
		"letters.jsonl": `{"id":"a","task":"l1"}` + "\n" + `{"id":"b","task":"l2"}` + "\n",
		"people.jsonl":  `{"id":"a","task":"p1"}` + "\n",
	})

	res := runMani(t, t.TempDir(), "run", "--config", filepath.Join(dir, "f.flow.yaml"))
	if res.code != 0 {
		t.Fatalf("exit %d\n%s", res.code, res.stderr)
	}
	recs := jsonLines(t, res.stdout)
	if len(recs) != 1 {
		t.Fatalf("stdout = %v, want one record", recs)
	}
	got, _ := json.Marshal(recs[0]["task"])
	if string(got) != `["letters/a=l1","letters/b=l2","people/a=p1"]` {
		t.Errorf("the fan-in received %s, want every record tagged with its source, sources in the order written", got)
	}
}

// "from" is an ordinary word, a letter has a sender: replacing one is said.
func TestFlow_FanInSaysWhenItReplacesAFromField(t *testing.T) {
	dir := project(t, map[string]string{
		"f.flow.yaml":   fanInFlow,
		"letters.jsonl": `{"id":"a","task":"l1","from":"Isabella"}` + "\n",
		"people.jsonl":  `{"id":"a","task":"p1"}` + "\n",
	})

	res := runMani(t, t.TempDir(), "run", "--config", filepath.Join(dir, "f.flow.yaml"))
	if res.code != 0 {
		t.Fatalf("exit %d\n%s", res.code, res.stderr)
	}
	if !strings.Contains(res.stderr, `1 records already had a "from" field`) {
		t.Errorf("stderr does not mention the replaced field:\n%s", res.stderr)
	}
	if !strings.Contains(res.stdout, "letters/a=l1") {
		t.Errorf("stdout = %s, want the record tagged with its step", res.stdout)
	}
}

// A whole-step run over a fan-in is fresh only if it is newer than every
// source, so a change in any one of them reruns it.
func TestFlow_FanInRerunsWhenAnySourceChanges(t *testing.T) {
	dir := project(t, map[string]string{
		"f.flow.yaml":   fanInFlow,
		"letters.jsonl": `{"id":"a","task":"l1"}` + "\n",
		"people.jsonl":  `{"id":"a","task":"p1"}` + "\n",
	})
	out := filepath.Join(dir, "out")
	args := []string{"run", "--config", filepath.Join(dir, "f.flow.yaml"), "--out", out}

	if res := runMani(t, t.TempDir(), args...); res.code != 0 {
		t.Fatalf("first run: exit %d\n%s", res.code, res.stderr)
	}
	if err := os.WriteFile(filepath.Join(dir, "people.jsonl"), []byte(`{"id":"a","task":"p2"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(out, "people", ".done")); err != nil {
		t.Fatal(err)
	}

	res := runMani(t, t.TempDir(), args...)
	if res.code != 0 {
		t.Fatalf("second run: exit %d\n%s", res.code, res.stderr)
	}
	if !strings.Contains(res.stdout, "people/a=p2") {
		t.Errorf("stdout = %s, want the fan-in rerun on the new people", res.stdout)
	}
	if strings.Count(scriptLog(t, dir), "sources\n") != 2 {
		t.Errorf("the fan-in ran %d times, want 2", strings.Count(scriptLog(t, dir), "sources\n"))
	}
}

// An agent over a fan-in receives the JSON list of the records, each tagged.
func TestFlow_AgentFanInGetsTaggedRecords(t *testing.T) {
	llm := newFakeLLM(t)
	home := cliHome(t, llm.srv.URL)
	dir := project(t, map[string]string{
		"f.flow.yaml":   strings.Replace(fanInFlow, "run: [{{BIN}}, __flow_script, sources]", "agent: agent.yaml", 1),
		"letters.jsonl": `{"id":"a","task":"l1"}` + "\n",
		"people.jsonl":  `{"id":"a","task":"p1"}` + "\n",
	})

	res := runMani(t, home, "run", "--config", filepath.Join(dir, "f.flow.yaml"))
	if res.code != 0 {
		t.Fatalf("exit %d\n%s", res.code, res.stderr)
	}
	tasks := llm.seen()
	if len(tasks) != 1 {
		t.Fatalf("model saw %q, want one task", tasks)
	}
	var list []map[string]any
	if err := json.Unmarshal([]byte(tasks[0]), &list); err != nil {
		t.Fatalf("task is not a JSON list: %q", tasks[0])
	}
	if len(list) != 2 || list[0]["from"] != "letters" || list[1]["from"] != "people" {
		t.Errorf("agent received %v, want both records tagged", list)
	}
}

func TestValidate_ReadsWhereAndFanInAloud(t *testing.T) {
	dir := project(t, map[string]string{"w.flow.yaml": whereFlow, "j.flow.yaml": fanInFlow})

	res := runMani(t, t.TempDir(), "validate", "--config", filepath.Join(dir, "w.flow.yaml"))
	if res.code != 0 || !strings.Contains(res.stdout, "for each record of label where label is L:two") {
		t.Errorf("exit %d, validate does not read the where aloud:\n%s%s", res.code, res.stdout, res.stderr)
	}

	res = runMani(t, t.TempDir(), "validate", "--config", filepath.Join(dir, "j.flow.yaml"))
	if res.code != 0 || !strings.Contains(res.stdout, "on all the records of letters and people") {
		t.Errorf("exit %d, validate does not read the fan-in aloud:\n%s%s", res.code, res.stdout, res.stderr)
	}
}

// A whole-step run behind a where must rerun when a record stops matching,
// even though that record is no longer among its inputs: freshness is measured
// against every upstream record, kept or not, or the step would keep a summary
// of a record that is gone.
func TestFlow_WhereRerunsAWholeStepWhenARecordStopsMatching(t *testing.T) {
	llm := newFakeLLM(t)
	home := cliHome(t, llm.srv.URL)
	dir := project(t, map[string]string{"f.flow.yaml": `
flow: review
about: "labels every task, then summarises the matching ones"
input: "tasks"
steps:
  - step: label
    does: "labels"
    agent: agent.yaml
    for_each: input
  - step: summary
    does: "lists the matching ones"
    run: [{{BIN}}, __flow_script, sources]
    from_all: label
    where: { label: "L:two" }
result: summary
`, "in.jsonl": threeInputs})
	out := filepath.Join(dir, "out")
	args := []string{"run", "--config", filepath.Join(dir, "f.flow.yaml"), "--in", filepath.Join(dir, "in.jsonl"), "--out", out}

	res := runMani(t, home, args...)
	if res.code != 0 {
		t.Fatalf("first run: exit %d\n%s", res.code, res.stderr)
	}
	if !strings.Contains(res.stdout, "/b=") {
		t.Fatalf("first summary = %s, want b in it", res.stdout)
	}

	if err := os.WriteFile(filepath.Join(dir, "in.jsonl"), []byte(strings.Replace(threeInputs, `"two"`, `"zwei"`, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(out, "label", "b.json")); err != nil {
		t.Fatal(err)
	}

	res = runMani(t, home, args...)
	if res.code != 0 {
		t.Fatalf("second run: exit %d\n%s", res.code, res.stderr)
	}
	if strings.Count(scriptLog(t, dir), "sources\n") != 2 {
		t.Errorf("the summary did not rerun after b stopped matching:\n%s", res.stderr)
	}
	if strings.Contains(res.stdout, "/b=") {
		t.Errorf("summary = %s, still lists b", res.stdout)
	}
}
