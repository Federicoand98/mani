package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Federicoand98/mani/app"
	"github.com/Federicoand98/mani/tool"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// chatRequest is the subset of Ollama's /api/chat body the tests inspect.
type chatRequest struct {
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
}

// transcript flattens a request into one string, so a test can ask whether
// something was shown to the model at all.
func (r chatRequest) transcript() string {
	var b strings.Builder
	for _, m := range r.Messages {
		fmt.Fprintf(&b, "[%s] %s\n", m.Role, m.Content)
	}
	return b.String()
}

// turn is one scripted model reply: plain text, or a single tool call.
type turn struct {
	text string
	tool string
	args map[string]any
}

// fakeModel serves Ollama's /api/chat from a script, one turn per request, and
// records every request so a test can check what the model was shown. Past the
// end of the script it repeats the last turn.
type fakeModel struct {
	script []turn

	mu       sync.Mutex
	requests []chatRequest
}

func (f *fakeModel) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/chat" {
		http.NotFound(w, r)
		return
	}
	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	f.mu.Lock()
	next := f.script[min(len(f.requests), len(f.script)-1)]
	f.requests = append(f.requests, req)
	f.mu.Unlock()

	msg := map[string]any{"role": "assistant", "content": next.text}
	if next.tool != "" {
		msg["tool_calls"] = []map[string]any{
			{"function": map[string]any{"name": next.tool, "arguments": next.args}},
		}
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"message": msg, "done": true, "done_reason": "stop",
		"prompt_eval_count": 11, "eval_count": 7,
	})
}

func (f *fakeModel) seen() []chatRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]chatRequest(nil), f.requests...)
}

// newServer builds an MCP server whose agent talks to model.
//
// The provider base URL comes from the global config, so XDG_CONFIG_HOME points
// at a temp dir holding a config.json aimed at the fake: hermetic, and no
// production code grows a test seam. Same approach as server/rest_test.go.
func newServer(t *testing.T, model *fakeModel, edit func(*app.RuntimeSpec)) *Server {
	t.Helper()
	llm := httptest.NewServer(model)
	t.Cleanup(llm.Close)

	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	if err := os.MkdirAll(filepath.Join(home, "mani"), 0o755); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	cfg := fmt.Sprintf(`{"provider":"ollama","providers":{"ollama":{"base_url":%q,"model":"test-model"}}}`, llm.URL)
	if err := os.WriteFile(filepath.Join(home, "mani", "config.json"), []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	spec := app.DefaultSpec()
	spec.Identity.Name = "summariser"
	spec.Identity.Description = "Summarises a document in two sentences."
	spec.Identity.Provider = "ollama"
	spec.Identity.Model = "test-model"
	// context injection reads AGENTS.md from disk: off, keeps the test hermetic
	spec.Context.Inject = false
	spec.Observability.Tracing = false
	spec.Capabilities.Workspace = t.TempDir()
	if edit != nil {
		edit(&spec)
	}

	s, err := New(context.Background(), spec, "test")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// connect serves s over an in-memory transport and returns a client session on
// the other end: the real protocol, minus the process boundary.
func connect(t *testing.T, s *Server) *mcp.ClientSession {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	serverSide, clientSide := mcp.NewInMemoryTransports()
	if _, err := s.srv.Connect(ctx, serverSide, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0"}, nil)
	cs, err := client.Connect(ctx, clientSide, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

// call invokes the agent tool. The timeout turns a hang into a failure: several
// of the properties under test are "this returns at all".
func call(t *testing.T, cs *mcp.ClientSession, args any) *mcp.CallToolResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "summariser", Arguments: args})
	if err != nil {
		t.Fatalf("CallTool returned a protocol error: %v", err)
	}
	return res
}

func textOf(res *mcp.CallToolResult) string {
	var parts []string
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			parts = append(parts, tc.Text)
		}
	}
	return strings.Join(parts, "")
}

