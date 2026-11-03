package manifest

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ParamSpec is one declared parameter of a flow.
//
//	params:
//	  busta: "The busta to rebuild"              # required
//	  since: { about: "Earliest year", default: "1490" }
type ParamSpec struct {
	Name       string
	About      string
	Default    string
	HasDefault bool

	// Filled by LoadFlow.
	Value  string
	Origin string // "--set", "default", or "" when there is no value

	line int
}

// Params keeps the parameters in the order they were written: `mani validate`
// lists them that way.
type Params []ParamSpec

func (p *Params) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: params: a map from name to description", n.Line)
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		ps := ParamSpec{Name: k.Value, line: k.Line}
		switch v.Kind {
		case yaml.ScalarNode:
			ps.About = v.Value
		case yaml.MappingNode:
			for j := 0; j+1 < len(v.Content); j += 2 {
				key, val := v.Content[j], v.Content[j+1]
				switch key.Value {
				case "about":
					ps.About = val.Value
				case "default":
					if val.Kind != yaml.ScalarNode {
						return fmt.Errorf("line %d: params.%s.default: a single value", val.Line, ps.Name)
					}
					ps.Default, ps.HasDefault = val.Value, true
				default:
					return fmt.Errorf("line %d: params.%s: unknown key %q (about, default)", key.Line, ps.Name, key.Value)
				}
			}
		default:
			return fmt.Errorf("line %d: params.%s: a description, or {about, default}", v.Line, ps.Name)
		}
		*p = append(*p, ps)
	}
	return nil
}

func (p Params) validate() error {
	seen := map[string]bool{}
	for _, ps := range p {
		switch {
		case !flowName.MatchString(ps.Name):
			return fmt.Errorf("line %d: params: %q is not a name: lowercase letters, digits and _", ps.line, ps.Name)
		case seen[ps.Name]:
			return fmt.Errorf("line %d: params.%s: declared twice", ps.line, ps.Name)
		case strings.TrimSpace(ps.About) == "":
			return fmt.Errorf("line %d: params.%s: say in a few words what the parameter is", ps.line, ps.Name)
		}
		seen[ps.Name] = true
	}
	return nil
}

func (p Params) find(name string) *ParamSpec {
	for i := range p {
		if p[i].Name == name {
			return &p[i]
		}
	}
	return nil
}

func (p Params) names() []string {
	out := make([]string, 0, len(p))
	for _, ps := range p {
		out = append(out, ps.Name)
	}
	return out
}

// resolve gives every parameter its value: --set wins over the default. A
// value for a parameter the flow does not declare is a typo, and an error: it
// would otherwise be silently ignored, which is the failure a declaration
// exists to prevent.
func (p Params) resolve(set map[string]string) error {
	unknown := make([]string, 0)
	for name := range set {
		if p.find(name) == nil {
			unknown = append(unknown, name)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		declared := "none"
		if len(p) > 0 {
			declared = strings.Join(p.names(), ", ")
		}
		return fmt.Errorf("--set %s: the flow declares no such parameter (declared: %s)", unknown[0], declared)
	}
	for i := range p {
		ps := &p[i]
		if v, ok := set[ps.Name]; ok {
			ps.Value, ps.Origin = v, "--set"
		} else if ps.HasDefault {
			ps.Value, ps.Origin = ps.Default, "default"
		}
	}
	return nil
}

// MissingParams lists the declared parameters that have no value: neither
// --set nor a default. A flow with any cannot run.
func (f Flow) MissingParams() []ParamSpec {
	var out []ParamSpec
	for _, ps := range f.Params {
		if ps.Origin == "" {
			out = append(out, ps)
		}
	}
	return out
}

// ParamValues is the value of every parameter, as a run of the flow sees it.
func (f Flow) ParamValues() map[string]string {
	out := make(map[string]string, len(f.Params))
	for _, ps := range f.Params {
		out[ps.Name] = ps.Value
	}
	return out
}

const paramPrefix = "${params."

var (
	// anyParamRef catches every ${params.…}, well formed or not, so that a
	// reference that will never be replaced is an error rather than a literal
	// that reaches a script.
	anyParamRef = regexp.MustCompile(`\$\{params\.([^}]*)\}`)
)

// substituteParams fills ${params.name} in every scalar value of the flow,
// under the same rules as ${VAR}: values only, never keys, never block scalars.
// The params block itself is skipped: a default does not refer to another
// parameter. A parameter that has no value is left as written, for
// MissingParams to report.
func substituteParams(root *yaml.Node, p Params) error {
	if len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		return nil
	}
	top := root.Content[0]
	for i := 0; i+1 < len(top.Content); i += 2 {
		if top.Content[i].Value == "params" {
			continue
		}
		if err := substituteNode(top.Content[i+1], p); err != nil {
			return err
		}
	}
	return nil
}

func substituteNode(n *yaml.Node, p Params) error {
	switch n.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			if err := substituteNode(n.Content[i+1], p); err != nil {
				return err
			}
		}
	case yaml.SequenceNode:
		for _, c := range n.Content {
			if err := substituteNode(c, p); err != nil {
				return err
			}
		}
	case yaml.ScalarNode:
		if n.Style == yaml.LiteralStyle || n.Style == yaml.FoldedStyle {
			return nil
		}
		var bad error
		n.Value = anyParamRef.ReplaceAllStringFunc(n.Value, func(ref string) string {
			name := anyParamRef.FindStringSubmatch(ref)[1]
			ps := p.find(name)
			switch {
			case ps == nil && bad == nil:
				declared := "none"
				if len(p) > 0 {
					declared = strings.Join(p.names(), ", ")
				}
				bad = fmt.Errorf("line %d: %s: the flow declares no parameter %q (declared: %s)", n.Line, ref, name, declared)
				return ref
			case ps == nil, ps.Origin == "":
				return ref
			}
			return ps.Value
		})
		return bad
	}
	return nil
}

// errMissingParams is what running a flow says when a parameter has no value.
func errMissingParams(missing []ParamSpec) error {
	lines := make([]string, 0, len(missing))
	for _, ps := range missing {
		lines = append(lines, fmt.Sprintf("%s (%s)", ps.Name, ps.About))
	}
	return errors.New("missing parameters: " + strings.Join(lines, ", "))
}

// CheckParams is the error a run of the flow must stop on: a parameter with no
// value, named with its description so the fix is in the message.
func (f Flow) CheckParams() error {
	if missing := f.MissingParams(); len(missing) > 0 {
		return fmt.Errorf("%w; pass --set %s=…", errMissingParams(missing), missing[0].Name)
	}
	return nil
}
