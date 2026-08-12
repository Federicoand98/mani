package subprocess

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Federicoand98/mani/core"
	"github.com/Federicoand98/mani/tool"
)

// writeScript drops an executable shell script into dir and returns its name.
func writeScript(t *testing.T, dir, name, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("script di shell: test non applicabile su Windows")
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatalf("scrittura script: %v", err)
	}
	return path
}

func newTool(t *testing.T, ws, command string, env map[string]string) tool.Tool {
	t.Helper()
	return New("probe", "a probe tool", command, nil, env,
		tool.ToolSchema{Name: "probe", InputSchema: tool.InputSchema{Type: "object"}},
		core.RiskExecute, ws)
}

// Il contratto: input JSON su stdin, risultato su stdout.
func TestSubprocess_JSONOnStdinResultOnStdout(t *testing.T) {
	ws := t.TempDir()
	writeScript(t, ws, "echo.sh", "cat\n") // rimanda indietro ciò che riceve

	tl := newTool(t, ws, "./echo.sh", nil)
	out, err := tl.Execute(context.Background(), map[string]any{"symbol": "ACME", "n": 3})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, `"symbol":"ACME"`) || !strings.Contains(out, `"n":3`) {
		t.Errorf("l'input non è arrivato su stdin come JSON: %q", out)
	}
}

// Exit non-zero: stderr diventa l'errore che il modello vede e può correggere.
func TestSubprocess_NonZeroExitSurfacesStderr(t *testing.T) {
	ws := t.TempDir()
	writeScript(t, ws, "fail.sh", "echo 'symbol non valido' >&2\nexit 2\n")

	tl := newTool(t, ws, "./fail.sh", nil)
	out, err := tl.Execute(context.Background(), map[string]any{})
	if err == nil {
		t.Fatal("un exit non-zero deve produrre un errore")
	}
	if !strings.Contains(err.Error(), "symbol non valido") {
		t.Errorf("stderr non riportato nell'errore: %v", err)
	}
	if out != "" {
		t.Errorf("output atteso vuoto in errore, ottenuto %q", out)
	}
}

// Se il processo fallisce senza scrivere su stderr, l'errore non deve essere vuoto.
func TestSubprocess_FailureWithoutStderrStillErrors(t *testing.T) {
	ws := t.TempDir()
	writeScript(t, ws, "silent.sh", "exit 1\n")

	tl := newTool(t, ws, "./silent.sh", nil)
	if _, err := tl.Execute(context.Background(), map[string]any{}); err == nil {
		t.Fatal("atteso errore")
	} else if !strings.Contains(err.Error(), "probe") {
		t.Errorf("l'errore deve nominare il tool: %v", err)
	}
}

// Un comando relativo si risolve dentro il workspace, non nella cwd del processo.
func TestSubprocess_RelativeCommandResolvesInWorkspace(t *testing.T) {
	ws := t.TempDir()
	writeScript(t, ws, "hello.sh", "echo ciao\n")

	tl := newTool(t, ws, "./hello.sh", nil)
	out, err := tl.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if strings.TrimSpace(out) != "ciao" {
		t.Errorf("output = %q, atteso 'ciao'", out)
	}
}

// Le variabili dichiarate nel manifest arrivano al processo.
func TestSubprocess_EnvIsPassed(t *testing.T) {
	ws := t.TempDir()
	writeScript(t, ws, "env.sh", "echo \"$API_MODE\"\n")

	tl := newTool(t, ws, "./env.sh", map[string]string{"API_MODE": "sandbox"})
	out, err := tl.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if strings.TrimSpace(out) != "sandbox" {
		t.Errorf("env non propagato: %q", out)
	}
}

// Un comando inesistente non deve andare in panic: è un errore normale del tool.
func TestSubprocess_MissingCommandIsAnError(t *testing.T) {
	tl := newTool(t, t.TempDir(), "./non-esiste.sh", nil)
	if _, err := tl.Execute(context.Background(), map[string]any{}); err == nil {
		t.Fatal("un comando inesistente deve dare errore")
	}
}

// La cancellazione del context ferma il processo: è ciò che rende
// efficace il tool_timeout dei limits.
func TestSubprocess_ContextCancellationStopsProcess(t *testing.T) {
	ws := t.TempDir()
	writeScript(t, ws, "slow.sh", "sleep 30\n")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	tl := newTool(t, ws, "./slow.sh", nil)
	if _, err := tl.Execute(ctx, map[string]any{}); err == nil {
		t.Fatal("con context cancellato l'esecuzione deve fallire subito")
	}
}

// Metadati esposti all'LLM e al gate dei permessi.
func TestSubprocess_Metadata(t *testing.T) {
	tl := newTool(t, t.TempDir(), "./x.sh", nil)

	if tl.Name() != "probe" {
		t.Errorf("Name = %q", tl.Name())
	}
	if tl.Description() != "a probe tool" {
		t.Errorf("Description = %q", tl.Description())
	}
	if tl.RiskLevel() != core.RiskExecute {
		t.Errorf("RiskLevel = %v, atteso execute", tl.RiskLevel())
	}
	if tl.Schema().InputSchema.Type != "object" {
		t.Errorf("schema non esposto: %+v", tl.Schema())
	}
}
