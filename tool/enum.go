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

func (e *EnumValues) UnmarshalJSON(b []byte) error {
	return json.Unmarshal(b, &e.Values)
}

func (e *EnumValues) UnmarshalYAML(node *yaml.Node) error {
	// The path is kept, not read: the file is resolved against the manifest's
	// directory, which this node knows nothing about.
	if node.Tag == "!include" {
		e.Include = node.Value
		return nil
	}
	return node.Decode(&e.Values)
}
