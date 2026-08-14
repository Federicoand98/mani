package command

import (
	"sort"
	"strings"
)

type Registry struct {
	commands map[string]Command
}

type Info struct {
	Name        string
	Description string
}

func NewRegistry() *Registry {
	return &Registry{commands: make(map[string]Command)}
}

func (r *Registry) Register(cmd Command) {
	r.commands[cmd.Name()] = cmd
}

func (r *Registry) Dispatch(input string) (Result, bool, error) {
	if !strings.HasPrefix(input, "/") {
		return Result{}, false, nil
	}

	parts := strings.Fields(input)
	name := parts[0]
	cmd, ok := r.commands[name]

	if !ok {
		return Result{}, false, nil
	}

	res, err := cmd.Execute(parts[1:])
	return res, true, err
}

func (r *Registry) List() []Info {
	out := make([]Info, 0, len(r.commands))

	for _, c := range r.commands {
		out = append(out, Info{Name: c.Name(), Description: c.Description()})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