// asMap normalises a schema or structured value, whatever Go type the SDK
// decoded it into, to its JSON shape.
func asMap(t *testing.T, v any) map[string]any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal %s: %v", b, err)
	}
	return m
}

// assertNoNulls fails on any JSON null inside a schema. No JSON Schema keyword
// accepts null ("items" wants a schema, "required" and "enum" want arrays), and
// clients that compile tool schemas against the meta-schema reject the tool.
func assertNoNulls(t *testing.T, path string, v any) {
	t.Helper()
	switch v := v.(type) {
	case nil:
		t.Errorf("%s is null", path)
	case map[string]any:
		for k, child := range v {
			assertNoNulls(t, path+"."+k, child)
		}
	case []any:
		for i, child := range v {
			assertNoNulls(t, fmt.Sprintf("%s[%d]", path, i), child)
		}
	}
}

func sentimentSchema() tool.InputSchema {
	return tool.InputSchema{
		Type: "object",
		Properties: map[string]tool.PropertySchema{
			"label": {Type: "string", Enum: []string{"positive", "negative"}},
			"score": {Type: "number"},
		},
		Required: []string{"label", "score"},
	}
}

// ---------------------------------------------------------------- construction

func TestNew_RequiresIdentityName(t *testing.T) {
	_, err := New(context.Background(), app.RuntimeSpec{}, "test")
	if err == nil || !strings.Contains(err.Error(), "identity.name") {
		t.Fatalf("err = %v, want an error naming identity.name", err)
	}
}

// The SDK only logs an invalid tool name, and its default logger discards, so
// without our own check a bad name would reach the client unnoticed.
func TestNew_RejectsInvalidToolName(t *testing.T) {
	for _, name := range []string{
		"my agent",              // spaces
		"agent/v2",              // slash
		"agent.v2",              // dot: allowed by MCP, refused by the model APIs clients forward names to
		"àgent",                 // non-ASCII
		strings.Repeat("a", 65), // over the 64 the model APIs accept
	} {
		t.Run(name, func(t *testing.T) {
			spec := app.RuntimeSpec{}
			spec.Identity.Name = name
			_, err := New(context.Background(), spec, "test")
			if err == nil {
				t.Fatalf("name %q was accepted", name)
			}
			// the message must say what a valid name looks like, not only that this one is wrong
			if !strings.Contains(err.Error(), "a-z") {
				t.Errorf("err = %v, want it to describe the valid form", err)
			}
		})
	}
}

func TestNew_AcceptsValidToolNames(t *testing.T) {
	for _, name := range []string{"summariser", "code-review_2", strings.Repeat("a", 64)} {
		t.Run(name, func(t *testing.T) {
			newServer(t, &fakeModel{script: []turn{{text: "unused"}}}, func(s *app.RuntimeSpec) {
				s.Identity.Name = name
			})
		})
	}
}

// -------------------------------------------------------------------- listing

func TestListTools_ExposesTheWholeAgentAsOneTool(t *testing.T) {
	cs := connect(t, newServer(t, &fakeModel{script: []turn{{text: "unused"}}}, nil))

	init := cs.InitializeResult()
	if init.ServerInfo.Name != "mani" {
		t.Errorf("server name = %q, want mani", init.ServerInfo.Name)
	}
	if init.Instructions != "Summarises a document in two sentences." {
		t.Errorf("instructions = %q, want identity.description", init.Instructions)
	}

	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(res.Tools) != 1 {
		t.Fatalf("got %d tools, want exactly one: the agent", len(res.Tools))
	}
	got := res.Tools[0]
	if got.Name != "summariser" {
		t.Errorf("tool name = %q, want identity.name", got.Name)
	}
	if got.Description != "Summarises a document in two sentences." {
		t.Errorf("tool description = %q, want identity.description", got.Description)
	}

	in := asMap(t, got.InputSchema)
	if in["type"] != "object" {
		t.Errorf("input schema type = %v, want object", in["type"])
	}
	if req, _ := in["required"].([]any); len(req) != 1 || req[0] != "task" {
		t.Errorf("input schema required = %v, want [task]", in["required"])
	}
	if got.OutputSchema != nil {
		t.Errorf("output schema = %v, want none for a manifest without output.schema", got.OutputSchema)
	}
}

