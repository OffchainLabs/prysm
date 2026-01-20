package grpc_api

import (
	"sync"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	validatorHelpers "github.com/OffchainLabs/prysm/v7/validator/helpers"
	"google.golang.org/grpc"
)

// mockProvider implements grpcutil.GrpcConnectionProvider for testing.
type mockProvider struct {
	hosts        []string
	currentIndex int
	mu           sync.Mutex
}

func (m *mockProvider) CurrentConn() *grpc.ClientConn { return nil }
func (m *mockProvider) Conn(int) *grpc.ClientConn    { return nil }
func (m *mockProvider) Hosts() []string              { return m.hosts }
func (m *mockProvider) Close() error                 { return nil }

func (m *mockProvider) CurrentHost() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.hosts[m.currentIndex]
}

func (m *mockProvider) SetHost(index int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.currentIndex = index
	return nil
}

func (m *mockProvider) NextHost() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.currentIndex = (m.currentIndex + 1) % len(m.hosts)
}

// testClient is a simple type for testing the generic client manager.
type testClient struct{ id int }

// testManager creates a manager with client creation counting.
func testManager(t *testing.T, provider *mockProvider) (*grpcClientManager[*testClient], *int) {
	conn := validatorHelpers.NewNodeConnection(provider, nil)

	clientCount := new(int)
	newClient := func(grpc.ClientConnInterface) *testClient {
		*clientCount++
		return &testClient{id: *clientCount}
	}

	manager := newGrpcClientManager(conn, newClient)
	require.NotNil(t, manager)
	return manager, clientCount
}

func TestGrpcClientManager(t *testing.T) {
	t.Run("tracks host", func(t *testing.T) {
		provider := &mockProvider{hosts: []string{"host1:4000", "host2:4000"}}
		manager, count := testManager(t, provider)
		assert.Equal(t, 1, *count)
		assert.Equal(t, "host1:4000", manager.lastHost)
	})

	t.Run("same host returns same client", func(t *testing.T) {
		provider := &mockProvider{hosts: []string{"host1:4000", "host2:4000"}}
		manager, count := testManager(t, provider)

		c1, c2, c3 := manager.getClient(), manager.getClient(), manager.getClient()
		assert.Equal(t, 1, *count)
		assert.Equal(t, c1, c2)
		assert.Equal(t, c2, c3)
	})

	t.Run("host change recreates client", func(t *testing.T) {
		provider := &mockProvider{hosts: []string{"host1:4000", "host2:4000"}}
		manager, count := testManager(t, provider)

		c1 := manager.getClient()
		assert.Equal(t, 1, c1.id)

		provider.NextHost()
		c2 := manager.getClient()
		assert.Equal(t, 2, *count)
		assert.Equal(t, 2, c2.id)

		// Same host again - no recreation
		c3 := manager.getClient()
		assert.Equal(t, 2, *count)
		assert.Equal(t, c2, c3)
	})

	t.Run("multiple host switches", func(t *testing.T) {
		provider := &mockProvider{hosts: []string{"host1:4000", "host2:4000", "host3:4000"}}
		manager, count := testManager(t, provider)
		assert.Equal(t, 1, *count)

		for expected := 2; expected <= 4; expected++ {
			provider.NextHost()
			_ = manager.getClient()
			assert.Equal(t, expected, *count)
		}
	})
}

func TestGrpcClientManager_Concurrent(t *testing.T) {
	t.Run("concurrent access same host", func(t *testing.T) {
		provider := &mockProvider{hosts: []string{"host1:4000", "host2:4000"}}
		manager, _ := testManager(t, provider)

		var clientCount int
		var countMu sync.Mutex
		// Override with thread-safe counter
		manager.newClient = func(grpc.ClientConnInterface) *testClient {
			countMu.Lock()
			clientCount++
			id := clientCount
			countMu.Unlock()
			return &testClient{id: id}
		}
		manager.client = manager.newClient(nil)
		clientCount = 1

		var wg sync.WaitGroup
		for range 100 {
			wg.Go(func() { _ = manager.getClient() })
		}
		wg.Wait()

		countMu.Lock()
		assert.Equal(t, 1, clientCount)
		countMu.Unlock()
	})

	t.Run("concurrent with host changes", func(t *testing.T) {
		provider := &mockProvider{hosts: []string{"host1:4000", "host2:4000"}}
		manager, _ := testManager(t, provider)

		var clientCount int
		var countMu sync.Mutex
		manager.newClient = func(grpc.ClientConnInterface) *testClient {
			countMu.Lock()
			clientCount++
			id := clientCount
			countMu.Unlock()
			return &testClient{id: id}
		}
		manager.client = manager.newClient(nil)
		clientCount = 1

		var wg sync.WaitGroup
		for range 50 {
			wg.Go(func() { _ = manager.getClient() })
			wg.Go(func() { provider.NextHost() })
		}
		wg.Wait()

		countMu.Lock()
		assert.NotEqual(t, 0, clientCount, "Should have created at least one client")
		countMu.Unlock()
	})
}
