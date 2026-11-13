package workspace

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/coder/websocket"

	"github.com/Federicoand98/mani/app"
	"github.com/Federicoand98/mani/config"
	"github.com/Federicoand98/mani/manifest"
	"github.com/Federicoand98/mani/server/wire"
)

// agentOllama plays a model that uses tools: "read" in the message asks for
// read(notes.txt), "run" for bash(echo hi); after a tool result it answers
// "tool said: <result>".
func agentOllama(t *testing.T) string {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			_, _ = w.Write([]byte(`{"models":[{"name":"m"}]}`))
			return
		}
		if r.URL.Path == "/api/show" {
			_, _ = w.Write([]byte(`{"model_info":{"llama.context_length":8192}}`))
			return
		}
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.Unmarshal(body, &req)
		last := req.Messages[len(req.Messages)-1]
		msg := map[string]any{"role": "assistant", "content": ""}
		switch {
		case last.Role == "tool":
			msg["content"] = "tool said: " + strings.TrimSpace(last.Content)
		case strings.Contains(last.Content, "read"):
			msg["tool_calls"] = []any{map[string]any{"function": map[string]any{"name": "read", "arguments": map[string]any{"path": "notes.txt"}}}}
		case strings.Contains(last.Content, "run"):
			msg["tool_calls"] = []any{map[string]any{"function": map[string]any{"name": "bash", "arguments": map[string]any{"command": "echo hi"}}}}
		default:
			msg["content"] = "plain answer"
		}
		enc := json.NewEncoder(w)
		_ = enc.Encode(map[string]any{"message": msg, "done": false})
		_ = enc.Encode(map[string]any{"message": map[string]any{"role": "assistant", "content": ""}, "done": true, "done_reason": "stop"})
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

const agentYAML = `identity:
  name: helper
  provider: ollama
  model: m
  prompt: "help"
capabilities:
  tools: [read, bash]
context:
  inject: false
policy:
  tools:
    read: allow
    bash: ask
`

