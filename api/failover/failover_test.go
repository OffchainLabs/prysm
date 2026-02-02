package failover

import (
	"context"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/assert"
)

// mockHostProvider is a minimal HostProvider for unit tests.
type mockHostProvider struct {
	hosts     []string
	hostIndex int
}

func (m *mockHostProvider) Hosts() []string { return m.hosts }
func (m *mockHostProvider) CurrentHost() string {
	return m.hosts[m.hostIndex%len(m.hosts)]
}
func (m *mockHostProvider) SwitchHost(index int) error { m.hostIndex = index; return nil }

// mockReadyChecker records per-call IsReady results in sequence.
type mockReadyChecker struct {
	results []bool
	idx     int
}

func (m *mockReadyChecker) IsReady(_ context.Context) bool {
	if m.idx >= len(m.results) {
		return false
	}
	r := m.results[m.idx]
	m.idx++
	return r
}

func TestEnsureReady_SingleHostReady(t *testing.T) {
	provider := &mockHostProvider{hosts: []string{"http://host1:3500"}, hostIndex: 0}
	checker := &mockReadyChecker{results: []bool{true}}
	assert.Equal(t, true, EnsureReady(t.Context(), provider, checker))
	assert.Equal(t, 0, provider.hostIndex)
}

func TestEnsureReady_SingleHostNotReady(t *testing.T) {
	provider := &mockHostProvider{hosts: []string{"http://host1:3500"}, hostIndex: 0}
	checker := &mockReadyChecker{results: []bool{false}}
	assert.Equal(t, false, EnsureReady(t.Context(), provider, checker))
}

func TestEnsureReady_SingleHostError(t *testing.T) {
	provider := &mockHostProvider{hosts: []string{"http://host1:3500"}, hostIndex: 0}
	checker := &mockReadyChecker{results: []bool{false}}
	assert.Equal(t, false, EnsureReady(t.Context(), provider, checker))
}

func TestEnsureReady_MultipleHostsFirstReady(t *testing.T) {
	provider := &mockHostProvider{
		hosts:     []string{"http://host1:3500", "http://host2:3500"},
		hostIndex: 0,
	}
	checker := &mockReadyChecker{results: []bool{true}}
	assert.Equal(t, true, EnsureReady(t.Context(), provider, checker))
	assert.Equal(t, 0, provider.hostIndex)
}

func TestEnsureReady_MultipleHostsFailoverToSecond(t *testing.T) {
	provider := &mockHostProvider{
		hosts:     []string{"http://host1:3500", "http://host2:3500"},
		hostIndex: 0,
	}
	checker := &mockReadyChecker{results: []bool{false, true}}
	assert.Equal(t, true, EnsureReady(t.Context(), provider, checker))
	assert.Equal(t, 1, provider.hostIndex)
}

func TestEnsureReady_MultipleHostsNoneReady(t *testing.T) {
	provider := &mockHostProvider{
		hosts:     []string{"http://host1:3500", "http://host2:3500", "http://host3:3500"},
		hostIndex: 0,
	}
	checker := &mockReadyChecker{results: []bool{false, false, false}}
	assert.Equal(t, false, EnsureReady(t.Context(), provider, checker))
}

func TestEnsureReady_WrapAroundFromNonZeroIndex(t *testing.T) {
	provider := &mockHostProvider{
		hosts:     []string{"http://host0:3500", "http://host1:3500", "http://host2:3500"},
		hostIndex: 1,
	}
	// host1 (start) fails, host2 fails, host0 succeeds
	checker := &mockReadyChecker{results: []bool{false, false, true}}
	assert.Equal(t, true, EnsureReady(t.Context(), provider, checker))
	assert.Equal(t, 0, provider.hostIndex)
}
