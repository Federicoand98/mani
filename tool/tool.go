package tool

import "context"

type ToolSchema struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema InputSchema `json:"input_schema"`
}

type InputSchema struct {
	Type       string                    `json:"type"`
	Properties map[string]PropertySchema `json:"properties"`
	Required   []string                  `json:"required"`
}

type PropertySchema struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

type Tool interface {
	Name() string
	Description() string
	Schema() ToolSchema
	Execute(ctx context.Context, input map[string]interface{}) (string, error)
}
