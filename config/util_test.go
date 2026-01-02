package config

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/sirupsen/logrus/hooks/test"
)

func TestUnmarshalFromURL_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte(`{"key":"value"}`))
		require.NoError(t, err)
	}))
	defer server.Close()

	var result map[string]string
	err := UnmarshalFromURL(t.Context(), server.URL, &result)
	if err != nil {
		t.Errorf("UnmarshalFromURL failed: %v", err)
	}
	if result["key"] != "value" {
		t.Errorf("Expected value to be 'value', got '%s'", result["key"])
	}
}

func TestUnmarshalFromFile_Success(t *testing.T) {
	// Temporarily create a YAML file
	tmpFile, err := os.CreateTemp(t.TempDir(), "example.*.yaml")
	require.NoError(t, err)
	defer require.NoError(t, os.Remove(tmpFile.Name())) // Clean up

	content := []byte("key: value")

	require.NoError(t, os.WriteFile(tmpFile.Name(), content, params.BeaconIoConfig().ReadWritePermissions))
	require.NoError(t, tmpFile.Close())

	var result map[string]string
	require.NoError(t, UnmarshalFromFile(tmpFile.Name(), &result))
	require.Equal(t, result["key"], "value")
}

func TestWarnNonChecksummedAddress(t *testing.T) {
	logHook := test.NewGlobal()
	address := "0x967646dCD8d34F4E02204faeDcbAe0cC96fB9245"
	err := WarnNonChecksummedAddress(address)
	require.NoError(t, err)
	assert.LogsDoNotContain(t, logHook, "is not a checksum Ethereum address")
	address = strings.ToLower("0x967646dCD8d34F4E02204faeDcbAe0cC96fB9244")
	err = WarnNonChecksummedAddress(address)
	require.NoError(t, err)
	assert.LogsContain(t, logHook, "is not a checksum Ethereum address")
}

func TestYamlUnmarshalViaJSON_NestedMaps(t *testing.T) {
	yamlContent := []byte(`
outer:
  inner:
    key: value
    number: 42
`)
	type nested struct {
		Outer struct {
			Inner struct {
				Key    string `json:"key"`
				Number int    `json:"number"`
			} `json:"inner"`
		} `json:"outer"`
	}
	var result nested
	require.NoError(t, yamlUnmarshalViaJSON(yamlContent, &result))
	require.Equal(t, "value", result.Outer.Inner.Key)
	require.Equal(t, 42, result.Outer.Inner.Number)
}

func TestYamlUnmarshalViaJSON_ArraysWithMaps(t *testing.T) {
	yamlContent := []byte(`
items:
  - name: first
    value: 1
  - name: second
    value: 2
`)
	type item struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}
	type config struct {
		Items []item `json:"items"`
	}
	var result config
	require.NoError(t, yamlUnmarshalViaJSON(yamlContent, &result))
	require.Equal(t, 2, len(result.Items))
	require.Equal(t, "first", result.Items[0].Name)
	require.Equal(t, 1, result.Items[0].Value)
	require.Equal(t, "second", result.Items[1].Name)
	require.Equal(t, 2, result.Items[1].Value)
}

func TestYamlUnmarshalViaJSON_JSONStructTags(t *testing.T) {
	// Test that json struct tags work correctly for YAML files
	// This is critical since proto-generated structs only have json tags
	yamlContent := []byte(`
proposer_config:
  "0x123abc":
    fee_recipient: "0xabc"
default_config:
  fee_recipient: "0xdef"
`)
	type feeRecipientConfig struct {
		FeeRecipient string `json:"fee_recipient"`
	}
	type proposerSettings struct {
		ProposerConfig map[string]*feeRecipientConfig `json:"proposer_config"`
		DefaultConfig  *feeRecipientConfig            `json:"default_config"`
	}
	var result proposerSettings
	require.NoError(t, yamlUnmarshalViaJSON(yamlContent, &result))
	require.NotNil(t, result.DefaultConfig)
	require.Equal(t, "0xdef", result.DefaultConfig.FeeRecipient)
	require.NotNil(t, result.ProposerConfig["0x123abc"])
	require.Equal(t, "0xabc", result.ProposerConfig["0x123abc"].FeeRecipient)
}

func TestConvertMapKeys(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected any
	}{
		{
			name:     "nil value",
			input:    nil,
			expected: nil,
		},
		{
			name:     "string value",
			input:    "hello",
			expected: "hello",
		},
		{
			name:     "integer value",
			input:    42,
			expected: 42,
		},
		{
			name: "map[string]any",
			input: map[string]any{
				"key": "value",
			},
			expected: map[string]any{
				"key": "value",
			},
		},
		{
			name: "map[interface{}]interface{}",
			input: map[interface{}]interface{}{
				"key": "value",
				123:   "numeric key",
			},
			expected: map[string]any{
				"key": "value",
				"123": "numeric key",
			},
		},
		{
			name: "nested maps",
			input: map[string]any{
				"outer": map[string]any{
					"inner": "value",
				},
			},
			expected: map[string]any{
				"outer": map[string]any{
					"inner": "value",
				},
			},
		},
		{
			name: "array with maps",
			input: []any{
				map[string]any{"name": "first"},
				map[string]any{"name": "second"},
			},
			expected: []any{
				map[string]any{"name": "first"},
				map[string]any{"name": "second"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertMapKeys(tt.input)
			require.DeepEqual(t, tt.expected, result)
		})
	}
}

func TestUnmarshalFromFile_JSONvsYAML(t *testing.T) {
	type config struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	// Test JSON file
	jsonFile, err := os.CreateTemp(t.TempDir(), "test.*.json")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(jsonFile.Name(), []byte(`{"name":"test","value":123}`), 0600))
	require.NoError(t, jsonFile.Close())

	var jsonResult config
	require.NoError(t, UnmarshalFromFile(jsonFile.Name(), &jsonResult))
	require.Equal(t, "test", jsonResult.Name)
	require.Equal(t, 123, jsonResult.Value)

	// Test YAML file with same content
	yamlFile, err := os.CreateTemp(t.TempDir(), "test.*.yaml")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(yamlFile.Name(), []byte("name: test\nvalue: 123"), 0600))
	require.NoError(t, yamlFile.Close())

	var yamlResult config
	require.NoError(t, UnmarshalFromFile(yamlFile.Name(), &yamlResult))
	require.Equal(t, "test", yamlResult.Name)
	require.Equal(t, 123, yamlResult.Value)

	// Both should produce identical results
	require.DeepEqual(t, jsonResult, yamlResult)
}
