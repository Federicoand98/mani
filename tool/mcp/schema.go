package mcp

import (
	"encoding/json"
	"fmt"

	"github.com/Federicoand98/mani/tool"
)

// rawSchema: sottoinsieme di JSON Schema che ci serve. Passiamo per JSON per
// disaccoppiarci dal tipo concreto del SDK (Tool.InputSchema è `any`: può essere
// un *jsonschema.Schema, una map, ecc. — a noi basta il JSON).
type rawSchema struct {
	Type        string               `json:"type"`
	Description string               `json:"description"`
	Properties  map[string]rawSchema `json:"properties"`
	Items       *rawSchema           `json:"items"`
	Required    []string             `json:"required"`
	Enum        []any                `json:"enum"` // gli enum JSON Schema non sono per forza stringhe
}

// convertSchema converte l'inputSchema di un tool MCP nello schema esteso di mani,
// passando per JSON. Su qualunque errore degrada a un object vuoto (tool senza params).
func convertSchema(in any) tool.InputSchema {
	if in == nil {
		return tool.InputSchema{Type: "object"}
	}
	data, err := json.Marshal(in)
	if err != nil {
		return tool.InputSchema{Type: "object"}
	}
	var rs rawSchema
	if err := json.Unmarshal(data, &rs); err != nil {
		return tool.InputSchema{Type: "object"}
	}

	props := map[string]tool.PropertySchema{}
	for name, p := range rs.Properties {
		props[name] = toProp(p)
	}
	typ := rs.Type
	if typ == "" {
		typ = "object"
	}
	return tool.InputSchema{Type: typ, Properties: props, Required: rs.Required}
}

func toProp(rs rawSchema) tool.PropertySchema {
	p := tool.PropertySchema{
		Type:        rs.Type,
		Description: rs.Description,
		Required:    rs.Required,
		Enum:        enumStrings(rs.Enum),
	}
	if rs.Items != nil {
		it := toProp(*rs.Items)
		p.Items = &it
	}
	if len(rs.Properties) > 0 {
		p.Properties = map[string]tool.PropertySchema{}
		for k, v := range rs.Properties {
			p.Properties[k] = toProp(v)
		}
	}
	return p
}

func enumStrings(vs []any) []string {
	if len(vs) == 0 {
		return nil
	}
	out := make([]string, len(vs))
	for i, v := range vs {
		out[i] = fmt.Sprint(v)
	}
	return out
}
