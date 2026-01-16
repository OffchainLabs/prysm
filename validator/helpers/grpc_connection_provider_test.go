package helpers

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
	tests := []struct {
		name      string
		endpoint  string
		setupCtx  func() context.Context
		wantError string
	}{
		{
			name:      "no endpoints",
			endpoint:  "",
			setupCtx:  context.Background,
			wantError: "no gRPC endpoints provided",
		},
		{
			name:     "connection failure",
			endpoint: "invalid:99999",
			setupCtx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
			wantError: "failed to connect to gRPC endpoint",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dialOpts := []grpc.DialOption{
				grpc.WithTransportCredentials(insecure.NewCredentials()),
				grpc.WithBlock(),
			}
			_, err := NewGrpcConnectionProvider(tt.setupCtx(), tt.endpoint, dialOpts)
			require.ErrorContains(t, tt.wantError, err)
		})
	}
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

	t.Run("Conn bounds checking", func(t *testing.T) {
		assert.NotNil(t, provider.Conn(0))
		assert.NotNil(t, provider.Conn(2))
		assert.Equal(t, (*grpc.ClientConn)(nil), provider.Conn(-1))
		assert.Equal(t, (*grpc.ClientConn)(nil), provider.Conn(3))
	})

	t.Run("SetHost", func(t *testing.T) {
		require.NoError(t, provider.SetHost(1))
		assert.Equal(t, addrs[1], provider.CurrentHost())
		require.NoError(t, provider.SetHost(0))
		assert.Equal(t, addrs[0], provider.CurrentHost())
		require.ErrorContains(t, "invalid host index", provider.SetHost(-1))
		require.ErrorContains(t, "invalid host index", provider.SetHost(3))
	})

	t.Run("NextHost circular", func(t *testing.T) {
		require.NoError(t, provider.SetHost(0)) // Reset to start
		for i, expected := range []string{addrs[1], addrs[2], addrs[0], addrs[1]} {
			provider.NextHost()
			assert.Equal(t, expected, provider.CurrentHost(), "iteration %d", i)
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
	assert.Equal(t, (*grpc.ClientConn)(nil), provider.Conn(0))
	require.NoError(t, provider.Close()) // Double close is safe
}
