package tool

import "github.com/Federicoand98/mani/core"

func ToDefinition(t Tool) core.ToolDefinition {
	schema := t.Schema()

	props := make(map[string]core.ToolProperty, len(schema.InputSchema.Properties))

	for name, p := range schema.InputSchema.Properties {
		props[name] = core.ToolProperty{
			Type:        p.Type,
			Description: p.Description,
		}
	}

	return core.ToolDefinition{
		Name:        t.Name(),
		Description: t.Description(),
		RiskLevel:   t.RiskLevel(),
		InputSchema: core.ToolInputSchema{
			Type:       schema.InputSchema.Type,
			Properties: props,
			Required:   schema.InputSchema.Required,
		},
	}
}
