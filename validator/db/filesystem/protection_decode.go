package filesystem

import (
	"bytes"
	"fmt"
	"io"

	"gopkg.in/yaml.v3"
)

func decodeSlashingProtection(data []byte) (*ValidatorSlashingProtection, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("invalid protection document: %w", err)
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("protection record must be a mapping")
	}
	root := document.Content[0]
	sourcePresent := false
	seen := make(map[string]bool)
	for i := 0; i < len(root.Content); i += 2 {
		key, value := root.Content[i], root.Content[i+1]
		if key.Kind != yaml.ScalarNode || key.Tag != "!!str" {
			return nil, fmt.Errorf("protection field names must be literal strings")
		}
		switch key.Value {
		case "lastSignedAttestationSourceEpoch", "latestSignedBlockSlot", "lastSignedAttestationTargetEpoch":
			if seen[key.Value] {
				return nil, fmt.Errorf("duplicate protection field %s", key.Value)
			}
			seen[key.Value] = true
			required := key.Value == "lastSignedAttestationSourceEpoch"
			if required {
				sourcePresent = true
			}
			if !required && value.Kind == yaml.ScalarNode && value.Tag == "!!null" {
				continue
			}
			if value.Kind != yaml.ScalarNode || value.Tag != "!!int" {
				return nil, fmt.Errorf("protection field %s must be an unsigned integer", key.Value)
			}
		}
	}
	if !sourcePresent {
		return nil, fmt.Errorf("missing lastSignedAttestationSourceEpoch")
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		if err != nil {
			return nil, fmt.Errorf("invalid trailing protection document: %w", err)
		}
		return nil, fmt.Errorf("multiple protection documents")
	}
	result := &ValidatorSlashingProtection{}
	if err := root.Decode(result); err != nil {
		return nil, fmt.Errorf("invalid protection fields: %w", err)
	}
	return result, nil
}
