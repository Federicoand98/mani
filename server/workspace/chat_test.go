package workspace

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/Federicoand98/mani/config"
	"github.com/Federicoand98/mani/server/wire"
)

// fakeOllama answers /api/chat in streaming, one word per chunk. With hold set,
// it stops after the first word until release is closed.
type fakeOllama struct {
	*httptest.Server
	mu      sync.Mutex
	reply   string
	status  int
	hold    bool
	release chan struct{}
	last    []byte // the body of the last chat request
}

func newFakeOllama(t *testing.T) *fakeOllama {
	f := &fakeOllama{reply: "Hello there, friend", release: make(chan struct{})}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			_, _ = w.Write([]byte(`{"models":[{"name":"qwen3.5:9b"}]}`))
			return
		}
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.last = body
		reply, status, hold := f.reply, f.status, f.hold
		f.mu.Unlock()
		if status != 0 {
			http.Error(w, "model not found", status)
			return
		}
		words := strings.SplitAfter(reply, " ")
		enc := json.NewEncoder(w)
		for i, word := range words {
			_ = enc.Encode(map[string]any{"message": map[string]string{"role": "assistant", "content": word}, "done": false})
			w.(http.Flusher).Flush()
			if i == 0 && hold {
				select {
				case <-f.release:
				case <-r.Context().Done():
					return
				}
			}
		}
		_ = enc.Encode(map[string]any{"message": map[string]string{"role": "assistant", "content": ""}, "done": true,
			"done_reason": "stop", "prompt_eval_count": 12, "eval_count": 4})
	}))
	t.Cleanup(f.Close)
	return f
}

