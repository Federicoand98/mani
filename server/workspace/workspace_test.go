package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/Federicoand98/mani/config"
	"github.com/Federicoand98/mani/llm/copilot"
)

const host = "127.0.0.1:7878"

type fixture struct {
	t      *testing.T
	dir    string
	h      http.Handler
	cookie *http.Cookie

	mu   sync.Mutex
	next string // the token handed to OnLogin
}

var assets = fstest.MapFS{
	"index.html":          {Data: []byte("<!doctype html><title>console</title>")},
	"_app/immutable/a.js": {Data: []byte("console.log(1)")},
	"favicon.svg":         {Data: []byte("<svg/>")},
}

// newFixture isolates config.json and auth.json in temp dirs and signs in.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	for _, k := range []string{"MANI_PROVIDER", "MANI_MODEL"} {
		os.Unsetenv(k)
	}
	f := &fixture{t: t, dir: t.TempDir()}
	srv := New(Options{Dir: f.dir, Version: "test", Assets: assets, Token: "tok", OnLogin: func(n string) {
		f.mu.Lock()
		f.next = n
		f.mu.Unlock()
	}})
	t.Cleanup(srv.Close)
	f.h = srv.Handler()
	f.cookie = f.login("tok")
	if f.cookie == nil {
		t.Fatal("login with the right token gave no cookie")
	}
	return f
}

func (f *fixture) login(token string) *http.Cookie {
	res := f.raw(httptest.NewRequest("GET", "http://"+host+"/providers?t="+token, nil))
	for _, c := range res.Result().Cookies() {
		if c.Name == cookieName {
			return c
		}
	}
	return nil
}

func (f *fixture) raw(r *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	f.h.ServeHTTP(w, r)
	return w
}

// do sends a signed-in API request and decodes the answer into out.
func (f *fixture) do(method, path, body string, out any) int {
	f.t.Helper()
	r := httptest.NewRequest(method, "http://"+host+path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.AddCookie(f.cookie)
	w := f.raw(r)
	if out != nil {
		if err := json.Unmarshal(w.Body.Bytes(), out); err != nil {
			f.t.Fatalf("%s %s: decode %q: %v", method, path, w.Body.String(), err)
		}
	}
	return w.Code
}

func TestLogin_SpendsTheTokenAndRedirectsWithoutIt(t *testing.T) {
	f := newFixture(t)

	w := f.raw(httptest.NewRequest("GET", "http://"+host+"/providers?t="+f.next+"&x=1", nil))
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/providers?x=1" {
		t.Errorf("redirect = %d %q, want 303 to /providers?x=1", w.Code, w.Header().Get("Location"))
	}
	c := w.Result().Cookies()[0]
	if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode {
		t.Errorf("cookie %+v must be HttpOnly and SameSite=Strict", c)
	}

	if f.login("tok") != nil {
		t.Error("the first token worked twice")
	}
	w = f.raw(httptest.NewRequest("GET", "http://"+host+"/?t=wrong", nil))
	if w.Code != http.StatusUnauthorized || !strings.Contains(w.Body.String(), "already been used") {
		t.Errorf("wrong token: %d %q", w.Code, w.Body.String())
	}
}

func TestAPI_NeedsTheSessionCookie(t *testing.T) {
	f := newFixture(t)

	w := f.raw(httptest.NewRequest("GET", "http://"+host+"/api/health", nil))
	if w.Code != http.StatusUnauthorized || !strings.Contains(w.Body.String(), "not signed in") {
		t.Errorf("without cookie: %d %s", w.Code, w.Body.String())
	}

	var h Health
	if code := f.do("GET", "/api/health", "", &h); code != 200 || h.Dir != f.dir || h.Version != "test" {
		t.Errorf("with cookie: %d %+v", code, h)
	}
}

func TestHostCheck_RefusesAForeignHost(t *testing.T) {
	f := newFixture(t)
	for _, h := range []string{"evil.example:7878", "192.168.1.5:7878", "localhost.evil.example"} {
		r := httptest.NewRequest("GET", "http://"+h+"/api/health", nil)
		r.AddCookie(f.cookie)
		if w := f.raw(r); w.Code != http.StatusForbidden {
			t.Errorf("Host %s: %d, want 403", h, w.Code)
		}
	}
	for _, h := range []string{"localhost:7878", "[::1]:7878", "127.0.0.1"} {
		if !IsLoopbackHost(h) {
			t.Errorf("%s should be loopback", h)
		}
	}
}

