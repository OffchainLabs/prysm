package fallback

import (
	"context"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	logTest "github.com/sirupsen/logrus/hooks/test"
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

// mockReadyChecker records per-call ReadyStatus results in sequence.
type mockReadyChecker struct {
	results []NodeStatus
	idx     int
}

func (m *mockReadyChecker) ReadyStatus(_ context.Context) NodeStatus {
	if m.idx >= len(m.results) {
		return NodeUnavailable
	}
	r := m.results[m.idx]
	m.idx++
	return r
}

func TestEnsureReady_SingleHostReady(t *testing.T) {
	provider := &mockHostProvider{hosts: []string{"http://host1:3500"}, hostIndex: 0}
	checker := &mockReadyChecker{results: []NodeStatus{NodeReady}}
	assert.Equal(t, true, EnsureReady(t.Context(), provider, checker))
	assert.Equal(t, 0, provider.hostIndex)
}

func TestEnsureReady_SingleHostNotReady(t *testing.T) {
	provider := &mockHostProvider{hosts: []string{"http://host1:3500"}, hostIndex: 0}
	checker := &mockReadyChecker{results: []NodeStatus{NodeUnavailable}}
	assert.Equal(t, false, EnsureReady(t.Context(), provider, checker))
}

func TestEnsureReady_SingleHostError(t *testing.T) {
	provider := &mockHostProvider{hosts: []string{"http://host1:3500"}, hostIndex: 0}
	checker := &mockReadyChecker{results: []NodeStatus{NodeUnavailable}}
	assert.Equal(t, false, EnsureReady(t.Context(), provider, checker))
}

func TestEnsureReady_MultipleHostsFirstReady(t *testing.T) {
	provider := &mockHostProvider{
		hosts:     []string{"http://host1:3500", "http://host2:3500"},
		hostIndex: 0,
	}
	checker := &mockReadyChecker{results: []NodeStatus{NodeReady}}
	assert.Equal(t, true, EnsureReady(t.Context(), provider, checker))
	assert.Equal(t, 0, provider.hostIndex)
}

func TestEnsureReady_MultipleHostsFailoverToSecond(t *testing.T) {
	provider := &mockHostProvider{
		hosts:     []string{"http://host1:3500", "http://host2:3500"},
		hostIndex: 0,
	}
	checker := &mockReadyChecker{results: []NodeStatus{NodeUnavailable, NodeReady}}
	assert.Equal(t, true, EnsureReady(t.Context(), provider, checker))
	assert.Equal(t, 1, provider.hostIndex)
}

func TestEnsureReady_MultipleHostsNoneReady(t *testing.T) {
	provider := &mockHostProvider{
		hosts:     []string{"http://host1:3500", "http://host2:3500", "http://host3:3500"},
		hostIndex: 0,
	}
	checker := &mockReadyChecker{results: []NodeStatus{NodeUnavailable, NodeUnavailable, NodeUnavailable}}
	assert.Equal(t, false, EnsureReady(t.Context(), provider, checker))
}

func TestEnsureReady_WrapAroundFromNonZeroIndex(t *testing.T) {
	provider := &mockHostProvider{
		hosts:     []string{"http://host0:3500", "http://host1:3500", "http://host2:3500"},
		hostIndex: 1,
	}
	// host1 (start) fails, host2 fails, host0 succeeds
	checker := &mockReadyChecker{results: []NodeStatus{NodeUnavailable, NodeUnavailable, NodeReady}}
	assert.Equal(t, true, EnsureReady(t.Context(), provider, checker))
	assert.Equal(t, 0, provider.hostIndex)
}

func TestEnsureReady_LogsReasonWhenNoneReady(t *testing.T) {
	hook := logTest.NewGlobal()
	provider := &mockHostProvider{
		hosts:     []string{"http://host1:3500", "http://host2:3500"},
		hostIndex: 0,
	}
	checker := &mockReadyChecker{results: []NodeStatus{NodeOptimistic, NodeOptimistic}}
	assert.Equal(t, false, EnsureReady(t.Context(), provider, checker))
	require.LogsContain(t, hook, "Beacon node is optimistic")
	require.LogsContain(t, hook, "http://host1:3500=optimistic, http://host2:3500=optimistic")
}

func TestLogNotReady(t *testing.T) {
	tests := []struct {
		name     string
		statuses []HostStatus
		wanted   string
	}{
		{
			name:     "all unavailable",
			statuses: []HostStatus{{Host: "http://host1:3500", Status: NodeUnavailable}, {Host: "http://host2:3500", Status: NodeUnavailable}},
			wanted:   "No reachable beacon node found",
		},
		{
			name:     "all syncing",
			statuses: []HostStatus{{Host: "http://host1:3500", Status: NodeSyncing}},
			wanted:   "Beacon node is syncing",
		},
		{
			name:     "all optimistic",
			statuses: []HostStatus{{Host: "http://host1:3500", Status: NodeOptimistic}},
			wanted:   "Beacon node is optimistic",
		},
		{
			name:     "mixed",
			statuses: []HostStatus{{Host: "http://host1:3500", Status: NodeOptimistic}, {Host: "http://host2:3500", Status: NodeUnavailable}},
			wanted:   "No ready beacon node found",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hook := logTest.NewGlobal()
			LogNotReady(tt.statuses)
			require.LogsContain(t, hook, tt.wanted)
		})
	}
}
