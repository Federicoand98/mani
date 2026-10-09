package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// What a provider and an MCP client receive must be a plain JSON Schema enum:
// a list of strings, not the struct mani uses internally.
func TestEnumValues_MarshalsAsAPlainArray(t *testing.T) {
	schema := ToolSchema{
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]PropertySchema{
				"label": {Type: "string", Enum: &EnumValues{Values: []string{"a", "b"}}},
			},
		},
	}

	b, err := json.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(b); !strings.Contains(got, `"enum":["a","b"]`) {
		t.Errorf("JSON = %s, want a plain array under enum", got)
	}
}

// An absent enum must disappear from the schema: `"enum": null` is not a thing
// any keyword accepts, and it reached MCP clients once already.
func TestEnumValues_AbsentEnumIsOmitted(t *testing.T) {
	b, err := json.Marshal(PropertySchema{Type: "string"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "enum") {
		t.Errorf("JSON = %s, want no enum key at all", b)
	}
}

func TestEnumValues_UnmarshalYAML(t *testing.T) {
	t.Run("inline list", func(t *testing.T) {
		var p PropertySchema
		if err := yaml.Unmarshal([]byte("type: string\nenum: [a, b]\n"), &p); err != nil {
			t.Fatal(err)
		}
		if p.Enum == nil || strings.Join(p.Enum.Values, "|") != "a|b" {
			t.Fatalf("enum = %+v", p.Enum)
		}
		if p.Enum.Include != "" {
			t.Errorf("include = %q, want empty for an inline list", p.Enum.Include)
		}
	})

	// The path has to survive the decode: the file is read later, against the
	// manifest's directory, which this node knows nothing about. Losing it here
	// would leave an empty enum, which constrains nothing — in silence.
	t.Run("include keeps the path", func(t *testing.T) {
		var p PropertySchema
		if err := yaml.Unmarshal([]byte("type: string\nenum: !include ./people.txt\n"), &p); err != nil {
			t.Fatal(err)
		}
		if p.Enum == nil {
			t.Fatal("enum = nil")
		}
		if p.Enum.Include != "./people.txt" {
			t.Errorf("include = %q, want ./people.txt", p.Enum.Include)
		}
		if len(p.Enum.Values) != 0 {
			t.Errorf("values = %q, want none before the file is read", p.Enum.Values)
		}
	})
}

// toCoreProp is what the agent loop and every provider adapter see.
func TestToCoreProp_CarriesEnumsAtEveryDepth(t *testing.T) {
	p := PropertySchema{
		Type: "object",
		Properties: map[string]PropertySchema{
			"label":  {Type: "string", Enum: &EnumValues{Values: []string{"a", "b"}}},
			"places": {Type: "array", Items: &PropertySchema{Type: "string", Enum: &EnumValues{Values: []string{"x"}}}},
			"plain":  {Type: "string"},
		},
	}

	cp := toCoreProp(p)
	if got := cp.Properties["label"].Enum; strings.Join(got, "|") != "a|b" {
		t.Errorf("label enum = %q", got)
	}
	items := cp.Properties["places"].Items
	if items == nil || strings.Join(items.Enum, "|") != "x" {
		t.Errorf("items enum = %+v", items)
	}
	if cp.Properties["plain"].Enum != nil {
		t.Errorf("plain enum = %q, want nil", cp.Properties["plain"].Enum)
	}
}
