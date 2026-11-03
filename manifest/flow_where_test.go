package manifest

import (
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func decodeWhere(t *testing.T, src string) (Where, error) {
	t.Helper()
	var st StepSpec
	err := yaml.Unmarshal([]byte(src), &st)
	return st.Where, err
}

func TestWhere_DecodesTypedScalars(t *testing.T) {
	w, err := decodeWhere(t, "where: { confidence: low, score: 3, reviewed: true }\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(w) != 3 || w[0].Field != "confidence" || w[1].Field != "score" || w[2].Field != "reviewed" {
		t.Fatalf("where = %+v, want three conditions in the order written", w)
	}
	if _, ok := w[0].Value.(string); !ok {
		t.Errorf("low decoded as %T, want a string", w[0].Value)
	}
	if _, ok := w[2].Value.(bool); !ok {
		t.Errorf("true decoded as %T, want a boolean", w[2].Value)
	}
}

// A list or a map as a value would be the first step of an expression
// language. It is refused at decode, with its line.
func TestWhere_RefusesAnythingButSingleValues(t *testing.T) {
	for _, src := range []string{
		"where:\n  confidence: [low, medium]\n",
		"where:\n  confidence: { gt: 3 }\n",
	} {
		_, err := decodeWhere(t, src)
		if err == nil || !strings.Contains(err.Error(), "line 2") || !strings.Contains(err.Error(), "single value") {
			t.Errorf("%q: err = %v, want a single-value error on line 2", src, err)
		}
	}
	if _, err := decodeWhere(t, "where: low\n"); err == nil {
		t.Error("a bare scalar must be refused: where is a map from field to value")
	}
}

func TestWhere_Matches(t *testing.T) {
	w := Where{{Field: "confidence", Value: "low"}, {Field: "score", Value: 3}}
	cases := []struct {
		name    string
		payload any
		want    bool
		wantErr bool
	}{
		{"all equal", map[string]any{"confidence": "low", "score": 3.0, "other": "x"}, true, false},
		// YAML wrote 3, JSON decoded 3.0: the same number.
		{"number across encodings", map[string]any{"confidence": "low", "score": float64(3)}, true, false},
		{"one field differs", map[string]any{"confidence": "high", "score": 3.0}, false, false},
		{"a field is missing", map[string]any{"confidence": "low"}, false, false},
		{"the string 3 is not the number 3", map[string]any{"confidence": "low", "score": "3"}, false, false},
		{"a string payload", "the text of a letter", false, true},
		{"no payload", nil, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := w.Matches(tc.payload)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("Matches = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestWhere_ReadsAloud(t *testing.T) {
	w := Where{{Field: "confidence", Value: "low"}, {Field: "score", Value: 3}, {Field: "ok", Value: true}}
	if got := w.String(); got != "confidence is low and score is 3 and ok is true" {
		t.Errorf("String = %q", got)
	}
}

func TestFlowValidate_Where(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(f *Flow)
		want   string
	}{
		{"on a step that reads nothing", func(f *Flow) {
			f.Steps[0].Where = Where{{Field: "x", Value: "y"}}
		}, "step fetch: where: the step reads nothing"},
		{"on a fan-in", func(f *Flow) {
			f.Steps[2].FromAll = Sources{Names: []string{"fetch", "extract"}, List: true}
			f.Steps[2].Where = Where{{Field: "x", Value: "y"}}
		}, "step timeline: where filters one source, and this step joins 2"},
		{"a field written twice", func(f *Flow) {
			f.Steps[1].Where = Where{{Field: "x", Value: "a"}, {Field: "x", Value: "b"}}
		}, "where.x: written twice"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := validFlow()
			tc.mutate(&f)
			err := f.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Validate = %v, want %q", err, tc.want)
			}
		})
	}

	f := validFlow()
	f.Steps[1].Where = Where{{Field: "kind", Value: "letter"}}
	if err := f.Validate(); err != nil {
		t.Errorf("a where on a for_each step must be valid: %v", err)
	}
}

const labelAgent = `
identity:
  name: labeller
  provider: ollama
  model: test-model
  prompt: "label"
output:
  schema:
    type: object
    properties:
      confidence: { type: string, enum: [low, medium, high] }
      score: { type: number }
      tags: { type: array, items: { type: string } }
    required: [confidence]
`

const textAgent = `
identity:
  name: talker
  provider: ollama
  model: test-model
  prompt: "talk"
`

func whereFlow(where string) string {
	return `
flow: review
about: "labels letters and reviews the doubtful ones"
input: "letters"
steps:
  - step: label
    does: "labels one letter"
    agent: label.yaml
    for_each: input
  - step: review
    does: "a second reading of the doubtful ones"
    agent: label.yaml
    for_each: label
    where: ` + where + `
result: review
`
}

// The upstream agent's schema is the contract a where is checked against at
// load time: a misspelt field or a value outside the enum would otherwise
// filter out every record, and the flow would succeed having done nothing.
func TestLoadFlow_WhereIsCheckedAgainstTheUpstreamSchema(t *testing.T) {
	cases := []struct {
		name, where, want string
	}{
		{"a field the agent does not return", "{ confidance: low }", `"confidance" is not a field of label.yaml's output (fields: confidence, score, tags)`},
		{"a value outside the enum", "{ confidence: lw }", "lw is not one of low, medium, high"},
		{"a string for a number", "{ score: high }", "the field is a number"},
		{"a list field", "{ tags: x }", "where compares single values"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := writeFlowDir(t, map[string]string{"f.yaml": whereFlow(tc.where), "label.yaml": labelAgent})
			_, err := LoadFlow(filepath.Join(dir, "f.yaml"), nil)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
			if !strings.Contains(err.Error(), "line 14") {
				t.Errorf("err = %v, want the line of the where", err)
			}
		})
	}

	dir := writeFlowDir(t, map[string]string{"f.yaml": whereFlow("{ confidence: low, score: 3 }"), "label.yaml": labelAgent})
	if _, err := LoadFlow(filepath.Join(dir, "f.yaml"), nil); err != nil {
		t.Errorf("a where that fits the schema must load: %v", err)
	}
}

func TestLoadFlow_WhereOnAnAgentWithoutSchemaIsAnError(t *testing.T) {
	body := strings.Replace(whereFlow("{ confidence: low }"), "agent: label.yaml\n    for_each: input", "agent: text.yaml\n    for_each: input", 1)
	dir := writeFlowDir(t, map[string]string{"f.yaml": body, "label.yaml": labelAgent, "text.yaml": textAgent})
	_, err := LoadFlow(filepath.Join(dir, "f.yaml"), nil)
	if err == nil || !strings.Contains(err.Error(), "no output.schema") {
		t.Errorf("err = %v, want the missing schema named", err)
	}
}

// Records from --in or from a script have no declared shape: nothing to check
// against, so any field is accepted at load and tested at run time.
func TestLoadFlow_WhereOnAScriptOrTheInputIsNotChecked(t *testing.T) {
	dir := writeFlowDir(t, map[string]string{"f.yaml": `
flow: review
about: "x"
input: "letters"
steps:
  - step: review
    does: "reads the doubtful ones"
    agent: label.yaml
    for_each: input
    where: { anything: goes }
result: review
`, "label.yaml": labelAgent})
	if _, err := LoadFlow(filepath.Join(dir, "f.yaml"), nil); err != nil {
		t.Errorf("err = %v, want no static check on input records", err)
	}
}
