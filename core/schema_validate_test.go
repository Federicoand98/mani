package core

import (
	"strings"
	"testing"
)

func TestValidateAgainstSchema(t *testing.T) {
	s := ToolInputSchema{
		Type: "object",
		Properties: map[string]ToolProperty{
			"name":  {Type: "string"},
			"score": {Type: "number"},
			"tag":   {Type: "string", Enum: []string{"a", "b"}},
			"n":     {Type: "integer"},
			"ok":    {Type: "boolean"},
		},
		Required: []string{"name", "score"},
	}

	tests := []struct {
		name    string
		input   map[string]any
		wantErr bool
	}{
		{"valido", map[string]any{"name": "x", "score": 1.5}, false},
		{"manca required", map[string]any{"name": "x"}, true},
		{"tipo string sbagliato", map[string]any{"name": 3, "score": 1.0}, true},
		{"number non numerico", map[string]any{"name": "x", "score": "alto"}, true},
		{"enum ok", map[string]any{"name": "x", "score": 1.0, "tag": "a"}, false},
		{"enum ko", map[string]any{"name": "x", "score": 1.0, "tag": "z"}, true},
		{"integer ok", map[string]any{"name": "x", "score": 1.0, "n": 3.0}, false},
		{"integer non intero", map[string]any{"name": "x", "score": 1.0, "n": 3.5}, true},
		{"boolean ok", map[string]any{"name": "x", "score": 1.0, "ok": true}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateAgainstSchema(tt.input, s); (err != nil) != tt.wantErr {
				t.Errorf("validateAgainstSchema(%v) err=%v, wantErr=%v", tt.input, err, tt.wantErr)
			}
		})
	}
}

// Until this phase the validator only looked at the top level: an array of
// objects, or an object inside an object, passed whatever it contained. That is
// the whole point of a declared schema, so these are the cases that matter.
func TestValidateAgainstSchema_ArrayItems(t *testing.T) {
	s := ToolInputSchema{
		Type: "object",
		Properties: map[string]ToolProperty{
			"tags": {Type: "array", Items: &ToolProperty{Type: "string", Enum: []string{"a", "b"}}},
			"nums": {Type: "array", Items: &ToolProperty{Type: "integer"}},
			"free": {Type: "array"}, // no items: anything goes
		},
	}

	tests := []struct {
		name    string
		input   map[string]any
		wantErr string // "" means valid
	}{
		{"every item valid", map[string]any{"tags": []any{"a", "b"}}, ""},
		{"empty array", map[string]any{"tags": []any{}}, ""},
		{"item of the wrong type", map[string]any{"tags": []any{"a", 3}}, "tags[1] must be a string"},
		{"item outside the enum", map[string]any{"tags": []any{"a", "z"}}, "tags[1] must be one of"},
		{"not an array at all", map[string]any{"tags": "a"}, "tags must be an array"},
		{"integer item not integer", map[string]any{"nums": []any{1.0, 2.5}}, "nums[1] must be an integer"},
		{"array without items is not inspected", map[string]any{"free": []any{"a", 3, true}}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAgainstSchema(tt.input, s)
			switch {
			case tt.wantErr == "" && err != nil:
				t.Errorf("err = %v, want valid", err)
			case tt.wantErr != "" && err == nil:
				t.Errorf("err = nil, want one containing %q", tt.wantErr)
			case tt.wantErr != "" && !strings.Contains(err.Error(), tt.wantErr):
				t.Errorf("err = %q, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidateAgainstSchema_NestedObjects(t *testing.T) {
	person := ToolProperty{
		Type: "object",
		Properties: map[string]ToolProperty{
			"name": {Type: "string"},
			"age":  {Type: "integer"},
			"role": {Type: "string", Enum: []string{"sender", "recipient"}},
		},
		Required: []string{"name"},
	}
	s := ToolInputSchema{
		Type: "object",
		Properties: map[string]ToolProperty{
			"person": person,
			"people": {Type: "array", Items: &person},
		},
		Required: []string{"person"},
	}

	tests := []struct {
		name    string
		input   map[string]any
		wantErr string
	}{
		{"valid", map[string]any{"person": map[string]any{"name": "Isabella", "role": "sender"}}, ""},
		{"nested required missing", map[string]any{"person": map[string]any{"age": 40.0}}, "required field name is missing"},
		{"nested type wrong", map[string]any{"person": map[string]any{"name": "x", "age": "forty"}}, "person.age must be an integer"},
		{"nested enum wrong", map[string]any{"person": map[string]any{"name": "x", "role": "witness"}}, "person.role must be one of"},
		{"not an object", map[string]any{"person": "Isabella"}, "person must be an object"},
		{"object inside an array", map[string]any{
			"person": map[string]any{"name": "x"},
			"people": []any{map[string]any{"name": "a"}, map[string]any{"age": 2.0}},
		}, "people[1]: required field name is missing"},
		{"optional nested field absent", map[string]any{"person": map[string]any{"name": "x"}}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAgainstSchema(tt.input, s)
			switch {
			case tt.wantErr == "" && err != nil:
				t.Errorf("err = %v, want valid", err)
			case tt.wantErr != "" && err == nil:
				t.Errorf("err = nil, want one containing %q", tt.wantErr)
			case tt.wantErr != "" && !strings.Contains(err.Error(), tt.wantErr):
				t.Errorf("err = %q, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

// The error has to say which element: with a hundred letters in one array,
// "must be a string" alone is not a usable message.
func TestValidateAgainstSchema_ErrorNamesThePath(t *testing.T) {
	s := ToolInputSchema{
		Type: "object",
		Properties: map[string]ToolProperty{
			"letters": {Type: "array", Items: &ToolProperty{
				Type: "object",
				Properties: map[string]ToolProperty{
					"places": {Type: "array", Items: &ToolProperty{Type: "string"}},
				},
			}},
		},
	}
	input := map[string]any{"letters": []any{
		map[string]any{"places": []any{"Mantova"}},
		map[string]any{"places": []any{"Ferrara", 7}},
	}}

	err := validateAgainstSchema(input, s)
	if err == nil {
		t.Fatal("err = nil, want the bad element named")
	}
	if !strings.Contains(err.Error(), "letters[1].places[1]") {
		t.Errorf("err = %q, want the full path letters[1].places[1]", err)
	}
}