// newAgentEnv is a chat env whose working folder holds agent.yaml and notes.txt,
// with Ollama pointed at a model that calls tools.
func newAgentEnv(t *testing.T) *chatEnv {
	e := newChatEnv(t)
	url := agentOllama(t)
	if err := config.Update(func(c *config.Config) error {
		c.Providers["ollama"] = config.ProviderConfig{BaseURL: url, Model: "m", Enabled: true}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	write(t, e.dir, "agent.yaml", agentYAML)
	write(t, e.dir, "notes.txt", "the secret plan\n")
	return e
}

func (e *chatEnv) createWithAgent(agent string) Conversation {
	e.t.Helper()
	var c Conversation
	if code := e.api("POST", "/api/conversations", `{"agent":"`+agent+`"}`, &c); code != 201 {
		e.t.Fatalf("create with agent: %d", code)
	}
	return c
}

// collect reads up to one of the given types and returns every message on the way.
func (s *socket) collect(types ...string) []wire.ServerMsg {
	s.t.Helper()
	var out []wire.ServerMsg
	for {
		m := s.next()
		out = append(out, m)
		if slices.Contains(types, m.Type) {
			return out
		}
	}
}

func payload(m wire.ServerMsg) map[string]any {
	p, _ := m.Payload.(map[string]any)
	return p
}

func TestAgentChat_UsesToolsInsideTheFolder(t *testing.T) {
	e := newAgentEnv(t)
	c := e.createWithAgent("agent.yaml")
	if c.Agent != "agent.yaml" {
		t.Fatalf("conversation agent = %q", c.Agent)
	}

	ws := e.mustDial(c.ID)
	ws.send(wire.ClientMsg{Type: "input", Input: "please read it"})
	msgs := ws.collect("done", "error")
	var sawCall, sawResult bool
	var text strings.Builder
	for _, m := range msgs {
		switch m.Type {
		case "tool_call":
			sawCall = payload(m)["name"] == "read"
		case "tool_result":
			sawResult = strings.Contains(payload(m)["result"].(string), "the secret plan")
		case "token":
			text.WriteString(payload(m)["text"].(string))
		}
	}
	if !sawCall || !sawResult || msgs[len(msgs)-1].Type != "done" || !strings.Contains(text.String(), "tool said: the secret plan") {
		t.Fatalf("messages = %+v, text = %q", msgs, text.String())
	}

	var got Conversation
	e.api("GET", "/api/conversations/"+c.ID, "", &got)
	var types []string
	for _, m := range got.Messages {
		for _, p := range m.Parts {
			types = append(types, m.Role+":"+p.Type)
		}
	}
	if strings.Join(types, ",") != "user:text,assistant:tool_call,tool:tool_result,assistant:text" {
		t.Errorf("saved parts = %v", types)
	}
}

func TestAgentChat_AskOnceForTheConversationOrDeny(t *testing.T) {
	e := newAgentEnv(t)
	c := e.createWithAgent("agent.yaml")
	ws := e.mustDial(c.ID)

	answer := func(decision string) []wire.ServerMsg {
		ws.send(wire.ClientMsg{Type: "input", Input: "run it"})
		msgs := ws.collect("permission_request", "done")
		last := msgs[len(msgs)-1]
		if last.Type != "permission_request" {
			return msgs // answered without asking
		}
		p := payload(last)
		if p["tool_name"] != "bash" || p["risk_level"] == "" {
			t.Fatalf("request = %+v", p)
		}
		ws.send(wire.ClientMsg{Type: "permission_response", RequestID: p["request_id"].(string), Decision: decision})
		return append(msgs, ws.collect("done", "error")...)
	}
	resultOf := func(msgs []wire.ServerMsg) (string, bool, bool) {
		asked := false
		for _, m := range msgs {
			if m.Type == "permission_request" {
				asked = true
			}
			if m.Type == "tool_result" {
				return payload(m)["result"].(string), payload(m)["is_error"].(bool), asked
			}
		}
		return "", false, asked
	}

	if res, isErr, asked := resultOf(answer("allow_once")); !asked || isErr || !strings.Contains(res, "hi") {
		t.Errorf("allow_once: %q error=%v asked=%v", res, isErr, asked)
	}
	if _, _, asked := resultOf(answer("allow_conversation")); !asked {
		t.Error("the second run did not ask")
	}
	if res, isErr, asked := resultOf(answer("unused")); asked || isErr || !strings.Contains(res, "hi") {
		t.Errorf("after allow_conversation: %q error=%v asked=%v", res, isErr, asked)
	}

	// Another conversation with the same agent still asks: the permission was this one's.
	other := e.createWithAgent("agent.yaml")
	ws = e.mustDial(other.ID)
	// A denied call never runs, so no tool_result: the model hears it was blocked.
	msgs := answer("deny")
	res, _, asked := resultOf(msgs)
	var text strings.Builder
	for _, m := range msgs {
		if m.Type == "token" {
			text.WriteString(payload(m)["text"].(string))
		}
	}
	if !asked || res != "" || !strings.Contains(text.String(), "blocked") {
		t.Errorf("deny in another conversation: result %q asked=%v reply %q", res, asked, text.String())
	}
	disk, _ := os.ReadFile(filepath.Join(e.dir, "agent.yaml"))
	if string(disk) != agentYAML {
		t.Error("an approval changed the manifest")
	}
}

func TestAgentChat_AReopenedTabCanAnswer(t *testing.T) {
	e := newAgentEnv(t)
	c := e.createWithAgent("agent.yaml")
	first := e.mustDial(c.ID)
	first.send(wire.ClientMsg{Type: "input", Input: "run it"})
	req := first.collect("permission_request")
	id := payload(req[len(req)-1])["request_id"].(string)

	second := e.mustDial(c.ID)
	if m := second.next(); m.Type != "busy" {
		t.Fatalf("reopened: %+v", m)
	}
	if m := second.next(); m.Type != "permission_request" || payload(m)["request_id"] != id {
		t.Fatalf("the reopened tab did not get the pending request: %+v", m)
	}
	second.send(wire.ClientMsg{Type: "permission_response", RequestID: id, Decision: "allow_once"})
	resolved := first.collect("permission_resolved")
	if p := payload(resolved[len(resolved)-1]); p["request_id"] != id || p["decision"] != "allow_once" {
		t.Errorf("the first tab was not told: %+v", p)
	}
	if msgs := second.collect("done", "error"); msgs[len(msgs)-1].Type != "done" {
		t.Errorf("after answering: %+v", msgs)
	}
}

func TestAgentChat_TheRuntimeIsSharedUntilFileOrSettingsChange(t *testing.T) {
	e := newAgentEnv(t)
	var builds atomic.Int32
	orig := buildAgent
	t.Cleanup(func() { buildAgent = orig })
	buildAgent = func(ctx context.Context, spec manifest.Spec) (*app.Runtime, error) {
		builds.Add(1)
		if !filepath.IsAbs(spec.Capabilities.Workspace) {
			t.Errorf("workspace not anchored to the folder: %q", spec.Capabilities.Workspace)
		}
		return orig(ctx, spec)
	}

	say := func(c Conversation) {
		ws := e.mustDial(c.ID)
		ws.send(wire.ClientMsg{Type: "input", Input: "hello"})
		if msgs := ws.collect("done", "error"); msgs[len(msgs)-1].Type != "done" {
			t.Fatalf("reply: %+v", msgs)
		}
		ws.conn.Close(websocket.StatusNormalClosure, "")
	}
	a, b := e.createWithAgent("agent.yaml"), e.createWithAgent("agent.yaml")
	say(a)
	say(b)
	say(a)
	if n := builds.Load(); n != 1 {
		t.Errorf("builds for one unchanged agent = %d, want 1", n)
	}
	var apiError APIError
	if code := e.api("PUT", "/api/settings", `{"maxOutputTokens":0}`, &apiError); code != 400 {
		t.Fatalf("invalid settings: %d", code)
	}
	if code := e.api("PUT", "/api/providers/ollama", `{"modelLimits":{"m":{"context_window":-1}}}`, &apiError); code != 400 {
		t.Fatalf("invalid model limits: %d", code)
	}
	say(a)
	if n := builds.Load(); n != 1 {
		t.Errorf("rejected settings invalidated the runtime: %d builds", n)
	}
	var settings Settings
	if code := e.api("PUT", "/api/settings", `{"maxOutputTokens":128}`, &settings); code != 200 {
		t.Fatalf("save requested output: %d", code)
	}
	say(a)
	var got Conversation
	e.api("GET", "/api/conversations/"+a.ID, "", &got)
	if builds.Load() != 2 || got.Context == nil || got.Context.OutputLimit != 128 {
		t.Fatalf("settings did not reach rebuilt runtime: builds=%d, context=%+v", builds.Load(), got.Context)
	}
	var provider Provider
	if code := e.api("PUT", "/api/providers/ollama", `{"modelLimits":{"m":{"context_window":1024}}}`, &provider); code != 200 {
		t.Fatalf("save model limits: %d", code)
	}
	say(a)
	e.api("GET", "/api/conversations/"+a.ID, "", &got)
	if builds.Load() != 3 || got.Context == nil || got.Context.InputBudget != 896 {
		t.Fatalf("model limits did not reach rebuilt runtime: builds=%d, context=%+v", builds.Load(), got.Context)
	}
	write(t, e.dir, "agent.yaml", agentYAML+"limits:\n  max_tool_calls: 5\n  max_output_tokens: 64\n")
	say(a)
	e.api("GET", "/api/conversations/"+a.ID, "", &got)
	if n := builds.Load(); n != 4 {
		t.Errorf("builds after the file changed = %d, want 4", n)
	}
	if got.Context == nil || got.Context.OutputLimit != 64 || got.Context.InputBudget != 960 {
		t.Fatalf("explicit manifest output did not override requested output: %+v", got.Context)
	}
}

func TestAgentRuntimeInvalidationKeepsActiveReferencesUntilRelease(t *testing.T) {
	e := newAgentEnv(t)
	s := New(Options{Dir: e.dir, Token: "tok"})
	t.Cleanup(s.Close)
	old, release, err := s.acquireAgent(context.Background(), "agent.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	_, releaseOther, err := s.acquireAgent(context.Background(), "agent.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer releaseOther()
	entry := s.agents.m["agent.yaml"]
	s.agents.invalidate()
	if !entry.stale || entry.refs != 2 || len(s.agents.m) != 0 {
		t.Fatalf("active runtime was not safely retired: %+v", entry)
	}
	next, releaseNext, err := s.acquireAgent(context.Background(), "agent.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer releaseNext()
	if old == next {
		t.Fatal("the next turn acquired the retired runtime")
	}
	release()
	release()
	if entry.refs != 1 {
		t.Fatalf("release was not idempotent: %d", entry.refs)
	}
	releaseOther()
	if entry.refs != 0 {
		t.Fatalf("active references remain: %d", entry.refs)
	}
}

func TestAgentChat_AMissingOrBrokenAgentIsExplained(t *testing.T) {
	e := newAgentEnv(t)
	c := e.createWithAgent("agent.yaml")
	ws := e.mustDial(c.ID)

	write(t, e.dir, "agent.yaml", agentYAML+"limits:\n  max_duration: soon\n")
	ws.send(wire.ClientMsg{Type: "input", Input: "hello"})
	if msgs := ws.collect("error", "done"); !strings.Contains(errText(msgs[len(msgs)-1]), "has 1 problem") {
		t.Errorf("broken agent: %+v", msgs[len(msgs)-1])
	}

	if err := os.Remove(filepath.Join(e.dir, "agent.yaml")); err != nil {
		t.Fatal(err)
	}
	ws.send(wire.ClientMsg{Type: "input", Input: "hello"})
	if msgs := ws.collect("error", "done"); !strings.Contains(errText(msgs[len(msgs)-1]), "does not exist any more") {
		t.Errorf("deleted agent: %+v", msgs[len(msgs)-1])
	}
}

func TestAgentChat_SwitchingBetweenAgentAndModel(t *testing.T) {
	e := newAgentEnv(t)
	c := e.createWithAgent("agent.yaml")

	var res APIError
	if code := e.api("PATCH", "/api/conversations/"+c.ID, `{"agent":""}`, &res); code != 400 || !strings.Contains(res.Error, "pick a model") {
		t.Errorf("remove the agent without a model: %d %q", code, res.Error)
	}
	var got Conversation
	if code := e.api("PATCH", "/api/conversations/"+c.ID, `{"agent":"","provider":"ollama","model":"m"}`, &got); code != 200 || got.Agent != "" || got.Model != "m" {
		t.Errorf("back to the model: %d %+v", code, got)
	}
	if code := e.api("PATCH", "/api/conversations/"+c.ID, `{"agent":"../outside.yaml"}`, &res); code != 400 {
		t.Errorf("an agent outside the folder: %d", code)
	}
	if code := e.api("PATCH", "/api/conversations/"+c.ID, `{"agent":"agent.yaml"}`, &got); code != 200 || got.Agent != "agent.yaml" {
		t.Errorf("back to the agent: %d %+v", code, got)
	}
}

// M3-11 and M3-7: a reply lands in the agent's journal as source=web, and a
// relative journal path counts from the working folder, not from the process.
func TestAgentChat_TheJournalRecordsWebRuns(t *testing.T) {
	e := newAgentEnv(t)
	write(t, e.dir, "agent.yaml", agentYAML+"observability:\n  journal:\n    enabled: true\n    path: runs\n")
	c := e.createWithAgent("agent.yaml")
	ws := e.mustDial(c.ID)
	ws.send(wire.ClientMsg{Type: "input", Input: "hello"})
	if msgs := ws.collect("done", "error"); msgs[len(msgs)-1].Type != "done" {
		t.Fatalf("reply: %+v", msgs)
	}

	var found bool
	_ = filepath.WalkDir(filepath.Join(e.dir, "runs"), func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			data, _ := os.ReadFile(p)
			found = found || strings.Contains(string(data), `"web"`)
		}
		return nil
	})
	if !found {
		t.Errorf("no run with source web under %s/runs", e.dir)
	}
}
