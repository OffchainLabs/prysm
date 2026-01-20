package helpers

import (
	"testing"
	"time"

	grpcutil "github.com/OffchainLabs/prysm/v7/api/grpc"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"google.golang.org/grpc"
)

func TestNewNodeConnection(t *testing.T) {
	t.Run("with provider", func(t *testing.T) {
		provider := &grpcutil.MockGrpcProvider{MockHosts: []string{"localhost:4000"}}
		conn := NewNodeConnection(provider, "http://localhost:3500")

		assert.Equal(t, provider, conn.GetGrpcConnectionProvider())
		assert.Equal(t, "http://localhost:3500", conn.GetBeaconApiUrl())
	})

	t.Run("with nil provider", func(t *testing.T) {
		conn := NewNodeConnection(nil, "http://localhost:3500")

		assert.Equal(t, (grpcutil.GrpcConnectionProvider)(nil), conn.GetGrpcConnectionProvider())
		assert.Equal(t, (*grpc.ClientConn)(nil), conn.GetGrpcClientConn())
	})

	t.Run("with options", func(t *testing.T) {
		provider := &grpcutil.MockGrpcProvider{MockHosts: []string{"localhost:4000"}}
		headers := map[string][]string{"Authorization": {"Bearer token"}}
		timeout := 30 * time.Second

		conn := NewNodeConnection(
			provider,
			"http://localhost:3500",
			WithBeaconApiHeaders(headers),
			WithBeaconApiTimeout(timeout),
		)

		assert.DeepEqual(t, headers, conn.GetBeaconApiHeaders())
		assert.Equal(t, timeout, conn.GetBeaconApiTimeout())
	})
}

func TestNodeConnection_GetGrpcClientConn(t *testing.T) {
	t.Run("delegates to provider", func(t *testing.T) {
		// We can't easily create a real grpc.ClientConn in tests,
		// but we can verify the delegation works with nil
		provider := &grpcutil.MockGrpcProvider{MockConn: nil, MockHosts: []string{"localhost:4000"}}
		conn := NewNodeConnection(provider, "")

		// Should delegate to provider.CurrentConn()
		assert.Equal(t, provider.CurrentConn(), conn.GetGrpcClientConn())
	})

	t.Run("returns nil when provider is nil", func(t *testing.T) {
		conn := NewNodeConnection(nil, "")
		assert.Equal(t, (*grpc.ClientConn)(nil), conn.GetGrpcClientConn())
	})
}

func TestNodeConnection_GetBeaconApiHosts(t *testing.T) {
	t.Run("single host", func(t *testing.T) {
		conn := NewNodeConnection(nil, "http://localhost:3500")
		assert.DeepEqual(t, []string{"http://localhost:3500"}, conn.GetBeaconApiHosts())
	})

	t.Run("multiple hosts", func(t *testing.T) {
		conn := NewNodeConnection(nil, "http://localhost:3500,http://localhost:3501,http://localhost:3502")
		assert.DeepEqual(t, []string{"http://localhost:3500", "http://localhost:3501", "http://localhost:3502"}, conn.GetBeaconApiHosts())
	})

	t.Run("with spaces", func(t *testing.T) {
		conn := NewNodeConnection(nil, "http://localhost:3500, http://localhost:3501")
		// Spaces should be removed
		assert.DeepEqual(t, []string{"http://localhost:3500", "http://localhost:3501"}, conn.GetBeaconApiHosts())
	})

	t.Run("empty url", func(t *testing.T) {
		conn := NewNodeConnection(nil, "")
		assert.DeepEqual(t, []string(nil), conn.GetBeaconApiHosts())
	})
}
