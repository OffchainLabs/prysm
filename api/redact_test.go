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
