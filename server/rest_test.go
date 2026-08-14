package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Federicoand98/mani/app"
)

// fakeOllama serves Ollama's /api/chat well enough for an end-to-end turn:
// one streamed chunk with the reply, then a final chunk carrying the counters.
func fakeOllama(t *testing.T, reply string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		fl, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("ResponseWriter senza Flusher")
		}
		fmt.Fprintf(w, `{"message":{"role":"assistant","content":%q},"done":false}`+"\n", reply)
		fl.Flush()
		fmt.Fprint(w, `{"message":{"role":"assistant","content":""},"done":true,"done_reason":"stop",`+
			`"prompt_eval_count":11,"eval_count":7}`+"\n")
		fl.Flush()
	}))
	t.Cleanup(srv.Close)
	return srv
}

// testServer wires a Server whose agents talk to the fake provider.
//
// The provider base URL comes from the global config, so we redirect XDG_CONFIG_HOME
// to a temp dir and write a config.json pointing at the fake server: hermetic, and no
// production code has to grow a test seam.
func testServer(t *testing.T, token, reply string) *Server {
	t.Helper()
	llm := fakeOllama(t, reply)

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
	spec.Identity.Provider = "ollama"
	spec.Identity.Model = "test-model"
	// context injection reads AGENTS.md from disk: off, keeps the test hermetic
	spec.Context.Inject = false
	spec.Observability.Tracing = false

	return New(spec, token)
}

func do(t *testing.T, s *Server, method, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, r)
	return rec
}

// Ogni route sta dietro l'auth: nessuna deve essere raggiungibile senza token.
func TestRoutes_AllRequireAuth(t *testing.T) {
	s := testServer(t, "secret", "ciao")

	routes := []struct{ method, path string }{
		{http.MethodPost, "/sessions"},
		{http.MethodGet, "/sessions"},
		{http.MethodDelete, "/sessions/abc"},
		{http.MethodPost, "/sessions/abc/chat"},
		{http.MethodPost, "/chat"},
		{http.MethodGet, "/runs"},
		{http.MethodGet, "/runs/abc"},
		{http.MethodGet, "/sessions/abc/turn"},
	}

	for _, rt := range routes {
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			if rec := do(t, s, rt.method, rt.path, "", ""); rec.Code != http.StatusUnauthorized {
				t.Errorf("senza token: status = %d, atteso 401", rec.Code)
			}
		})
	}
}

func TestSessions_CreateListDelete(t *testing.T) {
	s := testServer(t, "secret", "ciao")

	rec := do(t, s, http.MethodPost, "/sessions", "secret", "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /sessions: status = %d, body = %s", rec.Code, rec.Body)
	}
	var created struct {
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil || created.SessionID == "" {
		t.Fatalf("session_id mancante: %s (%v)", rec.Body, err)
	}

	rec = do(t, s, http.MethodGet, "/sessions", "secret", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), created.SessionID) {
		t.Fatalf("GET /sessions non elenca la sessione: %s", rec.Body)
	}

	if rec = do(t, s, http.MethodDelete, "/sessions/"+created.SessionID, "secret", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE: status = %d", rec.Code)
	}
	if rec = do(t, s, http.MethodDelete, "/sessions/"+created.SessionID, "secret", ""); rec.Code != http.StatusNotFound {
		t.Errorf("la seconda DELETE deve dare 404, ottenuto %d", rec.Code)
	}
}

func TestChat_Stateless(t *testing.T) {
	s := testServer(t, "secret", "risposta del modello")

	rec := do(t, s, http.MethodPost, "/chat", "secret", `{"input":"ciao"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}

	var resp struct {
		Output map[string]any `json:"output"`
		Usage  struct {
			Input  int `json:"input"`
			Output int `json:"output"`
		} `json:"usage"`
		IsError bool `json:"is_error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("risposta non deserializzabile: %v (%s)", err, rec.Body)
	}
	if resp.IsError {
		t.Fatalf("risposta in errore: %s", rec.Body)
	}
	// senza output.schema il testo è avvolto in {"response": ...}
	if got := resp.Output["response"]; got != "risposta del modello" {
		t.Errorf("output = %v, atteso 'risposta del modello'", resp.Output)
	}
	if resp.Usage.Input != 11 || resp.Usage.Output != 7 {
		t.Errorf("usage = %+v, attesi 11/7", resp.Usage)
	}
}

func TestChat_OnSession(t *testing.T) {
	s := testServer(t, "secret", "ok")

	rec := do(t, s, http.MethodPost, "/sessions", "secret", "")
	var created struct {
		SessionID string `json:"session_id"`
	}
	json.Unmarshal(rec.Body.Bytes(), &created)

	rec = do(t, s, http.MethodPost, "/sessions/"+created.SessionID+"/chat", "secret", `{"input":"ciao"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("chat su sessione: status = %d, body = %s", rec.Code, rec.Body)
	}

	if rec = do(t, s, http.MethodPost, "/sessions/inesistente/chat", "secret", `{"input":"ciao"}`); rec.Code != http.StatusNotFound {
		t.Errorf("sessione inesistente: status = %d, atteso 404", rec.Code)
	}
}

func TestChat_RejectsInvalidBody(t *testing.T) {
	s := testServer(t, "secret", "ok")

	for name, body := range map[string]string{
		"json rotto":    `{"input":`,
		"input assente": `{}`,
		"input vuoto":   `{"input":""}`,
	} {
		t.Run(name, func(t *testing.T) {
			if rec := do(t, s, http.MethodPost, "/chat", "secret", body); rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, atteso 400", rec.Code)
			}
		})
	}
}

// Senza observability.journal.path non c'è vista unificata dei run:
// meglio un 501 esplicito che una lista vuota che sembra "nessun run".
func TestRuns_NotImplementedWithoutJournal(t *testing.T) {
	s := testServer(t, "secret", "ok")

	for _, path := range []string{"/runs", "/runs/abc"} {
		if rec := do(t, s, http.MethodGet, path, "secret", ""); rec.Code != http.StatusNotImplemented {
			t.Errorf("GET %s: status = %d, atteso 501", path, rec.Code)
		}
	}
}

func TestRuns_ListAndGet(t *testing.T) {
	s := testServer(t, "secret", "ok")

	// journal su disco: è la sorgente della vista unificata cross-sessione
	dir := t.TempDir()
	j, err := app.NewJSONLJournal(dir)
	if err != nil {
		t.Fatalf("NewJSONLJournal: %v", err)
	}
	s.journal = j

	if rec := do(t, s, http.MethodGet, "/runs", "secret", ""); rec.Code != http.StatusOK {
		t.Fatalf("GET /runs vuoto: status = %d", rec.Code)
	}
	if rec := do(t, s, http.MethodGet, "/runs/inesistente", "secret", ""); rec.Code != http.StatusNotFound {
		t.Errorf("run inesistente: status = %d, atteso 404", rec.Code)
	}
}
