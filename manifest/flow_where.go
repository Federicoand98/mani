package manifest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Federicoand98/mani/tool"
)

// Where keeps a record when every listed field of its payload equals the
// value written. Equality, and nothing else: no comparisons, no `or`, no
// expressions. A branch that needs a threshold gets it from the agent upstream
// declaring a band in its schema (`enum: [low, medium, high]`), or from a run:
// step that computes it — judgement stays with the agent, the flow only routes.
type Where []Cond

// Cond is one field and the value it must equal.
type Cond struct {
	Field string
	Value any
	line  int
}

func (w *Where) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: where: a map from field to value", n.Line)
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		if v.Kind != yaml.ScalarNode {
			return fmt.Errorf("line %d: where.%s: a single value; lists, maps and comparisons are not supported", v.Line, k.Value)
		}
		var val any
		if err := v.Decode(&val); err != nil {
			return fmt.Errorf("line %d: where.%s: %w", v.Line, k.Value, err)
		}
		*w = append(*w, Cond{Field: k.Value, Value: val, line: k.Line})
	}
	return nil
}

func (w Where) validate() error {
	seen := map[string]bool{}
	for _, c := range w {
		switch {
		case strings.TrimSpace(c.Field) == "":
			return fmt.Errorf("line %d: where: a field name is empty", c.line)
		case seen[c.Field]:
			return fmt.Errorf("line %d: where.%s: written twice", c.line, c.Field)
		}
		seen[c.Field] = true
	}
	return nil
}

// Matches reports whether a record's payload passes the filter. A payload that
// is not an object has no fields to test, which is an error and not a silent
// "no": a flow that filtered everything out because of a shape mismatch would
// look exactly like a flow with nothing to do.
func (w Where) Matches(payload any) (bool, error) {
	obj, ok := payload.(map[string]any)
	if !ok {
		return false, fmt.Errorf("where needs an object to test, and this payload is %s", describe(payload))
	}
	for _, c := range w {
		got, present := obj[c.Field]
		if !present || !sameJSON(got, c.Value) {
			return false, nil
		}
	}
	return true, nil
}

// sameJSON compares as JSON: a YAML 3 and a JSON 3.0 are the same number.
func sameJSON(a, b any) bool {
	ja, err1 := json.Marshal(a)
	jb, err2 := json.Marshal(b)
	return err1 == nil && err2 == nil && bytes.Equal(ja, jb)
}

func describe(v any) string {
	switch v.(type) {
	case nil:
		return "missing"
	case string:
		return "a string"
	case []any:
		return "a list"
	case float64, int:
		return "a number"
	case bool:
		return "a boolean"
	}
	return fmt.Sprintf("a %T", v)
}

// String reads the filter aloud: "confidence is low and kind is letter".
func (w Where) String() string {
	parts := make([]string, 0, len(w))
	for _, c := range w {
		v := fmt.Sprint(c.Value)
		if _, isString := c.Value.(string); !isString {
			b, _ := json.Marshal(c.Value)
			v = string(b)
		}
		parts = append(parts, c.Field+" is "+v)
	}
	return strings.Join(parts, " and ")
}

// checkWhere tests every filter that reads an agent with an output schema
// against that schema, at load time: a misspelt field or a value outside the
// enum would otherwise filter out every record, and the flow would finish
// "successfully" having done nothing.
func (f Flow) checkWhere(agents map[string]Spec) error {
	for _, st := range f.Steps {
		if len(st.Where) == 0 {
			continue
		}
		from := st.Upstream()
		spec, isAgent := agents[from]
		if !isAgent {
			continue // input or a run: step: its records have no declared shape
		}
		upstream := f.step(from)
		schema := spec.Output.Schema
		if schema.Type == "" {
			return fmt.Errorf("step %s: where: %s is an agent with no output.schema, so its results have no fields to test", st.Step, from)
		}
		for _, c := range st.Where {
			prop, ok := schema.Properties[c.Field]
			if !ok {
				return fmt.Errorf("line %d: step %s: where: %q is not a field of %s's output (fields: %s)",
					c.line, st.Step, c.Field, upstream.Agent, strings.Join(fieldNames(schema), ", "))
			}
			if err := valueFits(c.Value, prop); err != nil {
				return fmt.Errorf("line %d: step %s: where.%s: %w", c.line, st.Step, c.Field, err)
			}
		}
	}
	return nil
}

func (f Flow) step(name string) StepSpec {
	for _, st := range f.Steps {
		if st.Step == name {
			return st
		}
	}
	return StepSpec{}
}

func fieldNames(s tool.InputSchema) []string {
	names := make([]string, 0, len(s.Properties))
	for n := range s.Properties {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func valueFits(v any, p tool.PropertySchema) error {
	if p.Enum != nil && len(p.Enum.Values) > 0 {
		s, ok := v.(string)
		for _, allowed := range p.Enum.Values {
			if ok && s == allowed {
				return nil
			}
		}
		return fmt.Errorf("%v is not one of %s", v, strings.Join(p.Enum.Values, ", "))
	}
	switch p.Type {
	case "string":
		if _, ok := v.(string); !ok {
			return fmt.Errorf("the field is a string, and %v is %s", v, describe(v))
		}
	case "number", "integer":
		switch v.(type) {
		case int, float64:
		default:
			return fmt.Errorf("the field is a number, and %v is %s", v, describe(v))
		}
	case "boolean":
		if _, ok := v.(bool); !ok {
			return fmt.Errorf("the field is a boolean, and %v is %s", v, describe(v))
		}
	case "array", "object":
		return fmt.Errorf("the field is %s %s; where compares single values", article(p.Type), p.Type)
	}
	return nil
}

func article(word string) string {
	if strings.ContainsRune("aeiou", rune(word[0])) {
		return "an"
	}
	return "a"
}
