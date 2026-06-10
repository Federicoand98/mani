package core

import "context"

type ToolExecutor interface {
	Name() string
	Execute(ctx context.Context, input map[string]interface{}) (string, error)
}
