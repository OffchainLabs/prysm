package kurtosis

import (
	"fmt"
	"os"

	"github.com/OffchainLabs/prysm/v7/build/bazel"
	"github.com/ghodss/yaml"
)

// readYamlConfigAsJson reads a YAML config file at the given path and returns its contents as a JSON string.
func readYamlConfigAsJson(networkConfigPath string) (string, error) {
	realPath, err := bazel.Runfile(networkConfigPath)
	if err != nil {
		return "", fmt.Errorf("failed to find runfile '%s': %w", networkConfigPath, err)
	}

	yamlData, err := os.ReadFile(realPath) // #nosec G304
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	return yamlToJSON(yamlData)
}

// yamlToJSON converts a YAML document to its JSON encoding.
func yamlToJSON(yamlData []byte) (string, error) {
	jsonData, err := yaml.YAMLToJSON(yamlData)
	if err != nil {
		return "", fmt.Errorf("failed to convert yaml to json: %w", err)
	}

	return string(jsonData), nil
}
