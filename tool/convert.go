package tool

import "github.com/Federicoand98/mani/core"

func ToDefinition(t Tool) core.ToolDefinition {
	schema := t.Schema()

	props := make(map[string]core.ToolProperty, len(schema.InputSchema.Properties))

	for name, p := range schema.InputSchema.Properties {
		props[name] = toCoreProp(p)
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

func toCoreProp(p PropertySchema) core.ToolProperty {
	cp := core.ToolProperty{
		Type: p.Type, Description: p.Description, Required: p.Required, Enum: p.Enum,
	}

	if p.Items != nil {
		it := toCoreProp(*p.Items)
		cp.Items = &it
	}

	if len(p.Properties) > 0 {
		cp.Properties = make(map[string]core.ToolProperty, len(p.Properties))
		for name, prop := range p.Properties {
			cp.Properties[name] = toCoreProp(prop)
		}
	}

	return cp
}
