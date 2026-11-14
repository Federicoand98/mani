package workspace

import (
	"fmt"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Federicoand98/mani/journal"
	"github.com/Federicoand98/mani/journal/sqlite"
)

func runsAgentYAML(name, journalBlock string) string {
	return fmt.Sprintf("identity:\n  name: %s\n  provider: ollama\n  model: qwen3.5:9b\n  prompt: hi\n%s", name, journalBlock)
}

func jsonlAt(path string) string {
	return "observability:\n  journal:\n    enabled: true\n    path: " + path + "\n"
}

var t0 = time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)

type fakeRun struct {
	id, agent, source, status string
	at                        time.Duration // after t0
	blocked                   bool
}

func record(t *testing.T, j journal.Journal, runs ...fakeRun) {
	t.Helper()
	for _, r := range runs {
		if err := j.Start(journal.RunRecord{ID: r.id, SessionID: "s-" + r.id, Source: r.source, StartedAt: t0.Add(r.at), Task: "task of " + r.id,
			Agent:  journal.Agent{Name: r.agent, Provider: "ollama", Model: "qwen3.5:9b"},
			Limits: journal.Limits{MaxToolCalls: 6}}); err != nil {
			t.Fatal(err)
		}
		if r.blocked {
			_ = j.Append(journal.RunEvent{RunID: r.id, At: t0.Add(r.at + time.Second), Kind: journal.EvPermission,
				Data: map[string]any{"tool": "bash", "decision": journal.DecisionExpired}})
		}
		if r.status != "running" {
			_ = j.Finish(r.id, journal.RunOutcome{Status: r.status, Answer: "answer of " + r.id})
		}
	}
}

func ids(runs []Run) string {
	out := make([]string, len(runs))
	for i, r := range runs {
		out[i] = r.ID
	}
	return strings.Join(out, " ")
}

// runsFolder: docs writes to a JSONL journal, monitor to SQLite, mail keeps none.
func runsFolder(t *testing.T) *fixture {
	f := newFixture(t)
	write(t, f.dir, "agents/docs.yaml", runsAgentYAML("docs", jsonlAt("runs/docs")))
	write(t, f.dir, "agents/monitor.yaml", runsAgentYAML("monitor", "observability:\n  journal:\n    enabled: true\n    backend: sqlite\n    path: runs/monitor.db\n"))
	write(t, f.dir, "mail.yaml", runsAgentYAML("mail", ""))

	docs, err := journal.NewJSONL(filepath.Join(f.dir, "runs/docs"))
	if err != nil {
		t.Fatal(err)
	}
	record(t, docs,
		fakeRun{id: "d1", agent: "docs", source: "web", status: "ok", at: 1 * time.Minute},
		fakeRun{id: "d2", agent: "docs", source: "cli", status: "error", at: 3 * time.Minute},
		fakeRun{id: "d3", agent: "docs", source: "web", status: "ok", at: 5 * time.Minute, blocked: true},
		fakeRun{id: "d4", agent: "", source: "cli", status: "ok", at: 0}, // recorded before runs named their agent
	)
	mon, err := sqlite.New(filepath.Join(f.dir, "runs/monitor.db"), 0)
	if err != nil {
		t.Fatal(err)
	}
	record(t, mon,
		fakeRun{id: "m1", agent: "monitor", source: "daily", status: "ok", at: 2 * time.Minute},
		fakeRun{id: "m2", agent: "monitor", source: "cron", status: "running", at: 4 * time.Minute},
		fakeRun{id: "m3", agent: "monitor", source: "daily", status: "error", at: 6 * time.Minute, blocked: true},
	)
	_ = mon.Close()
	return f
}

func TestRuns_MergesTheJournalsOfTheFolderNewestFirst(t *testing.T) {
	f := runsFolder(t)
	var got RunList
	if code := f.do("GET", "/api/runs?period=all", "", &got); code != 200 {
		t.Fatalf("status %d", code)
	}
	if ids(got.Runs) != "m3 d3 m2 d2 m1 d1 d4" {
		t.Errorf("runs = %s", ids(got.Runs))
	}
	if got.Next != "" {
		t.Errorf("next = %q, want none: everything fit", got.Next)
	}
	byID := map[string]Run{}
	for _, r := range got.Runs {
		byID[r.ID] = r
	}
	if r := byID["m2"]; r.Status != "running" || r.EndedAt != "" || r.AgentPath != "agents/monitor.yaml" || r.Task != "task of m2" {
		t.Errorf("m2 = %+v", r)
	}
	if r := byID["d3"]; r.Agent != "docs" || r.AgentPath != "agents/docs.yaml" || r.Model != "ollama / qwen3.5:9b" || r.Summary.Blocked != 1 {
		t.Errorf("d3 = %+v", r)
	}
	if r := byID["d4"]; r.Agent != "" || r.AgentPath != "" {
		t.Errorf("d4 = %+v, want no agent", r)
	}
	if len(got.Unrecorded) != 1 || got.Unrecorded[0] != (AgentRef{Name: "mail", Path: "mail.yaml"}) {
		t.Errorf("unrecorded = %+v", got.Unrecorded)
	}
}

