package tool

import (
	"context"
	"testing"

	"github.com/Federicoand98/mani/core"
)

// mockToolWithSchema è un Tool più ricco per testare la conversione.
type mockToolWithSchema struct {
	toolName   string
	toolDesc   string
	toolRisk   core.RiskLevel
	toolSchema ToolSchema
}

func (m mockToolWithSchema) Name() string             { return m.toolName }
func (m mockToolWithSchema) Description() string      { return m.toolDesc }
func (m mockToolWithSchema) Schema() ToolSchema       { return m.toolSchema }
func (m mockToolWithSchema) RiskLevel() core.RiskLevel { return m.toolRisk }
func (m mockToolWithSchema) Execute(_ context.Context, _ map[string]any) (string, error) {
	return "", nil
}

func makeReadFileTool() mockToolWithSchema {
	return mockToolWithSchema{
		toolName: "read_file",
		toolDesc: "Legge il contenuto di un file di testo.",
		toolSchema: ToolSchema{
			Name:        "read_file",
			Description: "Legge il contenuto di un file di testo.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertySchema{
					"path": {Type: "string", Description: "Percorso del file"},
				},
				Required: []string{"path"},
			},
		},
	}
}

func TestToDefinition_Name(t *testing.T) {
	def := ToDefinition(makeReadFileTool())
	if def.Name != "read_file" {
		t.Errorf("Name atteso 'read_file', ottenuto %q", def.Name)
	}
}

func TestToDefinition_Description(t *testing.T) {
	def := ToDefinition(makeReadFileTool())
	if def.Description != "Legge il contenuto di un file di testo." {
		t.Errorf("Description attesa non corrisponde, ottenuto %q", def.Description)
	}
}

func TestToDefinition_InputSchemaType(t *testing.T) {
	def := ToDefinition(makeReadFileTool())
	if def.InputSchema.Type != "object" {
		t.Errorf("InputSchema.Type atteso 'object', ottenuto %q", def.InputSchema.Type)
	}
}

func TestToDefinition_Properties_Mapped(t *testing.T) {
	def := ToDefinition(makeReadFileTool())

	prop, ok := def.InputSchema.Properties["path"]
	if !ok {
		t.Fatal("proprietà 'path' non trovata nella ToolDefinition")
	}
	if prop.Type != "string" {
		t.Errorf("prop.Type atteso 'string', ottenuto %q", prop.Type)
	}
	if prop.Description != "Percorso del file" {
		t.Errorf("prop.Description atteso 'Percorso del file', ottenuto %q", prop.Description)
	}
}

func TestToDefinition_Required_Preserved(t *testing.T) {
	def := ToDefinition(makeReadFileTool())

	if len(def.InputSchema.Required) != 1 {
		t.Fatalf("atteso 1 required, ottenuti %d", len(def.InputSchema.Required))
	}
	if def.InputSchema.Required[0] != "path" {
		t.Errorf("required[0] atteso 'path', ottenuto %q", def.InputSchema.Required[0])
	}
}

func TestToDefinition_MultipleProperties(t *testing.T) {
	mt := mockToolWithSchema{
		toolName: "write_file",
		toolSchema: ToolSchema{
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertySchema{
					"path":    {Type: "string", Description: "Percorso"},
					"content": {Type: "string", Description: "Contenuto"},
				},
				Required: []string{"path", "content"},
			},
		},
	}
	def := ToDefinition(mt)

	if len(def.InputSchema.Properties) != 2 {
		t.Errorf("attese 2 proprietà, ottenute %d", len(def.InputSchema.Properties))
	}
	if len(def.InputSchema.Required) != 2 {
		t.Errorf("attesi 2 required, ottenuti %d", len(def.InputSchema.Required))
	}
}

func TestToDefinition_EmptyProperties(t *testing.T) {
	mt := mockToolWithSchema{
		toolName: "noop",
		toolSchema: ToolSchema{
			InputSchema: InputSchema{
				Type:       "object",
				Properties: map[string]PropertySchema{},
				Required:   []string{},
			},
		},
	}
	def := ToDefinition(mt)

	if len(def.InputSchema.Properties) != 0 {
		t.Errorf("attese 0 proprietà, ottenute %d", len(def.InputSchema.Properties))
	}
}

func TestToDefinition_ReturnsCorrectType(t *testing.T) {
	// verifica compile-time che ToDefinition ritorni core.ToolDefinition
	var _ core.ToolDefinition = ToDefinition(makeReadFileTool())
}

func TestToDefinition_PropertiesNotShared(t *testing.T) {
	// la map delle properties deve essere una copia, non condivisa
	mt := makeReadFileTool()
	def := ToDefinition(mt)

	// modificare la definizione non deve alterare lo schema originale
	def.InputSchema.Properties["injected"] = core.ToolProperty{Type: "string"}

	if _, ok := mt.Schema().InputSchema.Properties["injected"]; ok {
		t.Error("la modifica alla ToolDefinition non deve alterare lo Schema originale")
	}
}
