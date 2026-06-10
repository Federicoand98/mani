//go:build phase2

package fs

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// helper: scrive un file nella dir temporanea e ritorna il path assoluto.
func writeTemp(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("impossibile creare file temporaneo %q: %v", name, err)
	}
	return path
}

// --- test successo ---

func TestReadFileTool_Execute_ValidFile(t *testing.T) {
	dir := t.TempDir()
	writeTemp(t, dir, "hello.txt", "ciao mondo")
	tool := NewReadFileTool(dir)

	result, err := tool.Execute(context.Background(), map[string]any{"path": "hello.txt"})
	if err != nil {
		t.Fatalf("errore inatteso: %v", err)
	}
	if result != "ciao mondo" {
		t.Errorf("contenuto atteso 'ciao mondo', ottenuto %q", result)
	}
}

func TestReadFileTool_Execute_NestedPath(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "pkg", "core")
	os.MkdirAll(subdir, 0755)
	writeTemp(t, subdir, "types.go", "package core")
	tool := NewReadFileTool(dir)

	result, err := tool.Execute(context.Background(), map[string]any{"path": "pkg/core/types.go"})
	if err != nil {
		t.Fatalf("errore inatteso: %v", err)
	}
	if result != "package core" {
		t.Errorf("contenuto atteso 'package core', ottenuto %q", result)
	}
}

func TestReadFileTool_Execute_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	writeTemp(t, dir, "empty.txt", "")
	tool := NewReadFileTool(dir)

	result, err := tool.Execute(context.Background(), map[string]any{"path": "empty.txt"})
	if err != nil {
		t.Fatalf("errore inatteso per file vuoto: %v", err)
	}
	if result != "" {
		t.Errorf("atteso contenuto vuoto, ottenuto %q", result)
	}
}

func TestReadFileTool_Execute_MultilineContent(t *testing.T) {
	dir := t.TempDir()
	content := "riga 1\nriga 2\nriga 3\n"
	writeTemp(t, dir, "multi.txt", content)
	tool := NewReadFileTool(dir)

	result, err := tool.Execute(context.Background(), map[string]any{"path": "multi.txt"})
	if err != nil {
		t.Fatalf("errore inatteso: %v", err)
	}
	if result != content {
		t.Errorf("contenuto atteso %q, ottenuto %q", content, result)
	}
}

func TestReadFileTool_Execute_FileWithUnicode(t *testing.T) {
	dir := t.TempDir()
	content := "// Package core — cuore dell'architettura\npackage core\n"
	writeTemp(t, dir, "core.go", content)
	tool := NewReadFileTool(dir)

	result, err := tool.Execute(context.Background(), map[string]any{"path": "core.go"})
	if err != nil {
		t.Fatalf("errore inatteso: %v", err)
	}
	if result != content {
		t.Errorf("contenuto con unicode non preservato")
	}
}

// --- test fallimento: parametri errati ---

func TestReadFileTool_Execute_MissingPathParam(t *testing.T) {
	tool := NewReadFileTool(t.TempDir())

	_, err := tool.Execute(context.Background(), map[string]any{})
	if err == nil {
		t.Fatal("atteso errore per parametro 'path' mancante, ottenuto nil")
	}
}

func TestReadFileTool_Execute_PathNotString(t *testing.T) {
	tool := NewReadFileTool(t.TempDir())

	_, err := tool.Execute(context.Background(), map[string]any{"path": 42})
	if err == nil {
		t.Fatal("atteso errore per 'path' non stringa, ottenuto nil")
	}
}

func TestReadFileTool_Execute_NilInput(t *testing.T) {
	tool := NewReadFileTool(t.TempDir())

	_, err := tool.Execute(context.Background(), nil)
	if err == nil {
		t.Fatal("atteso errore per input nil, ottenuto nil")
	}
}

// --- test fallimento: file non trovato ---

func TestReadFileTool_Execute_NonexistentFile(t *testing.T) {
	tool := NewReadFileTool(t.TempDir())

	_, err := tool.Execute(context.Background(), map[string]any{"path": "non_esiste.txt"})
	if err == nil {
		t.Fatal("atteso errore per file inesistente, ottenuto nil")
	}
}

func TestReadFileTool_Execute_NonexistentDirectory(t *testing.T) {
	tool := NewReadFileTool(t.TempDir())

	_, err := tool.Execute(context.Background(), map[string]any{"path": "dir_inesistente/file.txt"})
	if err == nil {
		t.Fatal("atteso errore per directory inesistente, ottenuto nil")
	}
}

// --- test sicurezza: path traversal ---

