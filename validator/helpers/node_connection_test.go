package helpers

import (
	"context"
	"testing"

	grpcutil "github.com/OffchainLabs/prysm/v7/api/grpc"
	"github.com/OffchainLabs/prysm/v7/api/rest"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"google.golang.org/grpc"
)

func TestNewNodeConnection(t *testing.T) {
	t.Run("with both providers", func(t *testing.T) {
		grpcProvider := &grpcutil.MockGrpcProvider{MockHosts: []string{"localhost:4000"}}
		restProvider := &rest.MockRestProvider{MockHosts: []string{"http://localhost:3500"}}
		conn, err := NewNodeConnection(
			WithGRPCProvider(grpcProvider),
			WithRestProvider(restProvider),
		)
		require.NoError(t, err)

		assert.Equal(t, grpcProvider, conn.GetGrpcConnectionProvider())
		assert.Equal(t, restProvider, conn.GetRestConnectionProvider())
	})

	t.Run("with only rest provider", func(t *testing.T) {
		restProvider := &rest.MockRestProvider{MockHosts: []string{"http://localhost:3500"}}
		conn, err := NewNodeConnection(WithRestProvider(restProvider))
		require.NoError(t, err)

		assert.Equal(t, (grpcutil.GrpcConnectionProvider)(nil), conn.GetGrpcConnectionProvider())
		assert.Equal(t, (*grpc.ClientConn)(nil), conn.GetGrpcClientConn())
		assert.Equal(t, restProvider, conn.GetRestConnectionProvider())
	})

	t.Run("with only grpc provider", func(t *testing.T) {
		grpcProvider := &grpcutil.MockGrpcProvider{MockHosts: []string{"localhost:4000"}}
		conn, err := NewNodeConnection(WithGRPCProvider(grpcProvider))
		require.NoError(t, err)

		assert.Equal(t, grpcProvider, conn.GetGrpcConnectionProvider())
		assert.Equal(t, (rest.RestConnectionProvider)(nil), conn.GetRestConnectionProvider())
		assert.Equal(t, (rest.Handler)(nil), conn.GetRestHandler())
	})

	t.Run("with no providers returns error", func(t *testing.T) {
		conn, err := NewNodeConnection()
		require.ErrorContains(t, "at least one beacon node endpoint must be provided", err)
		assert.Equal(t, (NodeConnection)(nil), conn)
	})

	t.Run("with empty endpoints is no-op", func(t *testing.T) {
		// Empty endpoints should be skipped, resulting in no providers
		conn, err := NewNodeConnection(
			WithGRPC(context.Background(), "", nil),
			WithREST(""),
		)
		require.ErrorContains(t, "at least one beacon node endpoint must be provided", err)
		assert.Equal(t, (NodeConnection)(nil), conn)
	})
}

func TestNodeConnection_GetGrpcClientConn(t *testing.T) {
	t.Run("delegates to provider", func(t *testing.T) {
		// We can't easily create a real grpc.ClientConn in tests,
		// but we can verify the delegation works with nil
		grpcProvider := &grpcutil.MockGrpcProvider{MockConn: nil, MockHosts: []string{"localhost:4000"}}
		conn, err := NewNodeConnection(WithGRPCProvider(grpcProvider))
		require.NoError(t, err)

		// Should delegate to provider.CurrentConn()
		assert.Equal(t, grpcProvider.CurrentConn(), conn.GetGrpcClientConn())
	})

	t.Run("returns nil when provider is nil", func(t *testing.T) {
		restProvider := &rest.MockRestProvider{MockHosts: []string{"http://localhost:3500"}}
		conn, err := NewNodeConnection(WithRestProvider(restProvider))
		require.NoError(t, err)
		assert.Equal(t, (*grpc.ClientConn)(nil), conn.GetGrpcClientConn())
	})
}

func TestNodeConnection_GetRestHandler(t *testing.T) {
	t.Run("delegates to provider", func(t *testing.T) {
		mockHandler := &rest.MockHandler{}
		restProvider := &rest.MockRestProvider{MockHandler: mockHandler, MockHosts: []string{"http://localhost:3500"}}
		conn, err := NewNodeConnection(WithRestProvider(restProvider))
		require.NoError(t, err)

		assert.Equal(t, mockHandler, conn.GetRestHandler())
	})

	t.Run("returns nil when provider is nil", func(t *testing.T) {
		grpcProvider := &grpcutil.MockGrpcProvider{MockHosts: []string{"localhost:4000"}}
		conn, err := NewNodeConnection(WithGRPCProvider(grpcProvider))
		require.NoError(t, err)
		assert.Equal(t, (rest.Handler)(nil), conn.GetRestHandler())
	})
}
