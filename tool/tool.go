package tool

import "context"

type ToolSchema struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema InputSchema `json:"input_schema"`
}

type InputSchema struct {
	Type       string                         `json:"type"`
	Properties map[string]InputSchemaProperty `json:"properties"`
	Required   []string                       `json:"required"`
}

type InputSchemaProperty struct {
	Path InputScehmaPropertyPath `json:"path"`
}

type InputScehmaPropertyPath struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

type Tool interface {
	Name() string
	Description() string
	Schema() string
	Execute(ctx context.Context, input map[string]interface{}) (string, error)
}
