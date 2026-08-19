package api

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestRedactEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		want     string
	}{
		{
			name:     "basic auth credentials masked",
			endpoint: "https://eth:fake-token-not-real@bn-lodestar.example.io",
			want:     "https://eth:xxxxx@bn-lodestar.example.io",
		},
		{
			name:     "no credentials unchanged",
			endpoint: "https://bn-lodestar.example.io:3500",
			want:     "https://bn-lodestar.example.io:3500",
		},
		{
			name:     "grpc host:port unchanged",
			endpoint: "localhost:4000",
			want:     "localhost:4000",
		},
		{
			name:     "username only still masked",
			endpoint: "https://eth@bn-lodestar.example.io",
			want:     "https://eth@bn-lodestar.example.io",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, RedactEndpoint(tt.endpoint))
		})
	}
}

func TestRedactEndpointList(t *testing.T) {
	tests := []struct {
		name      string
		endpoints string
		want      string
	}{
		{
			name:      "single endpoint behaves like RedactEndpoint",
			endpoints: "https://eth:fake-token-not-real@bn-lodestar.example.io",
			want:      "https://eth:xxxxx@bn-lodestar.example.io",
		},
		{
			name:      "credentials of every endpoint masked",
			endpoints: "https://eth:secret1@host1.example.io,https://eth:secret2@host2.example.io",
			want:      "https://eth:xxxxx@host1.example.io,https://eth:xxxxx@host2.example.io",
		},
		{
			name:      "surrounding spaces trimmed",
			endpoints: "https://host1.example.io:3500, https://eth:secret@host2.example.io:3500",
			want:      "https://host1.example.io:3500,https://eth:xxxxx@host2.example.io:3500",
		},
		{
			name:      "no credentials unchanged",
			endpoints: "http://host1:3500,http://host2:3500",
			want:      "http://host1:3500,http://host2:3500",
		},
		{
			name:      "already redacted is left alone",
			endpoints: "https://eth:xxxxx@host1.example.io,http://host2:3500",
			want:      "https://eth:xxxxx@host1.example.io,http://host2:3500",
		},
		{
			name:      "empty unchanged",
			endpoints: "",
			want:      "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, RedactEndpointList(tt.endpoints))
			// Redacting twice must be a no-op: callers may receive an
			// already-redacted value.
			require.Equal(t, tt.want, RedactEndpointList(RedactEndpointList(tt.endpoints)))
		})
	}
}

func TestRedactEndpoints(t *testing.T) {
	in := []string{
		"https://eth:secret@host1.example.io",
		"https://host2.example.io:3500",
	}
	want := []string{
		"https://eth:xxxxx@host1.example.io",
		"https://host2.example.io:3500",
	}
	require.DeepEqual(t, want, RedactEndpoints(in))
}