// Walking the cursor visits every run once, in order, across journals that
// interleave in time.
func TestRuns_TheCursorNeitherSkipsNorRepeats(t *testing.T) {
	f := runsFolder(t)
	var seen []string
	before := ""
	for range 10 {
		var page RunList
		path := "/api/runs?period=all&limit=2"
		if before != "" {
			path += "&before=" + url.QueryEscape(before)
		}
		if code := f.do("GET", path, "", &page); code != 200 {
			t.Fatalf("status %d", code)
		}
		if len(page.Runs) > 2 {
			t.Fatalf("page of %d runs, limit 2", len(page.Runs))
		}
		for _, r := range page.Runs {
			seen = append(seen, r.ID)
		}
		if page.Next == "" {
			break
		}
		before = page.Next
	}
	if strings.Join(seen, " ") != "m3 d3 m2 d2 m1 d1 d4" {
		t.Errorf("pages = %v", seen)
	}
}

func TestRuns_Filters(t *testing.T) {
	f := runsFolder(t)
	for _, c := range []struct{ query, want string }{
		{"status=error", "m3 d2"},
		{"status=running", "m2"},
		{"agent=docs", "d3 d2 d1"},
		{"agent=unknown", "d4"},
		{"source=daily", "m3 m1"},
		{"blocked=1", "m3 d3"},
		// Filters the journals cannot apply still fill a page: a short one
		// is followed by the next until the limit or the end.
		{"source=web&limit=1", "d3"},
		{"blocked=1&agent=docs", "d3"},
		{"session=s-d1", "d1"},
	} {
		var got RunList
		if code := f.do("GET", "/api/runs?period=all&"+c.query, "", &got); code != 200 {
			t.Fatalf("%s: status %d", c.query, code)
		}
		if ids(got.Runs) != c.want {
			t.Errorf("%s: runs = %q, want %q", c.query, ids(got.Runs), c.want)
		}
	}
	var bad map[string]string
	if code := f.do("GET", "/api/runs?period=1y", "", &bad); code != 400 {
		t.Errorf("period=1y: status %d, want 400", code)
	}
}

// Two agents writing to one journal stay apart: each run names its agent.
func TestRuns_ASharedJournalKeepsItsAgentsApart(t *testing.T) {
	f := newFixture(t)
	write(t, f.dir, "docs.yaml", runsAgentYAML("docs", jsonlAt("runs")))
	write(t, f.dir, "monitor.yaml", runsAgentYAML("monitor", jsonlAt("runs")))
	j, err := journal.NewJSONL(filepath.Join(f.dir, "runs"))
	if err != nil {
		t.Fatal(err)
	}
	record(t, j,
		fakeRun{id: "a", agent: "docs", source: "web", status: "ok", at: time.Minute},
		fakeRun{id: "b", agent: "monitor", source: "daily", status: "ok", at: 2 * time.Minute})

	var got RunList
	f.do("GET", "/api/runs?period=all", "", &got)
	if ids(got.Runs) != "b a" || got.Runs[0].AgentPath != "monitor.yaml" || got.Runs[1].AgentPath != "docs.yaml" {
		t.Errorf("runs = %+v", got.Runs)
	}
	f.do("GET", "/api/runs?period=all&agent=monitor", "", &got)
	if ids(got.Runs) != "b" {
		t.Errorf("agent=monitor: %s", ids(got.Runs))
	}
}

func TestRuns_DetailAndNewEvents(t *testing.T) {
	f := runsFolder(t)
	var d RunDetail
	if code := f.do("GET", "/api/runs/m3", "", &d); code != 200 {
		t.Fatalf("status %d", code)
	}
	if d.Agent != "monitor" || d.AgentPath != "agents/monitor.yaml" || d.Status != "error" || d.Answer != "answer of m3" {
		t.Errorf("detail = %+v", d.Run)
	}
	if d.Limits == nil || d.Limits.MaxToolCalls != 6 {
		t.Errorf("limits = %+v", d.Limits)
	}
	kinds := []string{}
	for _, ev := range d.Events {
		kinds = append(kinds, ev.Kind)
	}
	if !slices.Equal(kinds, []string{"run_start", "permission", "run_end"}) {
		t.Errorf("events = %v", kinds)
	}

	var since RunDetail
	f.do("GET", fmt.Sprintf("/api/runs/m3?after=%d", d.Events[0].Seq), "", &since)
	if len(since.Events) != 2 || since.Events[0].Kind != "permission" {
		t.Errorf("after the first event: %+v", since.Events)
	}

	var e map[string]string
	if code := f.do("GET", "/api/runs/nope", "", &e); code != 404 {
		t.Errorf("unknown run: status %d", code)
	}
}
