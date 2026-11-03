package manifest

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Flow wires manifests into a pipeline: which agents run, on what data, in which order.
// It has no behaviour of its own, every step is a manifest or a command that already run by itself, so every piece stays testable alone.
type Flow struct {
	Flow   string     `yaml:"flow"`
	About  string     `yaml:"about"`
	Input  string     `yaml:"input"`
	Params Params     `yaml:"params"`
	Steps  []StepSpec `yaml:"steps"`
	Result string     `yaml:"result"`
	Limits FlowLimits `yaml:"limits"`
}

type FlowLimits struct {
	Tokens int `yaml:"tokens"`
}

type StepSpec struct {
	Step    string  `yaml:"step"`
	Does    string  `yaml:"does"`
	Agent   string  `yaml:"agent"`
	Run     Command `yaml:"run"`
	ForEach Sources `yaml:"for_each"`
	FromAll Sources `yaml:"from_all"`
	Where   Where   `yaml:"where"`
	Jobs    int     `yaml:"jobs"`
}

// Command is an argv. Written as tring it is splt on spaces: there is no shell, so no quoting, pipes or glob.
type Command []string

// Upstream is the one step this step reads, or "" for a step that reads
// nothing. A fan-in step reads several: see Reads.
func (s StepSpec) Upstream() string {
	if s.ForEach.Set() {
		return s.ForEach.First()
	}
	return s.FromAll.First()
}

// Reads is every step this step reads, in the order written.
func (s StepSpec) Reads() []string {
	if s.ForEach.Set() {
		return s.ForEach.Names
	}
	return s.FromAll.Names
}

// FanIn reports whether the step joins several sources, which is what makes
// its records carry a "from" field.
func (s StepSpec) FanIn() bool {
	return s.FromAll.List
}

func (c *Command) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.ScalarNode:
		*c = strings.Fields(n.Value)
		return nil
	case yaml.SequenceNode:
		var args []string
		if err := n.Decode(&args); err != nil {
			return err
		}

		*c = args
		return nil
	}

	return fmt.Errorf("line %d: run: a command line or a list of arguments", n.Line)
}

// Sources is what a step reads: the name of one step, or, written as a list
// under from_all, several of them.
type Sources struct {
	Names []string
	// List is true when the sources were written as a list. That, and not how
	// many there are, decides whether records carry "from": the shape of the
	// data a script receives follows what the file says.
	List bool
}

// One is a single source, as a step written in Go would declare it.
func One(name string) Sources { return Sources{Names: []string{name}} }

// Set reports whether any source was declared.
func (s Sources) Set() bool { return len(s.Names) > 0 }

// First is the first source, or "" when there is none.
func (s Sources) First() string {
	if len(s.Names) == 0 {
		return ""
	}
	return s.Names[0]
}

func (s *Sources) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.ScalarNode:
		if n.Value != "" {
			s.Names = []string{n.Value}
		}
		return nil
	case yaml.SequenceNode:
		var names []string
		if err := n.Decode(&names); err != nil {
			return fmt.Errorf("line %d: a list of step names: %w", n.Line, err)
		}
		s.Names, s.List = names, true
		return nil
	}
	return fmt.Errorf("line %d: a step name, or a list of step names", n.Line)
}

// FlowInput is the name a step uses to read the records given with --in
const FlowInput = "input"

var flowName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

func AgentPath(flowDir, agent string) string {
	if filepath.IsAbs(agent) {
		return agent
	}
	return filepath.Join(flowDir, filepath.FromSlash(agent))
}

