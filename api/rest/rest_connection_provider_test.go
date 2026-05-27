package rest

import (
	"reflect"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestParseEndpoints(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{"single endpoint", "http://localhost:3500", []string{"http://localhost:3500"}},
		{"multiple endpoints", "http://host1:3500,http://host2:3500,http://host3:3500", []string{"http://host1:3500", "http://host2:3500", "http://host3:3500"}},
		{"endpoints with spaces", "http://host1:3500, http://host2:3500 , http://host3:3500", []string{"http://host1:3500", "http://host2:3500", "http://host3:3500"}},
		{"empty string", "", nil},
		{"only commas", ",,,", []string{}},
		{"trailing comma", "http://host1:3500,http://host2:3500,", []string{"http://host1:3500", "http://host2:3500"}},
		{"leading comma", ",http://host1:3500,http://host2:3500", []string{"http://host1:3500", "http://host2:3500"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseEndpoints(tt.input)
			if !reflect.DeepEqual(tt.expected, got) {
				t.Errorf("parseEndpoints(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestNewRestConnectionProvider_Errors(t *testing.T) {
	t.Run("no endpoints", func(t *testing.T) {
		_, err := NewRestConnectionProvider("")
		require.ErrorContains(t, "no REST API endpoints provided", err)
	})
}

func TestRestConnectionProvider(t *testing.T) {
	provider, err := NewRestConnectionProvider("http://host1:3500,http://host2:3500,http://host3:3500")
	require.NoError(t, err)

	t.Run("initial state", func(t *testing.T) {
		assert.Equal(t, 3, len(provider.Hosts()))
		assert.Equal(t, "http://host1:3500", provider.CurrentHost())
		assert.NotNil(t, provider.HttpClient())
	})

	t.Run("SwitchHost", func(t *testing.T) {
		require.NoError(t, provider.SwitchHost(1))
		assert.Equal(t, "http://host2:3500", provider.CurrentHost())
		require.NoError(t, provider.SwitchHost(0))
		assert.Equal(t, "http://host1:3500", provider.CurrentHost())
		require.ErrorContains(t, "invalid host index", provider.SwitchHost(-1))
		require.ErrorContains(t, "invalid host index", provider.SwitchHost(3))
	})

	t.Run("Hosts returns copy", func(t *testing.T) {
		hosts := provider.Hosts()
		original := hosts[0]
		hosts[0] = "modified"
		assert.Equal(t, original, provider.Hosts()[0])
	})
}

func TestRestConnectionProvider_WithOptions(t *testing.T) {
	headers := map[string][]string{"Authorization": {"Bearer token"}}
	provider, err := NewRestConnectionProvider(
		"http://localhost:3500",
		WithHttpHeaders(headers),
		WithHttpTimeout(30000000000), // 30 seconds in nanoseconds
		WithTracing(),
	)
	require.NoError(t, err)
	assert.NotNil(t, provider.HttpClient())
	assert.Equal(t, "http://localhost:3500", provider.CurrentHost())
}