// set changes how the next reply behaves.
func (f *fakeOllama) set(fn func(*fakeOllama)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

type chatEnv struct {
	t      *testing.T
	base   string
	client *http.Client
	cookie string
	ollama *fakeOllama
	dir    string
}

func newChatEnv(t *testing.T) *chatEnv {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	os.Unsetenv("MANI_PROVIDER")
	os.Unsetenv("MANI_MODEL")
	ollama := newFakeOllama(t)
	if err := config.Update(func(c *config.Config) error {
		c.Providers["ollama"] = config.ProviderConfig{BaseURL: ollama.URL, Model: "qwen3.5:9b", Enabled: true}
		c.Thinking = false
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	srv := New(Options{Dir: dir, Version: "test", Assets: assets, Token: "tok"})
	hs := httptest.NewServer(srv.Handler())
	t.Cleanup(func() {
		hs.Close()
		srv.Close()
	})

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	if _, err := client.Get(hs.URL + "/?t=tok"); err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(hs.URL)
	cookies := jar.Cookies(u)
	if len(cookies) == 0 {
		t.Fatal("no session cookie")
	}
	return &chatEnv{t: t, base: hs.URL, client: client, cookie: cookies[0].Name + "=" + cookies[0].Value, ollama: ollama, dir: dir}
}

func (e *chatEnv) api(method, path, body string, out any) int {
	e.t.Helper()
	req, _ := http.NewRequest(method, e.base+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res, err := e.client.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	if out != nil {
		data, _ := io.ReadAll(res.Body)
		if err := json.Unmarshal(data, out); err != nil {
			e.t.Fatalf("%s %s: %d %q", method, path, res.StatusCode, data)
		}
	}
	return res.StatusCode
}

func (e *chatEnv) create() Conversation {
	e.t.Helper()
	var c Conversation
	if code := e.api("POST", "/api/conversations", `{"provider":"ollama","model":"qwen3.5:9b"}`, &c); code != 201 {
		e.t.Fatalf("create: %d", code)
	}
	return c
}

type socket struct {
	t    *testing.T
	conn *websocket.Conn
}

func (e *chatEnv) dial(id string, header http.Header) (*socket, error) {
	if header == nil {
		header = http.Header{}
	}
	header.Set("Cookie", e.cookie)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(e.base, "http")+"/api/conversations/"+id+"/ws", &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		return nil, err
	}
	conn.SetReadLimit(64 << 20)
	s := &socket{t: e.t, conn: conn}
	e.t.Cleanup(func() { conn.CloseNow() })
	return s, nil
}

func (e *chatEnv) mustDial(id string) *socket {
	e.t.Helper()
	s, err := e.dial(id, nil)
	if err != nil {
		e.t.Fatal(err)
	}
	return s
}

func (s *socket) send(m wire.ClientMsg) {
	s.t.Helper()
	if err := wsjson.Write(context.Background(), s.conn, m); err != nil {
		s.t.Fatal(err)
	}
}

func (s *socket) next() wire.ServerMsg {
	s.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var m wire.ServerMsg
	if err := wsjson.Read(ctx, s.conn, &m); err != nil {
		s.t.Fatalf("read: %v", err)
	}
	return m
}

// until reads messages up to one of the given types, returning the text of the
// tokens on the way and the last message.
func (s *socket) until(types ...string) (string, wire.ServerMsg) {
	s.t.Helper()
	var text strings.Builder
	for {
		m := s.next()
		if m.Type == "token" {
			text.WriteString(m.Payload.(map[string]any)["text"].(string))
		}
		for _, t := range types {
			if m.Type == t {
				return text.String(), m
			}
		}
	}
}

func errText(m wire.ServerMsg) string {
	if p, ok := m.Payload.(map[string]any); ok {
		s, _ := p["message"].(string)
		return s
	}
	return ""
}

func TestChat_ARoundTrip(t *testing.T) {
	e := newChatEnv(t)
	c := e.create()

	ws := e.mustDial(c.ID)
	ws.send(wire.ClientMsg{Type: "input", Input: "hello"})
	text, last := ws.until("done", "error")
	if last.Type != "done" || text != "Hello there, friend" {
		t.Fatalf("reply = %q, last = %+v", text, last)
	}

	var got Conversation
	e.api("GET", "/api/conversations/"+c.ID, "", &got)
	if len(got.Messages) != 2 || got.Messages[1].Parts[0].Text != "Hello there, friend" || got.Title != "hello" || got.Running {
		t.Errorf("saved conversation: %+v", got)
	}
	var list ConversationList
	e.api("GET", "/api/conversations", "", &list)
	if len(list.Conversations) != 1 || list.Conversations[0].Provider != "ollama" {
		t.Errorf("list: %+v", list)
	}

	ignore, err := os.ReadFile(filepath.Join(e.dir, ".mani", ".gitignore"))
	if err != nil || !strings.Contains(string(ignore), "*") {
		t.Errorf(".mani/.gitignore: %q %v", ignore, err)
	}
}

func TestChat_RefusesAProviderNotAdded(t *testing.T) {
	e := newChatEnv(t)
	var res APIError
	if code := e.api("POST", "/api/conversations", `{"provider":"anthropic","model":"x"}`, &res); code != 400 || !strings.Contains(res.Error, "not added") {
		t.Errorf("anthropic: %d %q", code, res.Error)
	}
	if code := e.api("POST", "/api/conversations", `{"provider":"ollama"}`, &res); code != 400 {
		t.Errorf("no model: %d", code)
	}
}

// M2-4: closing the tab does not stop the reply; it finishes and is saved.
func TestChat_TheReplyOutlivesTheTab(t *testing.T) {
	e := newChatEnv(t)
	e.ollama.set(func(f *fakeOllama) { f.hold = true })
	c := e.create()

	ws := e.mustDial(c.ID)
	ws.send(wire.ClientMsg{Type: "input", Input: "hello"})
	if m := ws.next(); m.Type != "token" {
		t.Fatalf("first message = %+v", m)
	}
	ws.conn.Close(websocket.StatusGoingAway, "tab closed")

	var got Conversation
	e.api("GET", "/api/conversations/"+c.ID, "", &got)
	if !got.Running {
		t.Error("conversation not marked running while the model writes")
	}
	close(e.ollama.release)

	deadline := time.Now().Add(5 * time.Second)
	for got.Running && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
		e.api("GET", "/api/conversations/"+c.ID, "", &got)
	}
	if got.Running || len(got.Messages) != 2 || got.Messages[1].Parts[0].Text != "Hello there, friend" {
		t.Errorf("after the tab closed: %+v", got)
	}
}

func TestChat_ReopeningDuringAReplySaysBusy(t *testing.T) {
	e := newChatEnv(t)
	e.ollama.set(func(f *fakeOllama) { f.hold = true })
	c := e.create()

	first := e.mustDial(c.ID)
	first.send(wire.ClientMsg{Type: "input", Input: "hello"})
	first.next() // the first word

	second := e.mustDial(c.ID)
	if m := second.next(); m.Type != "busy" {
		t.Fatalf("reopened socket got %+v, want busy", m)
	}
	second.send(wire.ClientMsg{Type: "input", Input: "again"})
	if _, m := second.until("error"); !strings.Contains(errText(m), "still answering") {
		t.Errorf("second input during a reply: %+v", m)
	}
	var res APIError
	if code := e.api("DELETE", "/api/conversations/"+c.ID, "", &res); code != 409 {
		t.Errorf("delete while answering: %d", code)
	}

	close(e.ollama.release)
	if _, m := second.until("done", "error"); m.Type != "done" {
		t.Errorf("the reopened tab never heard the end: %+v", m)
	}
}

func TestChat_StopKeepsTheQuestion(t *testing.T) {
	e := newChatEnv(t)
	e.ollama.set(func(f *fakeOllama) { f.hold = true })
	c := e.create()

	ws := e.mustDial(c.ID)
	ws.send(wire.ClientMsg{Type: "input", Input: "a long question"})
	ws.next()
	ws.send(wire.ClientMsg{Type: "cancel"})
	if _, m := ws.until("cancelled", "done", "error"); m.Type != "cancelled" {
		t.Fatalf("after cancel: %+v", m)
	}

	var got Conversation
	e.api("GET", "/api/conversations/"+c.ID, "", &got)
	if len(got.Messages) == 0 || got.Messages[0].Parts[0].Text != "a long question" {
		t.Errorf("the question was lost: %+v", got.Messages)
	}
}

func TestChat_AProviderErrorKeepsTheQuestion(t *testing.T) {
	e := newChatEnv(t)
	e.ollama.set(func(f *fakeOllama) { f.status = http.StatusNotFound })
	c := e.create()

	ws := e.mustDial(c.ID)
	ws.send(wire.ClientMsg{Type: "input", Input: "hello"})
	if _, m := ws.until("error", "done"); m.Type != "error" || !strings.Contains(errText(m), "model not found") {
		t.Fatalf("provider error: %+v", m)
	}
	var got Conversation
	e.api("GET", "/api/conversations/"+c.ID, "", &got)
	if len(got.Messages) != 1 || got.Messages[0].Role != "user" {
		t.Errorf("after a provider error: %+v", got.Messages)
	}
}

func TestChat_ImagesReachTheModelAndAreChecked(t *testing.T) {
	e := newChatEnv(t)
	c := e.create()
	ws := e.mustDial(c.ID)

	png := base64.StdEncoding.EncodeToString([]byte("\x89PNG fake"))
	ws.send(wire.ClientMsg{Type: "input", Input: "what is this?", Images: []wire.Image{{MediaType: "image/png", Data: png}}})
	if _, m := ws.until("done", "error"); m.Type != "done" {
		t.Fatalf("with an image: %+v", m)
	}
	e.ollama.mu.Lock()
	sent := e.ollama.last
	e.ollama.mu.Unlock()
	if !bytes.Contains(sent, []byte(png)) {
		t.Errorf("the image never reached the model: %s", sent)
	}
	var got Conversation
	e.api("GET", "/api/conversations/"+c.ID, "", &got)
	if parts := got.Messages[0].Parts; len(parts) != 2 || parts[1].Type != "image" || parts[1].Data != png {
		t.Errorf("image not saved: %+v", parts)
	}

	big := base64.StdEncoding.EncodeToString(make([]byte, maxImageBytes+1))
	one := wire.Image{MediaType: "image/png", Data: png}
	cases := []struct {
		name   string
		images []wire.Image
		want   string
	}{
		{"too big", []wire.Image{{MediaType: "image/png", Data: big}}, "the limit is 5 MB"},
		{"wrong type", []wire.Image{{MediaType: "image/bmp", Data: png}}, "not supported"},
		{"too many", []wire.Image{one, one, one, one, one}, "at most 4 images"},
	}
	for _, tc := range cases {
		ws.send(wire.ClientMsg{Type: "input", Input: "x", Images: tc.images})
		if _, m := ws.until("error", "done"); !strings.Contains(errText(m), tc.want) {
			t.Errorf("%s: %+v", tc.name, m)
		}
	}
}

func TestChat_KeepIsValidated(t *testing.T) {
	e := newChatEnv(t)
	ws := e.mustDial(e.create().ID)
	ws.send(wire.ClientMsg{Type: "input", Input: "x", Keep: 1})
	if _, m := ws.until("error", "done"); !strings.Contains(errText(m), "between 2 and 500") {
		t.Errorf("keep 1: %+v", m)
	}
}

// Another site may open a WebSocket to localhost: the browser sends its Origin,
// and the handshake must fail even with the cookie attached.
func TestChat_ForeignOriginCannotOpenTheSocket(t *testing.T) {
	e := newChatEnv(t)
	c := e.create()
	if _, err := e.dial(c.ID, http.Header{"Origin": {"http://evil.example"}}); err == nil {
		t.Error("a foreign Origin opened the socket")
	}
	if _, err := e.dial(c.ID, http.Header{"Origin": {e.base}}); err != nil {
		t.Errorf("the console's own Origin was refused: %v", err)
	}
}

func TestChat_RenameSwitchModelAndDelete(t *testing.T) {
	e := newChatEnv(t)
	c := e.create()

	var got Conversation
	if code := e.api("PATCH", "/api/conversations/"+c.ID, `{"title":"Invoices"}`, &got); code != 200 || got.Title != "Invoices" {
		t.Errorf("rename: %d %+v", code, got)
	}
	var res APIError
	if code := e.api("PATCH", "/api/conversations/"+c.ID, `{"provider":"openai"}`, &res); code != 400 {
		t.Errorf("switch to a provider not added: %d", code)
	}
	if code := e.api("PATCH", "/api/conversations/"+c.ID, `{"model":"llama3.2:3b"}`, &got); code != 200 || got.Model != "llama3.2:3b" || got.Provider != "ollama" {
		t.Errorf("switch model: %d %+v", code, got)
	}
	if code := e.api("DELETE", "/api/conversations/"+c.ID, "", nil); code != 204 {
		t.Errorf("delete: %d", code)
	}
	if code := e.api("GET", "/api/conversations/"+c.ID, "", &res); code != 404 {
		t.Errorf("after delete: %d", code)
	}
	if code := e.api("GET", "/api/conversations/nope", "", &res); code != 404 {
		t.Errorf("unknown id: %d", code)
	}
}
