package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Federicoand98/mani/app"
)

// fakeProvider serves Ollama's /api/chat with a scripted reply: plain text, or
// a call to the terminal `respond` tool when the manifest declares a schema.
func fakeProvider(t *testing.T, toolCall string, args map[string]any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			http.NotFound(w, r)
			return
		}
		msg := map[string]any{"role": "assistant", "content": "una risposta in testo"}
		if toolCall != "" {
			msg = map[string]any{"role": "assistant", "content": "", "tool_calls": []map[string]any{
				{"function": map[string]any{"name": toolCall, "arguments": args}},
			}}
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": msg, "done": true, "done_reason": "stop",
			"prompt_eval_count": 11, "eval_count": 7,
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

// cliHome writes a config.json pointing at the fake provider and returns the
// directory to use as XDG_CONFIG_HOME: hermetic, and no production code grows a
// test seam.
func cliHome(t *testing.T, providerURL string) string {
	t.Helper()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "mani"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := fmt.Sprintf(`{"provider":"ollama","providers":{"ollama":{"base_url":%q,"model":"test-model"}}}`, providerURL)
	if err := os.WriteFile(filepath.Join(home, "mani", "config.json"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	return home
}

// writeManifestBody writes a manifest verbatim; runs_test.go already has a
// writeManifest that builds one from a journal path.
func writeManifestBody(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "agent.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// mani runs the real binary: the test binary re-executes itself through the
// TestMain hook, so flag parsing and output go through the actual command.
func mani(t *testing.T, home string, args ...string) (stdout string, err error) {
	t.Helper()
	cmd := exec.Command(os.Args[0], args...)
	cmd.Env = append(os.Environ(), "MANI_TEST_RUN_MAIN=1", "XDG_CONFIG_HOME="+home)
	var errBuf strings.Builder
	cmd.Stderr = &errBuf
	out, err := cmd.Output()
	if err != nil {
		t.Logf("stderr:\n%s", errBuf.String())
	}
	return string(out), err
}

const schemaManifest = `
identity:
  name: classifier
  provider: ollama
  model: test-model
  prompt: "classify"
context:
  inject: false
output:
  schema:
    type: object
    properties:
      label: { type: string }
    required: [label]
observability:
  tracing: false
  journal:
    enabled: true
    path: %q
`

func TestRun_ProvenanceEnvelope(t *testing.T) {
	llm := fakeProvider(t, "respond", map[string]any{"label": "positive"})
	home := cliHome(t, llm.URL)
	runs := t.TempDir()
	manifest := writeManifestBody(t, fmt.Sprintf(schemaManifest, runs))

	out, err := mani(t, home, "run", "--config", manifest, "--task", "classifica", "--provenance")
	if err != nil {
		t.Fatalf("mani run: %v", err)
	}

	var got struct {
		Result map[string]any `json:"result"`
		Run    struct {
			ID        string `json:"id"`
			Source    string `json:"source"`
			Provider  string `json:"provider"`
			Model     string `json:"model"`
			Manifest  string `json:"manifest"`
			InTokens  int    `json:"in_tokens"`
			OutTokens int    `json:"out_tokens"`
		} `json:"run"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not the envelope: %v\n%s", err, out)
	}
	if got.Result["label"] != "positive" {
		t.Errorf("result = %+v", got.Result)
	}
	if got.Run.Provider != "ollama" || got.Run.Model != "test-model" || got.Run.Manifest != manifest {
		t.Errorf("run = %+v", got.Run)
	}
	if got.Run.Source != "cli" {
		t.Errorf("source = %q, want cli", got.Run.Source)
	}
	if got.Run.InTokens != 11 || got.Run.OutTokens != 7 {
		t.Errorf("tokens = %d/%d, want 11/7", got.Run.InTokens, got.Run.OutTokens)
	}

	// The whole point of the envelope: the id leads back to the run, and the
	// journal holds the same answer.
	j, err := app.NewJSONLJournal(runs)
	if err != nil {
		t.Fatalf("open journal: %v", err)
	}
	defer j.Close()
	rec, err := j.Get(got.Run.ID)
	if err != nil {
		t.Fatalf("run %q not in the journal: %v", got.Run.ID, err)
	}
	if rec.Status != "ok" || rec.Results["label"] != "positive" {
		t.Errorf("journal record = %s %+v", rec.Status, rec.Results)
	}
}

// Non-regression: every pipe doing `mani run … | jq '.label'` must keep
// working, so without the flag the output is the bare result.
func TestRun_WithoutProvenanceTheOutputIsBare(t *testing.T) {
	llm := fakeProvider(t, "respond", map[string]any{"label": "positive"})
	home := cliHome(t, llm.URL)
	manifest := writeManifestBody(t, fmt.Sprintf(schemaManifest, t.TempDir()))

	out, err := mani(t, home, "run", "--config", manifest, "--task", "classifica")
	if err != nil {
		t.Fatalf("mani run: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	if got["label"] != "positive" {
		t.Errorf("output = %+v, want the schema object itself", got)
	}
	if _, wrapped := got["run"]; wrapped {
		t.Errorf("output is wrapped without the flag: %+v", got)
	}
}

// A run without output.schema answers in text; the envelope still has to carry
// a JSON result, and {"response": …} is the shape POST /chat already uses.
func TestRun_ProvenanceWrapsTextAsResponse(t *testing.T) {
	llm := fakeProvider(t, "", nil)
	home := cliHome(t, llm.URL)
	manifest := writeManifestBody(t, `
identity:
  name: plain
  provider: ollama
  model: test-model
  prompt: "answer"
context:
  inject: false
observability:
  tracing: false
`)

	out, err := mani(t, home, "run", "--config", manifest, "--task", "ciao", "--provenance")
	if err != nil {
		t.Fatalf("mani run: %v", err)
	}

	var got struct {
		Result map[string]any `json:"result"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not the envelope: %v\n%s", err, out)
	}
	if got.Result["response"] != "una risposta in testo" {
		t.Errorf("result = %+v, want the text under \"response\"", got.Result)
	}
}

// --image used to be declared after flag parsing, so the flag did not exist
// and the run died with "flag provided but not defined".
func TestRun_ImageFlagIsAccepted(t *testing.T) {
	llm := fakeProvider(t, "", nil)
	home := cliHome(t, llm.URL)
	manifest := writeManifestBody(t, `
identity:
  name: plain
  provider: ollama
  model: test-model
  prompt: "answer"
context:
  inject: false
observability:
  tracing: false
`)
	img := filepath.Join(t.TempDir(), "dot.png")
	// The smallest valid PNG: 1×1, one pixel.
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89" +
		"\x00\x00\x00\rIDATx\x9cc\xf8\x0f\x00\x00\x01\x01\x00\x05\x18\xd8N\x00\x00\x00\x00IEND\xaeB`\x82")
	if err := os.WriteFile(img, png, 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := mani(t, home, "run", "--config", manifest, "--task", "describe", "--image", img)
	if err != nil {
		t.Fatalf("mani run --image: %v", err)
	}
	if !strings.Contains(out, "una risposta in testo") {
		t.Errorf("stdout = %q", out)
	}
}
