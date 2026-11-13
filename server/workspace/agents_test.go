package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const goodAgent = `# a comment that must survive
identity:
  name: reviewer
  description: "Reviews things"
  provider: ollama
  model: qwen3.5:9b
capabilities:
  tools: [read]
run:
  triggers:
    - { type: daily, at: "07:00" }
`

func write(t *testing.T, dir, rel, body string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAgents_FindsManifestsAndSkipsTheRest(t *testing.T) {
	f := newFixture(t)
	write(t, f.dir, "reviewer.yaml", goodAgent)
	write(t, f.dir, "agents/broken.yaml", "identity:\n  name: [oops\n")
	write(t, f.dir, "agents/regex.yml", "identity:\n  name: regex\npolicy:\n  rules:\n    - { tool: bash, pattern: \"(x\" }\n")
	write(t, f.dir, "pipeline.flow.yaml", "flow: p\nsteps: []\n")
	write(t, f.dir, "notes.yaml", "title: not an agent\n")
	write(t, f.dir, ".mani/hidden.yaml", goodAgent)
	write(t, f.dir, "node_modules/x/agent.yaml", goodAgent)

	var list AgentList
	if code := f.do("GET", "/api/agents", "", &list); code != 200 {
		t.Fatalf("list: %d", code)
	}
	got := map[string]AgentSummary{}
	for _, a := range list.Agents {
		got[a.Path] = a
	}
	if len(got) != 3 {
		t.Fatalf("agents = %+v, want reviewer.yaml, agents/broken.yaml, agents/regex.yml", list.Agents)
	}
	if a := got["reviewer.yaml"]; a.Status != "ok" || a.Name != "reviewer" || a.Tools != 1 || len(a.Triggers) != 1 || a.Triggers[0] != "daily 07:00" {
		t.Errorf("reviewer = %+v", a)
	}
	if a := got["agents/broken.yaml"]; a.Status != "invalid" || a.Name != "broken" {
		t.Errorf("broken = %+v", a)
	}
	if a := got["agents/regex.yml"]; a.Status != "problems" || a.Problems != 1 {
		t.Errorf("regex = %+v", a)
	}
}

func TestAgents_SaveChecksTheVersion(t *testing.T) {
	f := newFixture(t)
	write(t, f.dir, "reviewer.yaml", goodAgent)

	var file AgentFile
	if code := f.do("GET", "/api/agents/file/reviewer.yaml", "", &file); code != 200 || file.Text != goodAgent || len(file.Problems) != 0 {
		t.Fatalf("get: %d %+v", code, file)
	}

	edited := strings.Replace(goodAgent, "Reviews things", "Reviews more things", 1)
	var saved AgentFile
	body := `{"text":` + quote(edited) + `,"version":"` + file.Version + `"}`
	if code := f.do("PUT", "/api/agents/file/reviewer.yaml", body, &saved); code != 200 || saved.Version == file.Version {
		t.Fatalf("save: %d %+v", code, saved)
	}
	disk, _ := os.ReadFile(filepath.Join(f.dir, "reviewer.yaml"))
	if string(disk) != edited {
		t.Errorf("on disk:\n%s", disk)
	}

	// Someone changes the file outside the console.
	write(t, f.dir, "reviewer.yaml", goodAgent+"# edited elsewhere\n")
	var conflict AgentFile
	if code := f.do("PUT", "/api/agents/file/reviewer.yaml", body, &conflict); code != 409 || !strings.Contains(conflict.Text, "edited elsewhere") {
		t.Fatalf("stale save: %d %+v", code, conflict)
	}
	forced := `{"text":` + quote(edited) + `,"force":true}`
	if code := f.do("PUT", "/api/agents/file/reviewer.yaml", forced, &saved); code != 200 {
		t.Errorf("forced save: %d", code)
	}
}

func TestAgents_CreateNeverReplaces(t *testing.T) {
	f := newFixture(t)
	write(t, f.dir, "reviewer.yaml", goodAgent)
	var file AgentFile
	if code := f.do("POST", "/api/agents/file/copies/reviewer-2.yaml", `{"text":`+quote(goodAgent)+`}`, &file); code != 201 {
		t.Fatalf("create: %d", code)
	}
	if _, err := os.Stat(filepath.Join(f.dir, "copies", "reviewer-2.yaml")); err != nil {
		t.Error("file not created")
	}
	var e APIError
	if code := f.do("POST", "/api/agents/file/reviewer.yaml", `{"text":"x"}`, &e); code != 409 {
		t.Errorf("create over an existing file: %d", code)
	}
}

func TestAgents_DeleteOnlyAgents(t *testing.T) {
	f := newFixture(t)
	write(t, f.dir, "reviewer.yaml", goodAgent)
	write(t, f.dir, "notes.yaml", "title: x\n")
	if code := f.do("DELETE", "/api/agents/file/reviewer.yaml", "", nil); code != 204 {
		t.Errorf("delete: %d", code)
	}
	var e APIError
	if code := f.do("DELETE", "/api/agents/file/notes.yaml", "", &e); code != 400 {
		t.Errorf("delete a non-agent: %d", code)
	}
	if code := f.do("GET", "/api/agents/file/reviewer.yaml", "", &e); code != 404 {
		t.Errorf("after delete: %d", code)
	}
}

func TestAgents_CheckReportsLines(t *testing.T) {
	f := newFixture(t)
	var res CheckResult
	text := "identity:\n  name: x\nlimits:\n  max_duration: soon\n"
	f.do("POST", "/api/agents/check", `{"text":`+quote(text)+`,"path":"a.yaml"}`, &res)
	if len(res.Problems) != 1 || res.Problems[0].Line != 4 || res.Problems[0].Path != "limits.max_duration" {
		t.Errorf("problems = %+v", res.Problems)
	}
}

func TestResolve_StaysInsideTheWorkingDirectory(t *testing.T) {
	f := newFixture(t)
	s := New(Options{Dir: f.dir})
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(f.dir, "escape")); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"../x.yaml", "a/../../x.yaml", "/etc/x.yaml", "x.txt", ".mani/x.yaml", "escape/x.yaml", ""} {
		if _, err := s.resolve(bad, true); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	for _, good := range []string{"a.yaml", "agents/b.yml", "./c.yaml"} {
		if _, err := s.resolve(good, true); err != nil {
			t.Errorf("%q refused: %v", good, err)
		}
	}
}

func TestFolders(t *testing.T) {
	f := newFixture(t)
	for _, d := range []string{"docs/drafts", "notes", ".git", ".mani", "node_modules"} {
		if err := os.MkdirAll(filepath.Join(f.dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	var list FolderList
	f.do("GET", "/api/folders", "", &list)
	if strings.Join(list.Folders, ",") != "docs,notes" {
		t.Errorf("root folders = %v", list.Folders)
	}
	f.do("GET", "/api/folders?path=docs", "", &list)
	if strings.Join(list.Folders, ",") != "drafts" || list.Path != "docs" {
		t.Errorf("docs = %+v", list)
	}
	var e APIError
	if code := f.do("GET", "/api/folders?path=..", "", &e); code != 400 {
		t.Errorf("outside: %d", code)
	}
}

func quote(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)
	return `"` + r.Replace(s) + `"`
}
