package tool

import (
	"context"
	"testing"
)

type mockTool struct {
	name string
}

func (m mockTool) Name() string        { return m.name }
func (m mockTool) Description() string { return "mock" }
func (m mockTool) Schema() ToolSchema  { return ToolSchema{Name: m.name} }
func (m mockTool) Execute(_ context.Context, _ map[string]any) (string, error) {
	return "ok", nil
}

func TestToolRegistry_RegisterAndGet(t *testing.T) {
	r := NewToolRegistry()
	r.Register(mockTool{name: "read_file"})

	got, ok := r.Get("read_file")
	if !ok {
		t.Fatal("tool 'read_file' non trovato dopo Register")
	}
	if got.Name() != "read_file" {
		t.Errorf("nome atteso 'read_file', ottenuto %q", got.Name())
	}
}

func TestToolRegistry_GetNonExistent(t *testing.T) {
	r := NewToolRegistry()
	_, ok := r.Get("non_esiste")
	if ok {
		t.Error("Get su tool non registrato dovrebbe restituire false")
	}
}

func TestToolRegistry_List(t *testing.T) {
	r := NewToolRegistry()
	r.Register(mockTool{name: "tool_a"})
	r.Register(mockTool{name: "tool_b"})

	tools := r.List()
	if len(tools) != 2 {
		t.Errorf("attesi 2 tools, ottenuti %d", len(tools))
	}
}

func TestToolRegistry_RegisterOverwrite(t *testing.T) {
	r := NewToolRegistry()
	r.Register(mockTool{name: "tool_a"})
	r.Register(mockTool{name: "tool_a"})

	// registrare lo stesso nome due volte non deve duplicarlo
	tools := r.List()
	if len(tools) != 1 {
		t.Errorf("atteso 1 tool dopo doppia registrazione, ottenuti %d", len(tools))
	}
}

func TestToolRegistry_ListEmpty(t *testing.T) {
	r := NewToolRegistry()
	tools := r.List()
	if len(tools) != 0 {
		t.Errorf("atteso registry vuoto, ottenuti %d tools", len(tools))
	}
}