func TestReadFileTool_Execute_PathTraversal_DotDot(t *testing.T) {
	tool := NewReadFileTool(t.TempDir())

	_, err := tool.Execute(context.Background(), map[string]any{"path": "../../etc/passwd"})
	if err == nil {
		t.Fatal("SECURITY: atteso errore per path traversal con .., ottenuto nil")
	}
}

func TestReadFileTool_Execute_PathTraversal_Hidden(t *testing.T) {
	// .. nascosto dentro un percorso apparentemente innocuo
	tool := NewReadFileTool(t.TempDir())

	_, err := tool.Execute(context.Background(), map[string]any{"path": "subdir/../../../../../../etc/passwd"})
	if err == nil {
		t.Fatal("SECURITY: atteso errore per path traversal nascosto, ottenuto nil")
	}
}

func TestReadFileTool_Execute_PathTraversal_AbsoluteOutside(t *testing.T) {
	// path assoluto che punta fuori dalla workspace
	tool := NewReadFileTool(t.TempDir())

	_, err := tool.Execute(context.Background(), map[string]any{"path": "/etc/passwd"})
	if err == nil {
		t.Fatal("SECURITY: atteso errore per path assoluto fuori dalla workspace, ottenuto nil")
	}
}

func TestReadFileTool_Execute_PathTraversal_AbsoluteInsideWorkspace(t *testing.T) {
	// un path assoluto che PUNTA dentro la workspace — comportamento implementation-defined,
	// ma l'errore è accettabile; l'importante è che /etc/passwd non sia accessibile
	dir := t.TempDir()
	writeTemp(t, dir, "ok.txt", "ok")
	absPath := filepath.Join(dir, "ok.txt")
	tool := NewReadFileTool(dir)

	// il tool può accettare o rifiutare path assoluti interni — ma non deve crashare
	_, _ = tool.Execute(context.Background(), map[string]any{"path": absPath})
}

func TestReadFileTool_Execute_PathTraversal_OnlyDots(t *testing.T) {
	tool := NewReadFileTool(t.TempDir())

	for _, p := range []string{"..", "../..", ".", "../"} {
		_, err := tool.Execute(context.Background(), map[string]any{"path": p})
		if err == nil {
			t.Errorf("SECURITY: atteso errore per path %q, ottenuto nil", p)
		}
	}
}

func TestReadFileTool_Execute_EmptyPath_IsError(t *testing.T) {
	// path vuoto è la root stessa — una directory, non un file
	tool := NewReadFileTool(t.TempDir())

	_, err := tool.Execute(context.Background(), map[string]any{"path": ""})
	if err == nil {
		t.Fatal("atteso errore per path vuoto, ottenuto nil")
	}
}

// --- test schema e metadata ---

func TestReadFileTool_Name(t *testing.T) {
	tool := NewReadFileTool("/tmp")
	if tool.Name() != "read_file" {
		t.Errorf("Name atteso 'read_file', ottenuto %q", tool.Name())
	}
}

func TestReadFileTool_Description_NotEmpty(t *testing.T) {
	tool := NewReadFileTool("/tmp")
	if tool.Description() == "" {
		t.Error("Description non deve essere vuota")
	}
}

func TestReadFileTool_Schema_NameMatches(t *testing.T) {
	tool := NewReadFileTool("/tmp")
	if tool.Schema().Name != tool.Name() {
		t.Errorf("Schema.Name %q non corrisponde a Name() %q", tool.Schema().Name, tool.Name())
	}
}

func TestReadFileTool_Schema_HasPathProperty(t *testing.T) {
	tool := NewReadFileTool("/tmp")
	schema := tool.Schema()

	if _, ok := schema.InputSchema.Properties["path"]; !ok {
		t.Error("schema deve avere la proprietà 'path'")
	}
}

func TestReadFileTool_Schema_PathIsString(t *testing.T) {
	tool := NewReadFileTool("/tmp")
	prop := tool.Schema().InputSchema.Properties["path"]

	if prop.Type != "string" {
		t.Errorf("path.Type atteso 'string', ottenuto %q", prop.Type)
	}
}

func TestReadFileTool_Schema_PathIsRequired(t *testing.T) {
	tool := NewReadFileTool("/tmp")
	for _, r := range tool.Schema().InputSchema.Required {
		if r == "path" {
			return
		}
	}
	t.Error("'path' deve essere nel campo Required dello schema")
}

func TestReadFileTool_Schema_TypeIsObject(t *testing.T) {
	tool := NewReadFileTool("/tmp")
	if tool.Schema().InputSchema.Type != "object" {
		t.Errorf("InputSchema.Type atteso 'object', ottenuto %q", tool.Schema().InputSchema.Type)
	}
}
