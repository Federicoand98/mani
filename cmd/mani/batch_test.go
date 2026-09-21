package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Federicoand98/mani/app"
)

const threeTasks = `{"id": "a", "task": "one", "shelf": "s1"}
{"id": "b", "task": "two", "shelf": "s2"}
{"id": "c", "task": "three", "shelf": "s3"}
`

// batchProject writes agent.yaml and in.jsonl, and returns their directory.
func batchProject(t *testing.T, input string) string {
	return project(t, map[string]string{"in.jsonl": input})
}

func batchArgs(dir string, extra ...string) []string {
	return append([]string{"batch", "--config", filepath.Join(dir, "agent.yaml"), "--in", filepath.Join(dir, "in.jsonl")}, extra...)
}

func TestBatch_WritesOneFilePerTask(t *testing.T) {
	llm := newFakeLLM(t)
	home := cliHome(t, llm.srv.URL)
	dir := batchProject(t, threeTasks)
	out := filepath.Join(dir, "out")

	res := runMani(t, home, batchArgs(dir, "--out", out)...)
	if res.code != 0 {
		t.Fatalf("exit %d\n%s", res.code, res.stderr)
	}
	if strings.TrimSpace(res.stdout) != "" {
		t.Errorf("with --out the results are files, stdout = %q", res.stdout)
	}
	if _, err := os.Stat(filepath.Join(out, "tasks")); err == nil {
		t.Error("the batch wrote into a step directory: its files belong straight in --out")
	}

	j, err := app.NewJSONLJournal(filepath.Join(dir, "runs"))
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()

	for id, want := range map[string][2]string{"a": {"one", "s1"}, "b": {"two", "s2"}, "c": {"three", "s3"}} {
		rec := readRecord(t, filepath.Join(out, id+".json"))
		if label(rec) != "L:"+want[0] || rec["shelf"] != want[1] {
			t.Errorf("%s = %v, want label L:%s and shelf %s", id, rec, want[0], want[1])
		}
		run, _ := rec["run"].(map[string]any)
		if run["source"] != "batch" {
			t.Errorf("%s: run.source = %v, want batch", id, run["source"])
		}
		runID, _ := run["id"].(string)
		got, err := j.Get(runID)
		if err != nil {
			t.Errorf("%s: run %q not in the journal: %v", id, runID, err)
			continue
		}
		if got.Source != "batch" {
			t.Errorf("%s: journal source = %q, want batch", id, got.Source)
		}
	}
}

func TestBatch_StreamsJSONLWithoutOut(t *testing.T) {
	llm := newFakeLLM(t)
	home := cliHome(t, llm.srv.URL)
	dir := batchProject(t, threeTasks)

	res := runMani(t, home, batchArgs(dir)...)
	if res.code != 0 {
		t.Fatalf("exit %d\n%s", res.code, res.stderr)
	}
	recs := jsonLines(t, res.stdout) // fails on any line that is not a record
	if len(recs) != 3 {
		t.Fatalf("stdout has %d records, want 3", len(recs))
	}
	for _, rec := range recs {
		for _, k := range []string{"id", "result", "run"} {
			if _, ok := rec[k]; !ok {
				t.Errorf("record %v has no %q", rec["id"], k)
			}
		}
	}
	if !strings.Contains(res.stderr, "3 to run") {
		t.Errorf("progress is not on stderr:\n%s", res.stderr)
	}
}

func TestBatch_ReadsStdin(t *testing.T) {
	llm := newFakeLLM(t)
	home := cliHome(t, llm.srv.URL)
	dir := batchProject(t, "")

	res := maniIn(t, home, threeTasks, nil, "batch", "--config", filepath.Join(dir, "agent.yaml"), "--in", "-")
	if res.code != 0 {
		t.Fatalf("exit %d\n%s", res.code, res.stderr)
	}
	if recs := jsonLines(t, res.stdout); len(recs) != 3 {
		t.Errorf("stdout has %d records, want 3", len(recs))
	}
}

func TestBatch_ResumesSkippingFinished(t *testing.T) {
	llm := newFakeLLM(t)
	home := cliHome(t, llm.srv.URL)
	dir := batchProject(t, threeTasks)
	args := batchArgs(dir, "--out", filepath.Join(dir, "out"))

	if res := runMani(t, home, args...); res.code != 0 {
		t.Fatalf("first run: exit %d\n%s", res.code, res.stderr)
	}
	calls := llm.calls.Load()

	res := runMani(t, home, args...)
	if res.code != 0 {
		t.Fatalf("second run: exit %d\n%s", res.code, res.stderr)
	}
	if got := llm.calls.Load() - calls; got != 0 {
		t.Errorf("second run made %d model calls, want 0", got)
	}
	if !strings.Contains(res.stderr, "0 to run, 3 already done") {
		t.Errorf("stderr does not say it resumed:\n%s", res.stderr)
	}
}

