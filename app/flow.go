package app

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

// FlowSpec wires manifests into a pipeline: which agents run, on what data, in which order.
// It has no behaviour of its own, every step is a manifest or a command that already run by itself, so every piece stays testable alone.
type FlowSpec struct {
	Flow   string     `yaml:"flow"`
	About  string     `yaml:"about"`
	Input  string     `yaml:"input"`
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
	ForEach string  `yaml:"for_each"`
	FromAll string  `yaml:"from_all"`
	Jobs    int     `yaml:"jobs"`
}

// Command is an argv. Written as tring it is splt on spaces: there is no shell, so no quoting, pipes or glob.
type Command []string

func (s StepSpec) Upstream() string {
	if s.ForEach != "" {
		return s.ForEach
	}
	return s.FromAll
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

// FlowInput is the name a step uses to read the records given with --in
const FlowInput = "input"

var flowName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

func AgentPath(flowDir, agent string) string {
	if filepath.IsAbs(agent) {
		return agent
	}
	return filepath.Join(flowDir, filepath.FromSlash(agent))
}

func (f FlowSpec) Validate() error {
	if !flowName.MatchString(f.Flow) {
		return fmt.Errorf("flow: %q is not a name: lowercase letters, digits and _", f.Flow)
	}

	if strings.TrimSpace(f.About) == "" {
		return errors.New("about: say in one sentence what the flow does")
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
		case st.ForEach != "" && st.FromAll != "":
			return fmt.Errorf("%s: reads with for_each or with from_all, not both", at)
		case st.Agent != "" && st.Upstream() == "":
			return fmt.Errorf("%s: an agent needs something to work on: for_each or from_all", at)
		case st.Jobs < 0:
			return fmt.Errorf("%s: jobs cannot be negative", at)
		case st.Jobs > 1 && (st.Agent == "" || st.ForEach == ""):
			return fmt.Errorf("%s: jobs: only an agent with for_each runs more than one thing at a time", at)
		}

		switch from := st.Upstream(); {
		case from == "":
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

// LoadFlow reads a flow the way LoadManifest reads a manifest — strict keys
// with their true line numbers, ${VAR} in values — and then loads every agent
// it names: a flow that validates has manifests that validate too.
func LoadFlow(path string) (FlowSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return FlowSpec{}, fmt.Errorf("[flow]: read %s: %w", path, err)
	}

	{
		var probe FlowSpec
		dec := yaml.NewDecoder(bytes.NewReader(data))
		dec.KnownFields(true)
		if err := dec.Decode(&probe); err != nil && !errors.Is(err, io.EOF) {
			return FlowSpec{}, fmt.Errorf("[flow]: decode %s: %w", path, err)
		}
	}

	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return FlowSpec{}, fmt.Errorf("[flow]: unmarshal %s: %w", path, err)
	}
	if err := expandEnvNodes(&root); err != nil {
		return FlowSpec{}, fmt.Errorf("[flow]: expand env %s: %w", path, err)
	}

	var f FlowSpec
	if len(root.Content) > 0 {
		if err := root.Decode(&f); err != nil {
			return FlowSpec{}, fmt.Errorf("[flow]: decode %s: %w", path, err)
		}
	}
	if err := f.Validate(); err != nil {
		return FlowSpec{}, fmt.Errorf("[flow]: %s: %w", path, err)
	}

	for _, st := range f.Steps {
		if st.Agent == "" {
			continue
		}
		if _, err := LoadManifest(AgentPath(filepath.Dir(path), st.Agent)); err != nil {
			return FlowSpec{}, fmt.Errorf("[flow]: step %s: %w", st.Step, err)
		}
	}
	return f, nil
}
