package bash

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBash_ValidCommand_ReturnsOutput(t *testing.T) {
	b := NewBashTool(t.TempDir())
	out, err := b.Execute(context.Background(), map[string]any{"command": "echo ok"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if strings.TrimSpace(out) != "ok" {
		t.Errorf("output atteso 'ok', ottenuto %q", out)
	}
}

func TestBash_NonZeroExit_ReturnsOutputNotError(t *testing.T) {
	b := NewBashTool(t.TempDir())
	out, err := b.Execute(context.Background(), map[string]any{
		"command": "echo boom >&2; exit 3",
	})
	if err != nil {
		t.Fatalf("un exit code non-zero non deve essere un errore del tool, ottenuto: %v", err)
	}
	if !strings.Contains(out, "exit status 3") {
		t.Errorf("il risultato deve riportare l'exit code, ottenuto %q", out)
	}
	if !strings.Contains(out, "boom") {
		t.Errorf("il risultato deve contenere stderr, ottenuto %q", out)
	}
}

func TestBash_RespectsExternalTimeout(t *testing.T) {
	b := NewBashTool(t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := b.Execute(ctx, map[string]any{"command": "sleep 5"})
	if err == nil {
		t.Fatal("atteso errore per timeout del context")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("atteso context.DeadlineExceeded, ottenuto %v", err)
	}
}

func TestBash_MissingCommand_Errors(t *testing.T) {
	b := NewBashTool(t.TempDir())
	_, err := b.Execute(context.Background(), map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "string") {
		t.Errorf("atteso errore di validazione, ottenuto %v", err)
	}
}

func TestBash_CommandNotString_Errors(t *testing.T) {
	b := NewBashTool(t.TempDir())
	_, err := b.Execute(context.Background(), map[string]any{"command": 42})
	if err == nil {
		t.Error("atteso errore per command non stringa")
	}
}

func TestBash_RunsInWorkspaceDir(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "probe.txt"), []byte("in-workspace"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	b := NewBashTool(root)

	// Si verifica il COMPORTAMENTO — il comando gira dentro il workspace — e non
	// la stringa stampata da `pwd`. Confrontare i path non puo' funzionare:
	//   - su macOS t.TempDir() da' /var/... mentre pwd stampa /private/var/...
	//   - su Windows la shell rilevata e' Git Bash, che stampa un path POSIX
	//     tradotto ("/tmp/...") dove Go riporta "C:\Users\...\Temp\...".
	// Leggere un file che esiste solo nel workspace prova la stessa cosa ovunque.
	out, err := b.Execute(context.Background(), map[string]any{"command": "cat probe.txt"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(out, "in-workspace") {
		t.Errorf("il comando non e' stato eseguito nel workspace, output: %q", out)
	}
}

func TestBash_ContextCancelled_Errors(t *testing.T) {
	b := NewBashTool(t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := b.Execute(ctx, map[string]any{"command": "echo x"})
	if err == nil {
		t.Error("atteso errore per context cancellato")
	}
}
