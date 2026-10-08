package tool

import (
	"encoding/json"

	"gopkg.in/yaml.v3"
)

type EnumValues struct {
	Values  []string
	Include string
}

func (e EnumValues) MarshalJSON() ([]byte, error) {
	return json.Marshal(e.Values)
}

func (e EnumValues) UnmarshalJSON(b []byte) error {
	return json.Unmarshal(b, &e.Values)
}

func (e *EnumValues) UnmarshalYAML(node *yaml.Node) error {
	if node.Tag == "!include" {
		return nil
	}
	return node.Decode(&e.Values)
}
