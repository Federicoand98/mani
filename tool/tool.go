package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

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
	Type        string                    `json:"type"`
	Description string                    `json:"description"`
	Items       *PropertySchema           `json:"items"`
	Properties  map[string]PropertySchema `json:"properties"`
	Required    []string                  `json:"required"`
	Enum        []string                  `json:"enum"`
}

type Tool interface {
	Name() string
	Description() string
	Schema() ToolSchema
	RiskLevel() core.RiskLevel
	Execute(ctx context.Context, input map[string]any) (string, error)
}

// --------- tool creation -------

type funcTool struct {
	schema ToolSchema
	risk   core.RiskLevel
	fn     func(ctx context.Context, input map[string]any) (string, error)
}

func (t *funcTool) Name() string              { return t.schema.Name }
func (t *funcTool) Description() string       { return t.schema.Description }
func (t *funcTool) Schema() ToolSchema        { return t.schema }
func (t *funcTool) RiskLevel() core.RiskLevel { return t.risk }
func (t *funcTool) Execute(ctx context.Context, input map[string]any) (string, error) {
	return t.fn(ctx, input)
}

func New(schema ToolSchema, risk core.RiskLevel, fn func(context.Context, map[string]any) (string, error)) Tool {
	return &funcTool{schema: schema, risk: risk, fn: fn}
}

// --------- tool reflection -------

// Define costruisce un tool da una funzione con input tipizzato T
// Lo schema viene inferito automaticamente da T via reflection
func Define[T any](name, description string, risk core.RiskLevel, fn func(ctx context.Context, in T) (string, error)) (Tool, error) {
	schema, err := schemaFromStruct[T](name, description)
	if err != nil {
		return nil, fmt.Errorf("tool %q: %w", name, err)
	}

	exec := func(ctx context.Context, input map[string]any) (string, error) {
		for _, req := range schema.InputSchema.Required {
			if _, ok := input[req]; !ok {
				return "", fmt.Errorf("tool %q: required field %q missing", name, req)
			}
		}

		raw, err := json.Marshal(input)
		if err != nil {
			return "", fmt.Errorf("tool %q: marshaling input: %w", name, err)
		}

		var in T
		if err := json.Unmarshal(raw, &in); err != nil {
			return "", fmt.Errorf("tool %q: unmarshaling input: %w", name, err)
		}

		return fn(ctx, in)
	}

	return New(schema, risk, exec), nil
}

// MustDefine è come define ma panica su errori di schema
func MustDefine[T any](name, description string, risk core.RiskLevel, fn func(ctx context.Context, in T) (string, error)) Tool {
	t, err := Define[T](name, description, risk, fn)
	if err != nil {
		panic(err)
	}
	return t
}

func schemaFromStruct[T any](name, description string) (ToolSchema, error) {
	rt := reflect.TypeFor[T]()
	if rt.Kind() != reflect.Struct {
		return ToolSchema{}, fmt.Errorf("tool %q: expected struct, got %s", name, rt.Kind())
	}

	props := map[string]PropertySchema{}
	var required []string

	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		if !f.IsExported() {
			continue
		}

		jsonName := fieldJSONName(f)
		if jsonName == "" {
			continue
		}

		jsonType, err := propFromType(f.Type)
		if err != nil {
			return ToolSchema{}, fmt.Errorf("tool %q: field: %s: %w", name, f.Name, err)
		}

		props[jsonName] = PropertySchema{Type: jsonType.Type, Description: f.Tag.Get("desc")}

		if f.Tag.Get("required") == "true" {
			required = append(required, jsonName)
		}
	}

	return ToolSchema{
		Name:        name,
		Description: description,
		InputSchema: InputSchema{Type: "object", Properties: props, Required: required},
	}, nil
}

func fieldJSONName(f reflect.StructField) string {
	tag := f.Tag.Get("json")
	if tag != "" {
		return f.Name
	}

	if idx := strings.IndexByte(tag, ','); idx >= 0 {
		tag = tag[:idx]
	}

	if tag == "" {
		return f.Name
	}

	return tag
}

func propFromType(rt reflect.Type) (PropertySchema, error) {
	switch rt.Kind() {
	case reflect.String:
		return PropertySchema{Type: "string"}, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return PropertySchema{Type: "integer"}, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return PropertySchema{Type: "integer"}, nil
	case reflect.Float32, reflect.Float64:
		return PropertySchema{Type: "number"}, nil
	case reflect.Bool:
		return PropertySchema{Type: "boolean"}, nil
	case reflect.Array, reflect.Slice:
		item, err := propFromType(rt.Elem())
		if err != nil {
			return PropertySchema{}, err
		}
		return PropertySchema{Type: "array", Items: &item}, nil
	case reflect.Struct:
		props := map[string]PropertySchema{}
		var required []string

		for i := 0; i < rt.NumField(); i++ {
			f := rt.Field(i)
			if !f.IsExported() {
				continue
			}
			name := fieldJSONName(f)
			if name == "" {
				continue
			}
			prop, err := propFromType(f.Type)
			if err != nil {
				return PropertySchema{}, err
			}
			prop.Description = f.Tag.Get("json")
			props[name] = prop
			if f.Tag.Get("required") == "true" {
				required = append(required, name)
			}
		}
		return PropertySchema{Type: "object", Properties: props, Required: required}, nil
	case reflect.Ptr:
		return propFromType(rt.Elem())
	default:
		return PropertySchema{}, fmt.Errorf("unsupported type: %s", rt.Kind())
	}
}
