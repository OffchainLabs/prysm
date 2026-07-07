package helpers

import (
	"context"
	"testing"

	grpcutil "github.com/OffchainLabs/prysm/v7/api/grpc"
	"github.com/OffchainLabs/prysm/v7/api/rest"
	"github.com/OffchainLabs/prysm/v7/config/features"
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
	})

	t.Run("with no providers returns error", func(t *testing.T) {
		conn, err := NewNodeConnection()
		require.ErrorContains(t, "at least one beacon node endpoint must be provided", err)
		require.IsNil(t, conn)
	})

	t.Run("with empty endpoints is no-op", func(t *testing.T) {
		// Empty endpoints should be skipped, resulting in no providers
		conn, err := NewNodeConnection(
			WithGRPC(context.Background(), "", nil),
			WithREST(""),
		)
		require.ErrorContains(t, "at least one beacon node endpoint must be provided", err)
		require.IsNil(t, conn)
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

func TestNodeConnection_ConnectionGeneration(t *testing.T) {
	t.Run("gRPC mode uses grpc provider counter", func(t *testing.T) {
		grpcProvider := &grpcutil.MockGrpcProvider{MockHosts: []string{"localhost:4000"}, ConnCounter: 7}
		conn, err := NewNodeConnection(WithGRPCProvider(grpcProvider))
		require.NoError(t, err)
		assert.Equal(t, uint64(7), conn.ConnectionGeneration())
	})

	t.Run("REST mode uses rest provider counter", func(t *testing.T) {
		reset := features.InitWithReset(&features.Flags{EnableBeaconRESTApi: true})
		defer reset()
		restProvider := &rest.MockRestProvider{MockHosts: []string{"http://localhost:3500"}, ConnCounter: 4}
		conn, err := NewNodeConnection(WithRestProvider(restProvider))
		require.NoError(t, err)
		assert.Equal(t, uint64(4), conn.ConnectionGeneration())
	})

	t.Run("gRPC mode reads grpc counter even when rest provider present", func(t *testing.T) {
		grpcProvider := &grpcutil.MockGrpcProvider{MockHosts: []string{"localhost:4000"}, ConnCounter: 2}
		restProvider := &rest.MockRestProvider{MockHosts: []string{"http://localhost:3500"}, ConnCounter: 9}
		conn, err := NewNodeConnection(WithGRPCProvider(grpcProvider), WithRestProvider(restProvider))
		require.NoError(t, err)
		assert.Equal(t, uint64(2), conn.ConnectionGeneration())
	})

	t.Run("REST mode reads rest counter even when grpc provider present", func(t *testing.T) {
		// Regression: gRPC provider is non-nil (its endpoint flag defaults), but
		// in REST mode the active provider is REST, so its counter must win.
		reset := features.InitWithReset(&features.Flags{EnableBeaconRESTApi: true})
		defer reset()
		grpcProvider := &grpcutil.MockGrpcProvider{MockHosts: []string{"localhost:4000"}, ConnCounter: 2}
		restProvider := &rest.MockRestProvider{MockHosts: []string{"http://localhost:3500"}, ConnCounter: 9}
		conn, err := NewNodeConnection(WithGRPCProvider(grpcProvider), WithRestProvider(restProvider))
		require.NoError(t, err)
		assert.Equal(t, uint64(9), conn.ConnectionGeneration())
	})
}
