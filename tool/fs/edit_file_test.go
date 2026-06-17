package fs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newEditTool(t *testing.T) (*EditFileTool, string) {
	t.Helper()
	root := t.TempDir()
	return NewEditFileTool(root), root
}

func writeTemp(t *testing.T, root, name, content string) string {
	t.Helper()
	p := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	return p
}

func TestEditFile_SingleOccurrence_Replaces(t *testing.T) {
	tool, root := newEditTool(t)
	p := writeTemp(t, root, "main.go", "package main\nfunc Hello() {}\n")

	out, err := tool.Execute(context.Background(), map[string]any{
		"path":        "main.go",
		"old_content": "Hello",
		"new_content": "Ciao",
	})
	if err != nil {
		t.Fatalf("Execute errore: %v", err)
	}
	if !strings.Contains(out, "edited file") {
		t.Errorf("output atteso 'edited file', ottenuto %q", out)
	}
	got, _ := os.ReadFile(p)
	if !strings.Contains(string(got), "func Ciao()") {
		t.Errorf("file non aggiornato: %q", got)
	}
}

func TestEditFile_ZeroOccurrences_Errors(t *testing.T) {
	tool, root := newEditTool(t)
	writeTemp(t, root, "a.txt", "abc")

	_, err := tool.Execute(context.Background(), map[string]any{
		"path":        "a.txt",
		"old_content": "xyz",
		"new_content": "qqq",
	})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("atteso errore 'not found', ottenuto %v", err)
	}
}

func TestEditFile_MultipleOccurrences_Errors(t *testing.T) {
	tool, root := newEditTool(t)
	original := "foo foo foo"
	p := writeTemp(t, root, "a.txt", original)

	_, err := tool.Execute(context.Background(), map[string]any{
		"path":        "a.txt",
		"old_content": "foo",
		"new_content": "bar",
	})
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("atteso errore 'ambiguous', ottenuto %v", err)
	}
	got, _ := os.ReadFile(p)
	if string(got) != original {
		t.Errorf("file modificato su errore: %q", got)
	}
}

func TestEditFile_EmptyOldContent_Errors(t *testing.T) {
	tool, root := newEditTool(t)
	writeTemp(t, root, "a.txt", "hello")

	_, err := tool.Execute(context.Background(), map[string]any{
		"path":        "a.txt",
		"old_content": "",
		"new_content": "x",
	})
	if err == nil || !strings.Contains(err.Error(), "cannot be empty") {
		t.Errorf("atteso errore su old_content vuoto, ottenuto %v", err)
	}
}

func TestEditFile_MissingPath_Errors(t *testing.T) {
	tool, _ := newEditTool(t)
	_, err := tool.Execute(context.Background(), map[string]any{
		"old_content": "x",
		"new_content": "y",
	})
	if err == nil || !strings.Contains(err.Error(), "path") {
		t.Errorf("atteso errore su path mancante, ottenuto %v", err)
	}
}

func TestEditFile_PathNotString_Errors(t *testing.T) {
	tool, _ := newEditTool(t)
	_, err := tool.Execute(context.Background(), map[string]any{
		"path":        42,
		"old_content": "x",
		"new_content": "y",
	})
	if err == nil || !strings.Contains(err.Error(), "string") {
		t.Errorf("atteso errore su path non stringa, ottenuto %v", err)
	}
}

func TestEditFile_OutsideWorkspace_Errors(t *testing.T) {
	tool, _ := newEditTool(t)
	_, err := tool.Execute(context.Background(), map[string]any{
		"path":        "../escape.txt",
		"old_content": "x",
		"new_content": "y",
	})
	if err == nil {
		t.Error("atteso errore per path fuori workspace")
	}
}

func TestEditFile_NonexistentFile_Errors(t *testing.T) {
	tool, _ := newEditTool(t)
	_, err := tool.Execute(context.Background(), map[string]any{
		"path":        "missing.txt",
		"old_content": "x",
		"new_content": "y",
	})
	if err == nil || !strings.Contains(err.Error(), "failed to read") {
		t.Errorf("atteso errore di lettura, ottenuto %v", err)
	}
}

func TestEditFile_PreservesPermissions(t *testing.T) {
	tool, root := newEditTool(t)
	p := filepath.Join(root, "a.txt")
	if err := os.WriteFile(p, []byte("hello"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	_, err := tool.Execute(context.Background(), map[string]any{
		"path":        "a.txt",
		"old_content": "hello",
		"new_content": "world",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	info, err := os.Stat(p)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("perm attesi 0600, ottenuti %o", info.Mode().Perm())
	}
}

func TestEditFile_ContextCancelled_Errors(t *testing.T) {
	tool, root := newEditTool(t)
	writeTemp(t, root, "a.txt", "hello")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := tool.Execute(ctx, map[string]any{
		"path":        "a.txt",
		"old_content": "hello",
		"new_content": "world",
	})
	if err == nil {
		t.Error("atteso errore per context cancellato")
	}
}
