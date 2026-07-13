package app

import (
	"testing"

	"gopkg.in/yaml.v3"
)

// Regressione del bug UnmarshalYAML: la lista tools mista (scalare + oggetto) deve decodificare.
func TestToolRef_UnmarshalScalarAndObject(t *testing.T) {
	data := `
tools:
  - read
  - name: fetch
    command: ./x.sh
    schema:
      type: object
`
	spec := DefaultSpec()
	if err := yaml.Unmarshal([]byte(data), &spec); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(spec.Tools) != 2 {
		t.Fatalf("attesi 2 tool, ottenuti %d", len(spec.Tools))
	}
	if spec.Tools[0].Name != "read" || spec.Tools[0].isSubprocess() {
		t.Errorf("tool[0] atteso catalog 'read', ottenuto %+v", spec.Tools[0])
	}
	if spec.Tools[1].Name != "fetch" || !spec.Tools[1].isSubprocess() {
		t.Errorf("tool[1] atteso subprocess 'fetch', ottenuto %+v", spec.Tools[1])
	}
}

func TestValidate_UnknownTool(t *testing.T) {
	s := DefaultSpec()
	s.Tools = []ToolRef{{Name: "nonexistent"}}
	if err := s.Validate(); err == nil {
		t.Fatal("atteso errore per tool sconosciuto")
	}
}

func TestValidate_KnownTools_OK(t *testing.T) {
	s := DefaultSpec()
	s.Tools = []ToolRef{{Name: "read"}, {Name: "bash"}}
	if err := s.Validate(); err != nil {
		t.Fatalf("Validate inatteso su tool noti: %v", err)
	}
}

func TestValidate_BadGuardrailRegex(t *testing.T) {
	s := DefaultSpec()
	s.Guardrails.Deny = []DenyRule{{Tool: "bash", Pattern: "["}} // regex invalida
	if err := s.Validate(); err == nil {
		t.Fatal("atteso errore per regex non valida")
	}
}
