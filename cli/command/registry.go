package command

import "strings"

type Registry struct {
	commands map[string]Command
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
