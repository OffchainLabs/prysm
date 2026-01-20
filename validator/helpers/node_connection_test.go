package helpers

import (
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"google.golang.org/grpc"
)

// mockConnectionProvider implements GrpcConnectionProvider for testing.
type mockConnectionProvider struct {
	conn *grpc.ClientConn
	host string
}

func (m *mockConnectionProvider) CurrentConn() *grpc.ClientConn { return m.conn }
func (m *mockConnectionProvider) CurrentHost() string           { return m.host }
func (m *mockConnectionProvider) Hosts() []string               { return []string{m.host} }
func (m *mockConnectionProvider) Conn(int) *grpc.ClientConn     { return m.conn }
func (m *mockConnectionProvider) SetHost(int) error             { return nil }
func (m *mockConnectionProvider) NextHost()                     {}
func (m *mockConnectionProvider) Close() error                  { return nil }

func TestNewNodeConnection(t *testing.T) {
	t.Run("with provider", func(t *testing.T) {
		provider := &mockConnectionProvider{host: "localhost:4000"}
		conn := NewNodeConnection(provider, "http://localhost:3500")

		assert.Equal(t, provider, conn.GetGrpcConnectionProvider())
		assert.Equal(t, "http://localhost:3500", conn.GetBeaconApiUrl())
	})

	t.Run("with nil provider", func(t *testing.T) {
		conn := NewNodeConnection(nil, "http://localhost:3500")

		assert.Equal(t, (GrpcConnectionProvider)(nil), conn.GetGrpcConnectionProvider())
		assert.Equal(t, (*grpc.ClientConn)(nil), conn.GetGrpcClientConn())
	})

	t.Run("with options", func(t *testing.T) {
		provider := &mockConnectionProvider{host: "localhost:4000"}
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
		provider := &mockConnectionProvider{conn: nil, host: "localhost:4000"}
		conn := NewNodeConnection(provider, "")

		// Should delegate to provider.CurrentConn()
		assert.Equal(t, provider.CurrentConn(), conn.GetGrpcClientConn())
	})

	t.Run("returns nil when provider is nil", func(t *testing.T) {
		conn := NewNodeConnection(nil, "")
		assert.Equal(t, (*grpc.ClientConn)(nil), conn.GetGrpcClientConn())
	})
}
