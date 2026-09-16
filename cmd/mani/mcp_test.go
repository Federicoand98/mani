package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestMain lets the test binary stand in for the mani binary: with
// MANI_TEST_RUN_MAIN set it runs main() instead of the tests. The end-to-end test
// re-executes itself this way, so it exercises the real process — flag parsing,
// logging setup, stdio — without a separate go build.
func TestMain(m *testing.M) {
	if os.Getenv("MANI_TEST_RUN_MAIN") == "1" {
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// On stdio, stdout IS the protocol. A single stray byte there — a log line, a
// debug print — and the client drops the connection with an error that says
// nothing. It is the most common way to break an MCP server, and it stays
// invisible until someone plugs the server into a client.
func TestMCP_StdoutCarriesOnlyJSONRPC(t *testing.T) {
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		fmt.Fprintln(w, `{"message":{"role":"assistant","content":"hello from the agent"},`+
			`"done":true,"done_reason":"stop","prompt_eval_count":11,"eval_count":7}`)
	}))
	defer llm.Close()

	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "mani"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := fmt.Sprintf(`{"provider":"ollama","providers":{"ollama":{"base_url":%q,"model":"test-model"}}}`, llm.URL)
	if err := os.WriteFile(filepath.Join(home, "mani", "config.json"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}

	// Tracing on and the log level at debug: the point is to produce as much
	// logging as possible and prove none of it lands on stdout.
	manifest := filepath.Join(t.TempDir(), "agent.yaml")
	if err := os.WriteFile(manifest, []byte(`
identity:
  name: greeter
  description: Says hello.
  provider: ollama
  model: test-model
context:
  inject: false
observability:
  tracing: true
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(os.Args[0], "mcp", "--config", manifest)
	cmd.Env = append(os.Environ(),
		"MANI_TEST_RUN_MAIN=1",
		"XDG_CONFIG_HOME="+home,
		"MANI_LOG_LEVEL=debug",
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })

	for _, msg := range []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"greeter","arguments":{"task":"say hello"}}}`,
	} {
		if _, err := io.WriteString(stdin, msg+"\n"); err != nil {
			t.Fatalf("write %s: %v", msg, err)
		}
	}

	type line struct {
		raw string
		msg map[string]any
		err error
	}
	lines := make(chan line)
	go func() {
		defer close(lines)
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for sc.Scan() {
			l := line{raw: sc.Text()}
			l.err = json.Unmarshal(sc.Bytes(), &l.msg)
			lines <- l
		}
	}()

	var answer map[string]any
	timeout := time.After(20 * time.Second)
	for answer == nil {
		select {
		case l, ok := <-lines:
			if !ok {
				t.Fatalf("stdout closed before the tools/call answer; stderr:\n%s", stderr.String())
			}
			if l.err != nil || l.msg["jsonrpc"] != "2.0" {
				t.Fatalf("stdout carried a non JSON-RPC line: %q", l.raw)
			}
			if id, _ := l.msg["id"].(float64); id == 2 {
				answer = l.msg
			}
		case <-timeout:
			t.Fatalf("no answer to tools/call within 20s; stderr:\n%s", stderr.String())
		}
	}

	result, _ := answer["result"].(map[string]any)
	if result == nil {
		t.Fatalf("tools/call answered with %v, want a result", answer)
	}
	if isErr, _ := result["isError"].(bool); isErr {
		t.Fatalf("tools/call failed: %v", result)
	}
	if !strings.Contains(fmt.Sprint(result["content"]), "hello from the agent") {
		t.Errorf("result content = %v, want the agent's reply", result["content"])
	}

	// A client closing stdin is how an MCP session ends: the process must exit,
	// and nothing it writes on the way out may break the stream either.
	_ = stdin.Close()
	for l := range lines {
		if l.err != nil || l.msg["jsonrpc"] != "2.0" {
			t.Errorf("stdout carried a non JSON-RPC line after close: %q", l.raw)
		}
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("mani mcp exited with %v after stdin closed; stderr:\n%s", err, stderr.String())
		}
	case <-time.After(5 * time.Second):
		t.Errorf("mani mcp did not exit within 5s of stdin closing")
	}

	// The logs went somewhere: if stderr were empty, the test would prove nothing.
	if stderr.Len() == 0 {
		t.Error("stderr is empty: logging was not exercised, so the stdout check proves nothing")
	}
}