func TestListTools_OutputSchemaComesFromTheManifest(t *testing.T) {
	cs := connect(t, newServer(t, &fakeModel{script: []turn{{text: "unused"}}}, func(s *app.RuntimeSpec) {
		s.Output.Schema = sentimentSchema()
	}))

	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if res.Tools[0].OutputSchema == nil {
		t.Fatal("output schema missing: a typed agent must stay typed over MCP")
	}
	out := asMap(t, res.Tools[0].OutputSchema)
	if out["type"] != "object" {
		t.Errorf("output schema type = %v, want object", out["type"])
	}
	props, _ := out["properties"].(map[string]any)
	if _, ok := props["label"]; !ok {
		t.Errorf("output schema properties = %v, want label", props)
	}
	assertNoNulls(t, "outputSchema", out)
}

// -------------------------------------------------------------------- calling

func TestCallTool_ReturnsTheAgentsText(t *testing.T) {
	model := &fakeModel{script: []turn{{text: "A short summary."}}}
	cs := connect(t, newServer(t, model, nil))

	res := call(t, cs, map[string]any{"task": "summarise README.md"})

	if res.IsError {
		t.Fatalf("IsError = true: %s", textOf(res))
	}
	if got := textOf(res); got != "A short summary." {
		t.Errorf("text = %q, want the agent's reply", got)
	}
	if res.StructuredContent != nil {
		t.Errorf("structured content = %v, want none without output.schema", res.StructuredContent)
	}
	if reqs := model.seen(); len(reqs) != 1 || !strings.Contains(reqs[0].transcript(), "summarise README.md") {
		t.Errorf("the task never reached the model: %+v", reqs)
	}
}

func TestCallTool_StructuredOutput(t *testing.T) {
	model := &fakeModel{script: []turn{
		{tool: "respond", args: map[string]any{"label": "positive", "score": 0.9}},
	}}
	cs := connect(t, newServer(t, model, func(s *app.RuntimeSpec) {
		s.Output.Schema = sentimentSchema()
	}))

	res := call(t, cs, map[string]any{"task": "classify: what a lovely day"})

	if res.IsError {
		t.Fatalf("IsError = true: %s", textOf(res))
	}
	got := asMap(t, res.StructuredContent)
	if got["label"] != "positive" || got["score"] != 0.9 {
		t.Errorf("structured content = %v", got)
	}

	// The spec asks a tool returning structured content to also return it as
	// text, for clients that only read content. Many do.
	var fromText map[string]any
	if err := json.Unmarshal([]byte(textOf(res)), &fromText); err != nil {
		t.Fatalf("text content %q is not the JSON of the result: %v", textOf(res), err)
	}
	if fromText["label"] != "positive" {
		t.Errorf("text content = %v, want the same object as the structured content", fromText)
	}
}

// A failed run is the tool's failure, not the protocol's: the calling model has
// to see it and be able to react.
func TestCallTool_RunFailureIsAToolError(t *testing.T) {
	model := &fakeModel{script: []turn{{text: "too long"}}}
	cs := connect(t, newServer(t, model, func(s *app.RuntimeSpec) {
		s.Limits.MaxTokens = 1 // the first reply already costs 18
	}))

	res := call(t, cs, map[string]any{"task": "anything"})

	if !res.IsError {
		t.Fatalf("IsError = false, want the budget failure reported as a tool error")
	}
	if !strings.Contains(textOf(res), "max_tokens") {
		t.Errorf("text = %q, want the reason", textOf(res))
	}
}