// Appending lines to an input that already ran is the everyday case: only the
// new lines run.
func TestBatch_NewLinesRunAlone(t *testing.T) {
	llm := newFakeLLM(t)
	home := cliHome(t, llm.srv.URL)
	dir := batchProject(t, threeTasks)
	args := batchArgs(dir, "--out", filepath.Join(dir, "out"))

	if res := runMani(t, home, args...); res.code != 0 {
		t.Fatalf("first run: exit %d\n%s", res.code, res.stderr)
	}
	calls := llm.calls.Load()
	more := threeTasks + `{"id": "d", "task": "four"}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "in.jsonl"), []byte(more), 0o644); err != nil {
		t.Fatal(err)
	}

	if res := runMani(t, home, args...); res.code != 0 {
		t.Fatalf("second run: exit %d\n%s", res.code, res.stderr)
	}
	if got := llm.seen()[calls:]; len(got) != 1 || got[0] != "four" {
		t.Errorf("second run sent %q, want only the new line", got)
	}
}

func TestBatch_FailedTaskWritesNoResultAndIsRetried(t *testing.T) {
	llm := newFakeLLM(t)
	llm.failing.Store(true)
	llm.fail = func(task string) bool { return task == "two" }
	home := cliHome(t, llm.srv.URL)
	dir := batchProject(t, threeTasks)
	out := filepath.Join(dir, "out")
	args := batchArgs(dir, "--out", out)

	res := runMani(t, home, args...)
	if res.code != exitRuntime {
		t.Fatalf("exit %d, want %d\n%s", res.code, exitRuntime, res.stderr)
	}
	if _, err := os.Stat(filepath.Join(out, "b.json")); err == nil {
		t.Error("a failed task left a result file: the next run would skip it")
	}
	for _, id := range []string{"a", "c"} {
		if _, err := os.Stat(filepath.Join(out, id+".json")); err != nil {
			t.Errorf("%s: one failure must not cost the others: %v", id, err)
		}
	}
	errs, err := os.ReadFile(filepath.Join(out, "errors.jsonl"))
	if err != nil || strings.Count(string(errs), "\n") != 1 || !strings.Contains(string(errs), `"id":"b"`) {
		t.Fatalf("errors.jsonl = %q (%v), want one line for b", errs, err)
	}

	llm.failing.Store(false)
	calls := llm.calls.Load()
	if res := runMani(t, home, args...); res.code != 0 {
		t.Fatalf("retry: exit %d\n%s", res.code, res.stderr)
	}
	if got := llm.seen()[calls:]; len(got) != 1 || got[0] != "two" {
		t.Errorf("retry sent %q, want only the failed task", got)
	}
	after, _ := os.ReadFile(filepath.Join(out, "errors.jsonl"))
	if string(after) != string(errs) {
		t.Errorf("errors.jsonl changed on a clean run: it is history, appended and never truncated")
	}
}

// A malformed input fails before a single model call, and says where.
func TestBatch_RejectsBadInput(t *testing.T) {
	cases := []struct {
		name, input, want string
	}{
		{"not JSON", "{\"id\": \"a\", \"task\": \"x\"}\nnot json\n", "line 2: not a JSON object"},
		{"id missing", `{"task": "x"}` + "\n", `line 1: "id" is required`},
		{"id not a string", `{"id": 7, "task": "x"}` + "\n", `line 1: "id" is required`},
		{"id climbs out", `{"id": "../x", "task": "x"}` + "\n", `line 1: id "../x" cannot be a file name`},
		{"id with a separator", `{"id": "a/b", "task": "x"}` + "\n", `cannot be a file name`},
		{"id repeated", `{"id": "a", "task": "x"}` + "\n" + `{"id": "a", "task": "y"}` + "\n", `line 2: id "a" already used on line 1`},
		{"empty input", "\n\n", "no records"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			llm := newFakeLLM(t)
			home := cliHome(t, llm.srv.URL)
			dir := batchProject(t, tc.input)

			res := runMani(t, home, batchArgs(dir)...)
			if res.code != exitUsage {
				t.Errorf("exit %d, want %d", res.code, exitUsage)
			}
			if !strings.Contains(res.stderr, tc.want) {
				t.Errorf("stderr = %q, want it to contain %q", res.stderr, tc.want)
			}
			if llm.calls.Load() != 0 {
				t.Error("a bad input reached the model")
			}
		})
	}
}

// A line without a task is found before the first model call too — even when
// it is the last line and the others are fine.
func TestBatch_MissingTaskFailsBeforeAnyRun(t *testing.T) {
	llm := newFakeLLM(t)
	home := cliHome(t, llm.srv.URL)
	dir := batchProject(t, `{"id": "a", "task": "one"}`+"\n"+`{"id": "b", "task": "  "}`+"\n")

	res := runMani(t, home, batchArgs(dir)...)
	if res.code == 0 {
		t.Fatal("exit 0 with an empty task")
	}
	if !strings.Contains(res.stderr, "record b") {
		t.Errorf("stderr does not name the record:\n%s", res.stderr)
	}
	if llm.calls.Load() != 0 {
		t.Error("the model was called before the input was checked")
	}
}

func TestBatch_ObjectTaskIsSentAsJSON(t *testing.T) {
	llm := newFakeLLM(t)
	home := cliHome(t, llm.srv.URL)
	dir := batchProject(t, `{"id": "a", "task": {"text": "one", "lang": "it"}}`+"\n")

	if res := runMani(t, home, batchArgs(dir)...); res.code != 0 {
		t.Fatalf("exit %d\n%s", res.code, res.stderr)
	}
	if got := llm.seen(); len(got) != 1 || got[0] != `{"lang":"it","text":"one"}` {
		t.Errorf("task sent = %q, want the object as JSON", got)
	}
}

func TestBatch_ConcurrencyIsBounded(t *testing.T) {
	llm := newFakeLLM(t)
	llm.delay = 50 * time.Millisecond
	home := cliHome(t, llm.srv.URL)
	var input strings.Builder
	for i := range 6 {
		input.WriteString(`{"id": "t` + string(rune('0'+i)) + `", "task": "x"}` + "\n")
	}
	dir := batchProject(t, input.String())

	if res := runMani(t, home, batchArgs(dir, "--jobs", "2")...); res.code != 0 {
		t.Fatalf("exit %d\n%s", res.code, res.stderr)
	}
	if got := llm.maxInFlight.Load(); got != 2 {
		t.Errorf("at most %d requests in flight, want exactly 2: never more, and not serial either", got)
	}
}

// Extras go in first and the fields mani owns win: an input line cannot fake
// a result or a provenance.
func TestBatch_ExtrasCannotForgeResult(t *testing.T) {
	llm := newFakeLLM(t)
	home := cliHome(t, llm.srv.URL)
	dir := batchProject(t, `{"id": "a", "task": "one", "result": "forged", "run": {"id": "fake"}}`+"\n")

	res := runMani(t, home, batchArgs(dir)...)
	if res.code != 0 {
		t.Fatalf("exit %d\n%s", res.code, res.stderr)
	}
	recs := jsonLines(t, res.stdout)
	if len(recs) != 1 || label(recs[0]) != "L:one" {
		t.Fatalf("record = %v, want the real result", recs)
	}
	if run, _ := recs[0]["run"].(map[string]any); run["id"] == "fake" {
		t.Error("the input line forged the run")
	}
}

func TestBatch_LimitStopsAfterNNewTasks(t *testing.T) {
	llm := newFakeLLM(t)
	home := cliHome(t, llm.srv.URL)
	dir := batchProject(t, threeTasks)

	res := runMani(t, home, batchArgs(dir, "--out", filepath.Join(dir, "out"), "--limit", "2")...)
	if res.code != 0 {
		t.Fatalf("exit %d\n%s", res.code, res.stderr)
	}
	if got := llm.calls.Load(); got != 2 {
		t.Errorf("model calls = %d, want 2", got)
	}
}

func TestBatch_FlagErrors(t *testing.T) {
	dir := batchProject(t, threeTasks)
	cases := map[string][]string{
		"no config":  {"batch", "--in", filepath.Join(dir, "in.jsonl")},
		"no input":   {"batch", "--config", filepath.Join(dir, "agent.yaml")},
		"zero jobs":  batchArgs(dir, "--jobs", "0"),
		"bad config": {"batch", "--config", filepath.Join(dir, "missing.yaml"), "--in", filepath.Join(dir, "in.jsonl")},
	}
	for name, args := range cases {
		if res := runMani(t, t.TempDir(), args...); res.code != exitUsage {
			t.Errorf("%s: exit %d, want %d\n%s", name, res.code, exitUsage, res.stderr)
		}
	}
}
