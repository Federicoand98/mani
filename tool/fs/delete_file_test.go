package fs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newDeleteTool(t *testing.T) (*DeleteFileTool, string) {
	t.Helper()
	root := t.TempDir()
	return NewDeleteFileTool(root), root
}

func TestDeleteFile_ExistingFile_Removed(t *testing.T) {
	tool, root := newDeleteTool(t)
	p := filepath.Join(root, "a.txt")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if _, err := tool.Execute(context.Background(), map[string]any{"path": "a.txt"}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Errorf("file ancora presente, errore stat = %v", err)
	}
}

func TestDeleteFile_NonexistentFile_Errors(t *testing.T) {
	tool, _ := newDeleteTool(t)
	_, err := tool.Execute(context.Background(), map[string]any{"path": "missing.txt"})
	if err == nil {
		t.Error("atteso errore per file non esistente")
	}
}

func TestDeleteFile_OutsideWorkspace_Errors(t *testing.T) {
	tool, _ := newDeleteTool(t)
	_, err := tool.Execute(context.Background(), map[string]any{"path": "../escape.txt"})
	if err == nil {
		t.Error("atteso errore per path fuori workspace")
	}
}

func TestDeleteFile_PathNotString_Errors(t *testing.T) {
	tool, _ := newDeleteTool(t)
	_, err := tool.Execute(context.Background(), map[string]any{"path": 42})
	if err == nil || !strings.Contains(err.Error(), "string") {
		t.Errorf("atteso errore tipo, ottenuto %v", err)
	}
}

func TestDeleteFile_EmptyDirectory_Removed(t *testing.T) {
	// os.Remove rimuove directory vuote: documentiamo il comportamento.
	tool, root := newDeleteTool(t)
	dir := filepath.Join(root, "empty")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if _, err := tool.Execute(context.Background(), map[string]any{"path": "empty"}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("dir vuota deve essere rimossa")
	}
}

func TestDeleteFile_NonEmptyDirectory_Errors(t *testing.T) {
	// os.Remove fallisce su directory non vuote.
	tool, root := newDeleteTool(t)
	dir := filepath.Join(root, "full")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "f"), []byte("x"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	_, err := tool.Execute(context.Background(), map[string]any{"path": "full"})
	if err == nil {
		t.Error("atteso errore su directory non vuota")
	}
}
