package grpc

import (
	"context"
	"net"
	"strings"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestParseEndpoints(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{"single endpoint", "localhost:4000", []string{"localhost:4000"}},
		{"multiple endpoints", "host1:4000,host2:4000,host3:4000", []string{"host1:4000", "host2:4000", "host3:4000"}},
		{"endpoints with spaces", "host1:4000, host2:4000 , host3:4000", []string{"host1:4000", "host2:4000", "host3:4000"}},
		{"empty string", "", nil},
		{"only commas", ",,,", nil},
		{"trailing comma", "host1:4000,host2:4000,", []string{"host1:4000", "host2:4000"}},
		{"leading comma", ",host1:4000,host2:4000", []string{"host1:4000", "host2:4000"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.DeepEqual(t, tt.expected, parseEndpoints(tt.input))
		})
	}
}

func TestNewGrpcConnectionProvider_Errors(t *testing.T) {
	t.Run("no endpoints", func(t *testing.T) {
		dialOpts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
		_, err := NewGrpcConnectionProvider(context.Background(), "", dialOpts)
		require.ErrorContains(t, "no gRPC endpoints provided", err)
	})
}

func TestGrpcConnectionProvider_LazyConnection(t *testing.T) {
	// Start only one server but configure provider with two endpoints
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	server := grpc.NewServer()
	go func() { _ = server.Serve(lis) }()
	defer server.Stop()

	validAddr := lis.Addr().String()
	invalidAddr := "127.0.0.1:1" // Port 1 is unlikely to be listening

	// Provider should succeed even though second endpoint is invalid (lazy connections)
	endpoint := validAddr + "," + invalidAddr
	ctx := context.Background()
	dialOpts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	provider, err := NewGrpcConnectionProvider(ctx, endpoint, dialOpts)
	require.NoError(t, err, "Provider creation should succeed with lazy connections")
	defer func() { _ = provider.Close() }()

	// First endpoint should work
	conn := provider.CurrentConn()
	assert.NotNil(t, conn, "First connection should be created lazily")
}

func TestGrpcConnectionProvider_SingleConnectionModel(t *testing.T) {
	// Create provider with 3 endpoints
	var addrs []string
	var servers []*grpc.Server

	for range 3 {
		lis, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		server := grpc.NewServer()
		go func() { _ = server.Serve(lis) }()
		addrs = append(addrs, lis.Addr().String())
		servers = append(servers, server)
	}
	defer func() {
		for _, s := range servers {
			s.Stop()
		}
	}()

	endpoint := strings.Join(addrs, ",")
	ctx := context.Background()
	dialOpts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	provider, err := NewGrpcConnectionProvider(ctx, endpoint, dialOpts)
	require.NoError(t, err)
	defer func() { _ = provider.Close() }()

	// Access the internal state to verify single connection behavior
	p := provider.(*grpcConnectionProvider)

	// Initially no connection
	p.mu.Lock()
	assert.Equal(t, (*grpc.ClientConn)(nil), p.conn, "Connection should be nil before access")
	p.mu.Unlock()

	// Access connection - should create one
	conn0 := provider.CurrentConn()
	assert.NotNil(t, conn0)

	p.mu.Lock()
	assert.NotNil(t, p.conn, "Connection should be created after CurrentConn()")
	firstConn := p.conn
	p.mu.Unlock()

	// Call CurrentConn again - should return same connection
	conn0Again := provider.CurrentConn()
	assert.Equal(t, conn0, conn0Again, "Should return same connection")

	// Switch to different host - old connection should be closed, new one created lazily
	require.NoError(t, provider.SetHost(1))

	p.mu.Lock()
	assert.Equal(t, (*grpc.ClientConn)(nil), p.conn, "Connection should be nil after SetHost (lazy)")
	p.mu.Unlock()

	// Get new connection
	conn1 := provider.CurrentConn()
	assert.NotNil(t, conn1)
	assert.NotEqual(t, firstConn, conn1, "Should be a different connection after switching hosts")
}

// testProvider creates a provider with n test servers and returns cleanup function.
func testProvider(t *testing.T, n int) (GrpcConnectionProvider, []string, func()) {
	var addrs []string
	var cleanups []func()

	for range n {
		lis, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		server := grpc.NewServer()
		go func() { _ = server.Serve(lis) }()
		addrs = append(addrs, lis.Addr().String())
		cleanups = append(cleanups, server.Stop)
	}

	endpoint := strings.Join(addrs, ",")

	ctx := context.Background()
	dialOpts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	provider, err := NewGrpcConnectionProvider(ctx, endpoint, dialOpts)
	require.NoError(t, err)

	cleanup := func() {
		_ = provider.Close()
		for _, c := range cleanups {
			c()
		}
	}
	return provider, addrs, cleanup
}

func TestGrpcConnectionProvider(t *testing.T) {
	provider, addrs, cleanup := testProvider(t, 3)
	defer cleanup()

	t.Run("initial state", func(t *testing.T) {
		assert.Equal(t, 3, len(provider.Hosts()))
		assert.Equal(t, addrs[0], provider.CurrentHost())
		assert.NotNil(t, provider.CurrentConn())
	})

	t.Run("SetHost", func(t *testing.T) {
		require.NoError(t, provider.SetHost(1))
		assert.Equal(t, addrs[1], provider.CurrentHost())
		assert.NotNil(t, provider.CurrentConn()) // New connection created lazily
		require.NoError(t, provider.SetHost(0))
		assert.Equal(t, addrs[0], provider.CurrentHost())
		require.ErrorContains(t, "invalid host index", provider.SetHost(-1))
		require.ErrorContains(t, "invalid host index", provider.SetHost(3))
	})

	t.Run("SetHost circular", func(t *testing.T) {
		// Test round-robin style switching using SetHost with manual index
		indices := []int{1, 2, 0, 1} // Simulate circular switching
		for i, idx := range indices {
			require.NoError(t, provider.SetHost(idx))
			assert.Equal(t, addrs[idx], provider.CurrentHost(), "iteration %d", i)
		}
	})

	t.Run("Hosts returns copy", func(t *testing.T) {
		hosts := provider.Hosts()
		original := hosts[0]
		hosts[0] = "modified"
		assert.Equal(t, original, provider.Hosts()[0])
	})
}

func TestGrpcConnectionProvider_Close(t *testing.T) {
	provider, _, cleanup := testProvider(t, 1)
	defer cleanup()

	assert.NotNil(t, provider.CurrentConn())
	require.NoError(t, provider.Close())
	assert.Equal(t, (*grpc.ClientConn)(nil), provider.CurrentConn())
	require.NoError(t, provider.Close()) // Double close is safe
}