func TestSameOrigin_RefusesACrossOriginWrite(t *testing.T) {
	f := newFixture(t)
	r := httptest.NewRequest("DELETE", "http://"+host+"/api/providers/openai", nil)
	r.Header.Set("Origin", "http://evil.example")
	r.AddCookie(f.cookie)
	if w := f.raw(r); w.Code != http.StatusForbidden {
		t.Errorf("cross-origin DELETE: %d, want 403", w.Code)
	}
}

func TestProviders_StartEmptyAndRoundTrip(t *testing.T) {
	f := newFixture(t)

	var list ProviderList
	f.do("GET", "/api/providers", "", &list)
	if len(list.Providers) != 5 {
		t.Fatalf("catalog has %d providers, want 5", len(list.Providers))
	}
	for _, p := range list.Providers {
		if p.Enabled {
			t.Errorf("%s enabled on a fresh config", p.Name)
		}
	}

	var e APIError
	if code := f.do("PUT", "/api/providers/openai", `{}`, &e); code != 400 || !strings.Contains(e.Error, "API key") {
		t.Errorf("openai without key: %d %q", code, e.Error)
	}

	var p Provider
	if code := f.do("PUT", "/api/providers/openai", `{"key":"sk-secret-1234","model":"gpt-4.1-mini"}`, &p); code != 200 {
		t.Fatalf("PUT openai: %d", code)
	}
	if !p.Enabled || p.KeyHint != "…1234" || p.Model != "gpt-4.1-mini" {
		t.Errorf("after PUT: %+v", p)
	}

	cfgFile, _ := os.ReadFile(config.ConfigPath())
	if strings.Contains(string(cfgFile), "sk-secret") {
		t.Error("the key landed in config.json")
	}
	auth, _ := config.LoadAuth()
	if c, _ := auth.Get("openai"); c.Key != "sk-secret-1234" {
		t.Errorf("auth.json has %+v", c)
	}

	var removed Provider
	if code := f.do("DELETE", "/api/providers/openai", "", &removed); code != 200 || removed.Enabled || removed.KeyHint != "" {
		t.Errorf("after DELETE: %d %+v", code, removed)
	}
	auth, _ = config.LoadAuth()
	if _, ok := auth.Get("openai"); ok {
		t.Error("Remove kept the key in auth.json")
	}

	// An unknown name is a new custom endpoint, which needs an API type.
	if code := f.do("PUT", "/api/providers/nope", `{}`, &e); code != 400 || !strings.Contains(e.Error, "API type") {
		t.Errorf("unknown provider without a type: %d %q", code, e.Error)
	}
	if code := f.do("PUT", "/api/providers/ollama", `{"baseUrl":"x","typo":1}`, &e); code != 400 {
		t.Errorf("unknown field accepted: %d", code)
	}
}

