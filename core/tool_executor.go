package core

import "context"

type ToolExecutor interface {
	Name() string
	Execute(ctx context.Context, input map[string]any) (string, error)
}

type ToolEventHandler func(name string, input map[string]any, result string, isError bool)
