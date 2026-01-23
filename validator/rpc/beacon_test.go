package rpc

import (
	"strings"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"google.golang.org/grpc/metadata"
)

func TestGrpcHeaders(t *testing.T) {
	s := &Server{
		ctx:         t.Context(),
		grpcHeaders: []string{"first=value1", "second=value2"},
	}
	err := s.registerBeaconClient()
	require.NoError(t, err)
	md, _ := metadata.FromOutgoingContext(s.ctx)
	require.Equal(t, 2, md.Len(), "MetadataV0 contains wrong number of values")
	assert.Equal(t, "value1", md.Get("first")[0])
	assert.Equal(t, "value2", md.Get("second")[0])
}

func TestRegisterBeaconClient_CommaSeparatedEndpoints(t *testing.T) {
	tests := []struct {
		name              string
		beaconApiEndpoint string
		wantError         bool
		errorContains     string
		expectedFirstHost string
	}{
		{
			name:              "single endpoint",
			beaconApiEndpoint: "http://localhost:5052",
			wantError:         false,
			expectedFirstHost: "http://localhost:5052",
		},
		{
			name:              "comma-separated endpoints",
			beaconApiEndpoint: "http://node1:5052,http://node2:5052",
			wantError:         false,
			expectedFirstHost: "http://node1:5052",
		},
		{
			name:              "comma-separated endpoints with spaces",
			beaconApiEndpoint: "http://node1:5052 , http://node2:5052",
			wantError:         false,
			expectedFirstHost: "http://node1:5052",
		},
		{
			name:              "empty endpoint",
			beaconApiEndpoint: "",
			wantError:         true,
			errorContains:     "no beacon API hosts provided",
		},
		{
			name:              "only spaces",
			beaconApiEndpoint: "   ",
			wantError:         true,
			errorContains:     "no beacon API hosts provided",
		},
		{
			name:              "multiple endpoints with empty ones",
			beaconApiEndpoint: "http://node1:5052,,http://node2:5052",
			wantError:         false,
			expectedFirstHost: "http://node1:5052",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trimmed := strings.ReplaceAll(tt.beaconApiEndpoint, " ", "")
			rawHosts := strings.Split(trimmed, ",")
			var hosts []string
			for _, h := range rawHosts {
				if h != "" {
					hosts = append(hosts, h)
				}
			}

			if tt.wantError {
				require.Equal(t, 0, len(hosts), "Should have no valid hosts for error cases")
			} else {
				require.NotEmpty(t, hosts, "Should have at least one valid host")
				// Verify the first host is correctly parsed
				assert.Equal(t, tt.expectedFirstHost, hosts[0])
			}
		})
	}
}
