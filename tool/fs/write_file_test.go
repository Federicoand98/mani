package fs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newWriteTool(t *testing.T) (*WriteFileTool, string) {
	t.Helper()
	root := t.TempDir()
	return NewWriteFileTool(root), root
}

func TestWriteFile_CreatesNewFile(t *testing.T) {
	tool, root := newWriteTool(t)
	_, err := tool.Execute(context.Background(), map[string]any{
		"path":    "new.txt",
		"content": "ciao",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(root, "new.txt"))
	if err != nil {
		t.Fatalf("file non creato: %v", err)
	}
	if string(got) != "ciao" {
		t.Errorf("contenuto atteso 'ciao', ottenuto %q", got)
	}
}

func TestWriteFile_Overwrites(t *testing.T) {
	tool, root := newWriteTool(t)
	p := filepath.Join(root, "x.txt")
	if err := os.WriteFile(p, []byte("vecchio"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	_, err := tool.Execute(context.Background(), map[string]any{
		"path":    "x.txt",
		"content": "nuovo",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	got, _ := os.ReadFile(p)
	if string(got) != "nuovo" {
		t.Errorf("contenuto atteso 'nuovo', ottenuto %q", got)
	}
}

func TestWriteFile_OutsideWorkspace_Errors(t *testing.T) {
	tool, _ := newWriteTool(t)
	_, err := tool.Execute(context.Background(), map[string]any{
		"path":    "../escape.txt",
		"content": "x",
	})
	if err == nil {
		t.Error("atteso errore per path fuori workspace")
	}
}

func TestWriteFile_MissingInputs_Errors(t *testing.T) {
	tool, _ := newWriteTool(t)

	if _, err := tool.Execute(context.Background(), map[string]any{"content": "x"}); err == nil {
		t.Error("atteso errore con path mancante")
	}
	if _, err := tool.Execute(context.Background(), map[string]any{"path": "a.txt"}); err == nil {
		t.Error("atteso errore con content mancante")
	}
}

func TestWriteFile_NonStringInputs_Errors(t *testing.T) {
	tool, _ := newWriteTool(t)
	_, err := tool.Execute(context.Background(), map[string]any{
		"path":    42,
		"content": "x",
	})
	if err == nil || !strings.Contains(err.Error(), "string") {
		t.Errorf("atteso errore tipo, ottenuto %v", err)
	}
}
