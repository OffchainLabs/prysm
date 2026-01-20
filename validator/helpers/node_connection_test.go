package helpers

import (
	"net/http"
	"testing"

	grpcutil "github.com/OffchainLabs/prysm/v7/api/grpc"
	"github.com/OffchainLabs/prysm/v7/api/rest"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"google.golang.org/grpc"
)

func TestNewNodeConnection(t *testing.T) {
	t.Run("with both providers", func(t *testing.T) {
		grpcProvider := &grpcutil.MockGrpcProvider{MockHosts: []string{"localhost:4000"}}
		restProvider := &rest.MockRestProvider{MockHosts: []string{"http://localhost:3500"}}
		conn := NewNodeConnection(grpcProvider, restProvider)

		assert.Equal(t, grpcProvider, conn.GetGrpcConnectionProvider())
		assert.Equal(t, restProvider, conn.GetRestConnectionProvider())
	})

	t.Run("with nil grpc provider", func(t *testing.T) {
		restProvider := &rest.MockRestProvider{MockHosts: []string{"http://localhost:3500"}}
		conn := NewNodeConnection(nil, restProvider)

		assert.Equal(t, (grpcutil.GrpcConnectionProvider)(nil), conn.GetGrpcConnectionProvider())
		assert.Equal(t, (*grpc.ClientConn)(nil), conn.GetGrpcClientConn())
		assert.Equal(t, restProvider, conn.GetRestConnectionProvider())
	})

	t.Run("with nil rest provider", func(t *testing.T) {
		grpcProvider := &grpcutil.MockGrpcProvider{MockHosts: []string{"localhost:4000"}}
		conn := NewNodeConnection(grpcProvider, nil)

		assert.Equal(t, grpcProvider, conn.GetGrpcConnectionProvider())
		assert.Equal(t, (rest.RestConnectionProvider)(nil), conn.GetRestConnectionProvider())
		assert.Equal(t, (*http.Client)(nil), conn.GetHttpClient())
	})

	t.Run("with both nil", func(t *testing.T) {
		conn := NewNodeConnection(nil, nil)

		assert.Equal(t, (grpcutil.GrpcConnectionProvider)(nil), conn.GetGrpcConnectionProvider())
		assert.Equal(t, (rest.RestConnectionProvider)(nil), conn.GetRestConnectionProvider())
		assert.Equal(t, (*grpc.ClientConn)(nil), conn.GetGrpcClientConn())
		assert.Equal(t, (*http.Client)(nil), conn.GetHttpClient())
	})
}

func TestNodeConnection_GetGrpcClientConn(t *testing.T) {
	t.Run("delegates to provider", func(t *testing.T) {
		// We can't easily create a real grpc.ClientConn in tests,
		// but we can verify the delegation works with nil
		grpcProvider := &grpcutil.MockGrpcProvider{MockConn: nil, MockHosts: []string{"localhost:4000"}}
		conn := NewNodeConnection(grpcProvider, nil)

		// Should delegate to provider.CurrentConn()
		assert.Equal(t, grpcProvider.CurrentConn(), conn.GetGrpcClientConn())
	})

	t.Run("returns nil when provider is nil", func(t *testing.T) {
		conn := NewNodeConnection(nil, nil)
		assert.Equal(t, (*grpc.ClientConn)(nil), conn.GetGrpcClientConn())
	})
}

func TestNodeConnection_GetHttpClient(t *testing.T) {
	t.Run("delegates to provider", func(t *testing.T) {
		mockClient := &http.Client{}
		restProvider := &rest.MockRestProvider{MockClient: mockClient, MockHosts: []string{"http://localhost:3500"}}
		conn := NewNodeConnection(nil, restProvider)

		assert.Equal(t, mockClient, conn.GetHttpClient())
	})

	t.Run("returns nil when provider is nil", func(t *testing.T) {
		conn := NewNodeConnection(nil, nil)
		assert.Equal(t, (*http.Client)(nil), conn.GetHttpClient())
	})
}
