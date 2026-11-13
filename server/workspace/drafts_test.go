package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Federicoand98/mani/server/wire"
)

func TestTemplates_Listed(t *testing.T) {
	f := newFixture(t)
	var list TemplateList
	f.do("GET", "/api/templates", "", &list)
	var names []string
	for _, tp := range list.Templates {
		names = append(names, tp.Name)
		if tp.Title == "" || !strings.Contains(tp.Text, "identity:") {
			t.Errorf("template %s = %+v", tp.Name, tp)
		}
	}
	if strings.Join(names, ",") != "agent,docs,monitor,classifier,blank" {
		t.Errorf("templates = %v", names)
	}
}

// M4-4: a trial talks like a saved agent, tools and approvals included, and
// leaves nothing on disk: no conversation file, no journal.
func TestDraft_ATrialLeavesNothingBehind(t *testing.T) {
	e := newAgentEnv(t)
	text := agentYAML + "observability:\n  journal:\n    enabled: true\n    path: runs\n"

	var d Draft
	if code := e.api("POST", "/api/drafts", `{"text":`+quote(text)+`}`, &d); code != 201 || d.ID == "" {
		t.Fatalf("create: %d %+v", code, d)
	}
	ws, err := e.dialPath("/api/drafts/"+d.ID+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	ws.send(wire.ClientMsg{Type: "input", Input: "run it"})
	msgs := ws.collect("permission_request")
	ws.send(wire.ClientMsg{Type: "permission_response", RequestID: payload(msgs[len(msgs)-1])["request_id"].(string), Decision: "allow_once"})
	msgs = ws.collect("done", "error")
	var ran bool
	for _, m := range msgs {
		ran = ran || (m.Type == "tool_result" && strings.Contains(payload(m)["result"].(string), "hi"))
	}
	if !ran || msgs[len(msgs)-1].Type != "done" {
		t.Fatalf("trial: %+v", msgs)
	}

	var list ConversationList
	e.api("GET", "/api/conversations", "", &list)
	if len(list.Conversations) != 0 {
		t.Errorf("a trial is listed among conversations: %+v", list.Conversations)
	}
	for _, p := range []string{".mani/sessions", "runs"} {
		if entries, _ := os.ReadDir(filepath.Join(e.dir, p)); len(entries) > 0 {
			t.Errorf("a trial left files in %s: %v", p, entries)
		}
	}

	if code := e.api("DELETE", "/api/drafts/"+d.ID, "", nil); code != 204 {
		t.Errorf("end the trial: %d", code)
	}
	if _, err := e.dialPath("/api/drafts/"+d.ID+"/ws", nil); err == nil {
		t.Error("an ended trial still opens")
	}
}

func TestDraft_AnInvalidTextIsRefusedWithItsProblems(t *testing.T) {
	e := newAgentEnv(t)
	var res CheckResult
	text := "identity:\n  name: x\nlimits:\n  max_duration: soon\n"
	if code := e.api("POST", "/api/drafts", `{"text":`+quote(text)+`}`, &res); code != 400 || len(res.Problems) != 1 || res.Problems[0].Line != 4 {
		t.Errorf("invalid draft: %d %+v", code, res)
	}
	var apiErr APIError
	if code := e.api("POST", "/api/drafts", `{"text":"identity:\n  name: x\n","folder":"../out"}`, &apiErr); code != 400 {
		t.Errorf("a folder outside the working directory: %d", code)
	}
}
