package core

import "fmt"

func validateAgainstSchema(input map[string]any, s ToolInputSchema) error {
	for _, req := range s.Required {
		if _, ok := input[req]; !ok {
			return fmt.Errorf("required field %s is missing", req)
		}
	}

	for name, prop := range s.Properties {
		v, ok := input[name]
		if !ok {
			continue // field not present, skip validation
		}

		if err := checkType(name, v, prop); err != nil {
			return err
		}
	}
	return nil
}

func checkType(name string, v any, prop ToolProperty) error {
	switch prop.Type {
	case "string":
		s, ok := v.(string)
		if !ok {
			return fmt.Errorf("field %s must be a string", name)
		}
		if len(prop.Enum) > 0 && !contains(prop.Enum, s) {
			return fmt.Errorf("field %s must be one of %v", name, prop.Enum)
		}

	case "number":
		if _, ok := v.(float64); !ok {
			return fmt.Errorf("field %s must be a number", name)
		}

	case "integer":
		n, ok := v.(float64)
		if !ok || n != float64(int64(n)) {
			return fmt.Errorf("field %s must be an integer", name)
		}

	case "boolean":
		if _, ok := v.(bool); !ok {
			return fmt.Errorf("field %s must be a boolean", name)
		}

	case "array":
		if _, ok := v.([]any); !ok {
			return fmt.Errorf("field %s must be an array", name)
		}

	case "object":
		if _, ok := v.(map[string]any); !ok {
			return fmt.Errorf("field %s must be an object", name)
		}
	}

	return nil
}

func contains(slice []string, item string) bool {
	for _, v := range slice {
		if v == item {
			return true
		}
	}
	return false
}