func (f Flow) Validate() error {
	if !flowName.MatchString(f.Flow) {
		return fmt.Errorf("flow: %q is not a name: lowercase letters, digits and _", f.Flow)
	}

	if strings.TrimSpace(f.About) == "" {
		return errors.New("about: say in one sentence what the flow does")
	}

	if err := f.Params.validate(); err != nil {
		return err
	}

	if len(f.Steps) == 0 {
		return errors.New("steps: a flow needs at least one step")
	}

	defined := map[string]bool{}
	for _, step := range f.Steps {
		defined[step.Step] = true
	}

	above := map[string]bool{}
	inputRead := false
	for i, st := range f.Steps {
		at := fmt.Sprintf("steps[%d]", i)
		if st.Step != "" {
			at = "step " + st.Step
		}

		switch {
		case !flowName.MatchString(st.Step):
			return fmt.Errorf("%s: step name %q is not a name: lowercase letters, digits and _", at, st.Step)
		case st.Step == FlowInput:
			return fmt.Errorf("%s: %q is reserved for the records given with --in", at, FlowInput)
		case above[st.Step]:
			return fmt.Errorf("%s: defined twice", at)
		case strings.TrimSpace(st.Does) == "":
			return fmt.Errorf("%s: does: say in one sentence what the step does", at)
		case (st.Agent == "") == (len(st.Run) == 0):
			return fmt.Errorf("%s: needs exactly one of agent or run", at)
		case st.ForEach.Set() && st.FromAll.Set():
			return fmt.Errorf("%s: reads with for_each or with from_all, not both", at)
		case st.ForEach.List || len(st.ForEach.Names) > 1:
			return fmt.Errorf("%s: for_each reads one step; to combine several, use from_all: [a, b]", at)
		case st.FromAll.List && len(st.FromAll.Names) == 0:
			return fmt.Errorf("%s: from_all: the list names no step", at)
		case st.Agent != "" && st.Upstream() == "":
			return fmt.Errorf("%s: an agent needs something to work on: for_each or from_all", at)
		case st.Jobs < 0:
			return fmt.Errorf("%s: jobs cannot be negative", at)
		case st.Jobs > 1 && (st.Agent == "" || !st.ForEach.Set()):
			return fmt.Errorf("%s: jobs: only an agent with for_each runs more than one thing at a time", at)
		}

		if len(st.Where) > 0 {
			switch {
			case st.Upstream() == "":
				return fmt.Errorf("%s: where: the step reads nothing to filter", at)
			case len(st.FromAll.Names) > 1:
				return fmt.Errorf("%s: where filters one source, and this step joins %d; filter each one in its own step", at, len(st.FromAll.Names))
			}
			if err := st.Where.validate(); err != nil {
				return fmt.Errorf("%s: %w", at, err)
			}
		}

		seen := map[string]bool{}
		for _, from := range st.Reads() {
			switch {
			case seen[from]:
				return fmt.Errorf("%s: reads %s twice", at, from)
			case from == FlowInput:
				if f.Input == "" {
					return fmt.Errorf("%s: reads %q, but the flow declares no input", at, FlowInput)
				}
				inputRead = true
			case from == st.Step:
				return fmt.Errorf("%s: reads itself", at)
			case !defined[from]:
				return fmt.Errorf("%s: reads %s, which is not a step", at, from)
			case !above[from]:
				return fmt.Errorf("%s: reads %s, which comes later: a step can only read the steps above it", at, from)
			}
			seen[from] = true
		}
		above[st.Step] = true
	}

	if f.Input != "" && !inputRead {
		return errors.New("input: declared, but no step reads it (for_each: input or from_all: input)")
	}
	if !defined[f.Result] {
		return fmt.Errorf("result: %q is not a step", f.Result)
	}
	if f.Limits.Tokens < 0 {
		return errors.New("limits.tokens cannot be negative")
	}
	return nil
}

// IsFlowFile tells a flow from an agent manifest by what it says, not by its
// name: a flow declares itself with a top-level `flow:` key. An extension
// would be a convention that a rename breaks in silence.
func IsFlowFile(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	var top map[string]yaml.Node
	if err := yaml.Unmarshal(data, &top); err != nil {
		return false, fmt.Errorf("parse %s: %w", path, err)
	}
	_, ok := top["flow"]
	return ok, nil
}

// LoadFlow reads a flow the way Load reads a manifest — strict keys with their
// true line numbers, ${VAR} in values — fills in the parameters, and then
// loads every agent it names: a flow that validates has manifests that
// validate too.
//
// set holds the values given with --set. A declared parameter that has neither
// a value nor a default is not an error here, so that `mani validate` can show
// the flow; it is left as written and reported by MissingParams, and running
// the flow is what refuses it.
func LoadFlow(path string, set map[string]string) (Flow, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Flow{}, fmt.Errorf("[flow]: read %s: %w", path, err)
	}

	{
		var probe Flow
		dec := yaml.NewDecoder(bytes.NewReader(data))
		dec.KnownFields(true)
		if err := dec.Decode(&probe); err != nil && !errors.Is(err, io.EOF) {
			return Flow{}, fmt.Errorf("[flow]: decode %s: %w", path, err)
		}
	}

	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return Flow{}, fmt.Errorf("[flow]: unmarshal %s: %w", path, err)
	}
	if err := expandEnvNodes(&root); err != nil {
		return Flow{}, fmt.Errorf("[flow]: expand env %s: %w", path, err)
	}

	// The parameters are read first: their values have to be known before
	// the rest of the file can be filled in with them.
	var head struct {
		Params Params `yaml:"params"`
	}
	if len(root.Content) > 0 {
		if err := root.Decode(&head); err != nil {
			return Flow{}, fmt.Errorf("[flow]: decode %s: %w", path, err)
		}
	}
	if err := head.Params.validate(); err != nil {
		return Flow{}, fmt.Errorf("[flow]: %s: %w", path, err)
	}
	if err := head.Params.resolve(set); err != nil {
		return Flow{}, fmt.Errorf("[flow]: %s: %w", path, err)
	}
	if err := substituteParams(&root, head.Params); err != nil {
		return Flow{}, fmt.Errorf("[flow]: %s: %w", path, err)
	}

	var f Flow
	if len(root.Content) > 0 {
		if err := root.Decode(&f); err != nil {
			return Flow{}, fmt.Errorf("[flow]: decode %s: %w", path, err)
		}
	}
	f.Params = head.Params
	if err := f.Validate(); err != nil {
		return Flow{}, fmt.Errorf("[flow]: %s: %w", path, err)
	}

	agents := map[string]Spec{}
	for _, st := range f.Steps {
		if st.Agent == "" {
			continue
		}
		if strings.Contains(st.Agent, paramPrefix) {
			return Flow{}, fmt.Errorf("[flow]: %s: step %s: the agent path %q needs a parameter that has no value", path, st.Step, st.Agent)
		}
		spec, err := Load(AgentPath(filepath.Dir(path), st.Agent))
		if err != nil {
			return Flow{}, fmt.Errorf("[flow]: step %s: %w", st.Step, err)
		}
		agents[st.Step] = spec
	}

	if err := f.checkWhere(agents); err != nil {
		return Flow{}, fmt.Errorf("[flow]: %s: %w", path, err)
	}
	return f, nil
}
