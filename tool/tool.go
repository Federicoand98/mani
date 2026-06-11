package tool

import (
	"context"

	"github.com/Federicoand98/mani/core"
)

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
	RiskLevel() core.RiskLevel
	Execute(ctx context.Context, input map[string]any) (string, error)
}