// The real path: Ollama is added by URL, and its models come from /api/tags.
func TestProviders_OllamaAgainstAFakeServer(t *testing.T) {
	f := newFixture(t)
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"models":[{"name":"qwen3.5:9b"},{"name":"llama3.2:3b"}]}`))
	}))
	defer fake.Close()

	var res TestResult
	f.do("POST", "/api/providers/ollama/test", `{"baseUrl":"`+fake.URL+`/"}`, &res)
	if !res.OK || len(res.Models) != 2 {
		t.Fatalf("test against the fake: %+v", res)
	}
	f.do("POST", "/api/providers/ollama/test", `{"baseUrl":"http://127.0.0.1:1"}`, &res)
	if res.OK || res.Error == "" {
		t.Errorf("unreachable URL reported ok: %+v", res)
	}

	var p Provider
	if code := f.do("PUT", "/api/providers/ollama", `{"baseUrl":"`+fake.URL+`/"}`, &p); code != 200 || !p.Enabled || p.BaseURL != fake.URL {
		t.Fatalf("PUT ollama: %d %+v", code, p)
	}
	var ml ModelList
	if code := f.do("GET", "/api/providers/ollama/models", "", &ml); code != 200 || len(ml.Models) != 2 {
		t.Errorf("models: %d %+v", code, ml)
	}
	var e APIError
	if code := f.do("GET", "/api/providers/anthropic/models", "", &e); code != 409 {
		t.Errorf("models of a provider not added: %d", code)
	}
}

func TestProviders_TestReportsTheProviderRefusal(t *testing.T) {
	f := newFixture(t)
	orig := listModels
	t.Cleanup(func() { listModels = orig })
	listModels = func(_ context.Context, _ config.Config, auth config.Auth, name string) ([]string, error) {
		if c, _ := auth.Get(name); c.Key != "sk-good" {
			return nil, errors.New("anthropic: HTTP 401: invalid x-api-key")
		}
		return []string{"claude-sonnet-5-5"}, nil
	}

	var res TestResult
	f.do("POST", "/api/providers/anthropic/test", `{"key":"bad"}`, &res)
	if res.OK || !strings.Contains(res.Error, "invalid x-api-key") {
		t.Errorf("refusal: %+v", res)
	}
	f.do("POST", "/api/providers/anthropic/test", `{"key":"sk-good"}`, &res)
	if !res.OK || res.Models[0] != "claude-sonnet-5-5" {
		t.Errorf("success: %+v", res)
	}
	// A test saves nothing.
	if auth, _ := config.LoadAuth(); len(auth.Providers) != 0 {
		t.Errorf("test wrote auth.json: %+v", auth.Providers)
	}
}

func TestCopilot_DeviceFlowEnablesItWhenAuthorized(t *testing.T) {
	f := newFixture(t)
	origStart, origPoll := startDevice, pollDevice
	t.Cleanup(func() { startDevice, pollDevice = origStart, origPoll })
	release := make(chan struct{})
	startDevice = func(context.Context) (copilot.DeviceCode, error) {
		return copilot.DeviceCode{UserCode: "WDJB-MJHT", VerificationURI: "https://github.com/login/device", ExpiresIn: 60}, nil
	}
	pollDevice = func(ctx context.Context, _ copilot.DeviceCode) (string, error) {
		<-release
		return "gho_token", nil
	}

	var e APIError
	if code := f.do("PUT", "/api/providers/copilot", `{}`, &e); code != 400 {
		t.Errorf("copilot without sign-in: %d", code)
	}

	var ds DeviceStart
	if code := f.do("POST", "/api/providers/copilot/device", "", &ds); code != 200 || ds.UserCode != "WDJB-MJHT" {
		t.Fatalf("start: %d %+v", code, ds)
	}
	var st DeviceStatus
	if f.do("GET", "/api/providers/copilot/device", "", &st); st.Status != "pending" {
		t.Errorf("status before authorizing: %+v", st)
	}
	close(release)
	deadline := time.Now().Add(2 * time.Second)
	for st.Status != "done" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
		f.do("GET", "/api/providers/copilot/device", "", &st)
	}
	if st.Status != "done" {
		t.Fatalf("status after authorizing: %+v", st)
	}
	var list ProviderList
	f.do("GET", "/api/providers", "", &list)
	for _, p := range list.Providers {
		if p.Name == "copilot" && !p.Enabled {
			t.Error("copilot not enabled after sign-in")
		}
	}
}

func TestStatic_ServesTheAppAndFallsBackToIndex(t *testing.T) {
	f := newFixture(t)
	get := func(p string) *httptest.ResponseRecorder {
		return f.raw(httptest.NewRequest("GET", "http://"+host+p, nil))
	}

	for _, p := range []string{"/", "/providers", "/settings/theme", "/agents/team/reviewer.yaml"} {
		if w := get(p); w.Code != 200 || !strings.Contains(w.Body.String(), "console") || w.Header().Get("Cache-Control") != "no-cache" {
			t.Errorf("%s: %d %q cache=%q", p, w.Code, w.Body.String(), w.Header().Get("Cache-Control"))
		}
	}
	if w := get("/_app/immutable/a.js"); w.Code != 200 || !strings.Contains(w.Header().Get("Cache-Control"), "immutable") {
		t.Errorf("immutable asset: %d %q", w.Code, w.Header().Get("Cache-Control"))
	}
	if w := get("/_app/immutable/missing.js"); w.Code != 404 {
		t.Errorf("missing asset: %d, want 404", w.Code)
	}
	// The mux cleans the path with a redirect; the handler cleans it again.
	if w := get("/../../etc/passwd"); w.Code != http.StatusMovedPermanently && !strings.Contains(w.Body.String(), "console") {
		t.Errorf("traversal: %d %q", w.Code, w.Body.String())
	}
	if w := get("/api/nope"); w.Code != 401 {
		t.Errorf("unknown API path without cookie: %d", w.Code)
	}
	if code := f.do("GET", "/api/nope", "", nil); code != 404 {
		t.Errorf("unknown API path: %d", code)
	}
}

func TestStatic_ExplainsAMissingBuild(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	h := New(Options{Assets: fstest.MapFS{".gitkeep": {}}, Token: "tok"}).Handler()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "http://"+host+"/", nil))
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "make web") {
		t.Errorf("not built: %d %q", w.Code, w.Body.String())
	}
}