// Bad arguments are reported as tool errors too: a protocol error is invisible
// to the calling model, which then cannot correct its call.
func TestCallTool_InvalidArgumentsAreToolErrors(t *testing.T) {
	model := &fakeModel{script: []turn{{text: "unused"}}}
	cs := connect(t, newServer(t, model, nil))

	for name, args := range map[string]any{
		"no arguments":   nil,
		"missing task":   map[string]any{},
		"blank task":     map[string]any{"task": "   "},
		"task not text":  map[string]any{"task": 42},
		"unrelated only": map[string]any{"prompt": "hello"},
	} {
		t.Run(name, func(t *testing.T) {
			res := call(t, cs, args)
			if !res.IsError {
				t.Fatalf("IsError = false for %v", args)
			}
			if !strings.Contains(textOf(res), "task") {
				t.Errorf("text = %q, want it to name the task argument", textOf(res))
			}
		})
	}
	if n := len(model.seen()); n != 0 {
		t.Errorf("model called %d times, want none for invalid arguments", n)
	}
}

// An MCP client cannot answer an `ask`. It must resolve to deny and the run must
// finish; the failure mode is a call that never returns.
func TestCallTool_AskIsDeniedWithoutBlocking(t *testing.T) {
	model := &fakeModel{script: []turn{
		{tool: "write", args: map[string]any{"path": "out.txt", "content": "x"}},
		{text: "I was not allowed to write the file."},
	}}
	var workspace string
	cs := connect(t, newServer(t, model, func(s *app.RuntimeSpec) {
		workspace = s.Capabilities.Workspace
		s.Capabilities.Tools = []app.ToolRef{{Name: "write"}}
		s.Policy.Tools = map[string]app.RiskPolicy{"write": app.RiskPolicyAsk}
	}))

	res := call(t, cs, map[string]any{"task": "write out.txt"})

	if res.IsError {
		t.Fatalf("IsError = true: a refused tool is not a failed run: %s", textOf(res))
	}
	if _, err := os.Stat(filepath.Join(workspace, "out.txt")); !os.IsNotExist(err) {
		t.Errorf("out.txt exists (stat err = %v): the ask was approved", err)
	}
	reqs := model.seen()
	if len(reqs) != 2 {
		t.Fatalf("model called %d times, want 2 (tool call, then the answer)", len(reqs))
	}
	last := reqs[1].Messages[len(reqs[1].Messages)-1]
	if last.Role != "tool" || !strings.Contains(strings.ToLower(last.Content), "denied") {
		t.Errorf("last message before the answer = %+v, want the denied tool result", last)
	}
}

// A tool is a function, not a conversation: the second call must not see the
// first one.
func TestCallTool_EachCallIsAFreshRun(t *testing.T) {
	model := &fakeModel{script: []turn{{text: "first answer"}, {text: "second answer"}}}
	cs := connect(t, newServer(t, model, nil))

	call(t, cs, map[string]any{"task": "alpha task"})
	call(t, cs, map[string]any{"task": "beta task"})

	reqs := model.seen()
	if len(reqs) != 2 {
		t.Fatalf("model called %d times, want 2", len(reqs))
	}
	second := reqs[1].transcript()
	for _, leak := range []string{"alpha task", "first answer"} {
		if strings.Contains(second, leak) {
			t.Errorf("second call saw %q from the first:\n%s", leak, second)
		}
	}
}

// The audit trail is the argument for running a governed agent inside an
// editor, so an MCP run must be journaled and recognisable as one.
func TestCallTool_RunIsJournaledAsMCP(t *testing.T) {
	dir := t.TempDir()
	model := &fakeModel{script: []turn{{text: "done"}}}
	cs := connect(t, newServer(t, model, func(s *app.RuntimeSpec) {
		s.Observability.Journal = app.JournalSpec{Enabled: true, Path: dir}
	}))

	call(t, cs, map[string]any{"task": "anything"})

	j, err := app.NewJSONLJournal(dir)
	if err != nil {
		t.Fatalf("open journal: %v", err)
	}
	runs, err := j.List(app.ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("journal has %d runs, want 1", len(runs))
	}
	if runs[0].Source != "mcp" || runs[0].Status != "ok" {
		t.Errorf("run = source %q status %q, want mcp / ok", runs[0].Source, runs[0].Status)
	}
}
