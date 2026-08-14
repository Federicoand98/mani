package core

import "testing"

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
