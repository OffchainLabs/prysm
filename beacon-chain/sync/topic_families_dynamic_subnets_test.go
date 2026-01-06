package sync

import (
	"fmt"
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/stretchr/testify/require"
)

// mockDynamicSubnetFamily is a test implementation of dynamicSubnetFamily.
type mockDynamicSubnetFamily struct {
	meshSubnets   map[uint64]bool
	fanoutSubnets map[uint64]bool
	topicPrefix   string
}

func (m *mockDynamicSubnetFamily) getSubnetsToJoin(_ primitives.Slot) map[uint64]bool {
	return m.meshSubnets
}

func (m *mockDynamicSubnetFamily) getSubnetsForBroadcast(_ primitives.Slot) map[uint64]bool {
	return m.fanoutSubnets
}

func (m *mockDynamicSubnetFamily) getFullTopicString(subnet uint64) string {
	return fmt.Sprintf("%s/subnet/%d", m.topicPrefix, subnet)
}

func TestTopicsWithMinPeerCount(t *testing.T) {
	tests := []struct {
		name           string
		meshSubnets    map[uint64]bool
		fanoutSubnets  map[uint64]bool
		minMeshPeers   int
		minFanoutPeers int
		expected       map[string]int
	}{
		{
			name:           "empty subnets returns empty map",
			meshSubnets:    nil,
			fanoutSubnets:  nil,
			minMeshPeers:   8,
			minFanoutPeers: 6,
			expected:       map[string]int{},
		},
		{
			name:           "empty maps returns empty map",
			meshSubnets:    map[uint64]bool{},
			fanoutSubnets:  map[uint64]bool{},
			minMeshPeers:   8,
			minFanoutPeers: 6,
			expected:       map[string]int{},
		},
		{
			name:           "only mesh subnets",
			meshSubnets:    map[uint64]bool{1: true, 2: true, 3: true},
			fanoutSubnets:  nil,
			minMeshPeers:   8,
			minFanoutPeers: 6,
			expected: map[string]int{
				"test/subnet/1": 8,
				"test/subnet/2": 8,
				"test/subnet/3": 8,
			},
		},
		{
			name:           "only fanout subnets",
			meshSubnets:    nil,
			fanoutSubnets:  map[uint64]bool{4: true, 5: true},
			minMeshPeers:   8,
			minFanoutPeers: 6,
			expected: map[string]int{
				"test/subnet/4": 6,
				"test/subnet/5": 6,
			},
		},
		{
			name:           "mesh and fanout with no overlap",
			meshSubnets:    map[uint64]bool{1: true, 2: true},
			fanoutSubnets:  map[uint64]bool{3: true, 4: true},
			minMeshPeers:   8,
			minFanoutPeers: 6,
			expected: map[string]int{
				"test/subnet/1": 8,
				"test/subnet/2": 8,
				"test/subnet/3": 6,
				"test/subnet/4": 6,
			},
		},
		{
			name:           "fanout subset of mesh - all get mesh peer count",
			meshSubnets:    map[uint64]bool{1: true, 2: true, 3: true, 4: true},
			fanoutSubnets:  map[uint64]bool{2: true, 3: true},
			minMeshPeers:   8,
			minFanoutPeers: 6,
			expected: map[string]int{
				"test/subnet/1": 8,
				"test/subnet/2": 8, // in both, mesh takes precedence
				"test/subnet/3": 8, // in both, mesh takes precedence
				"test/subnet/4": 8,
			},
		},
		{
			name:           "mesh subset of fanout - mesh subnets get mesh count, remaining get fanout",
			meshSubnets:    map[uint64]bool{2: true, 3: true},
			fanoutSubnets:  map[uint64]bool{1: true, 2: true, 3: true, 4: true},
			minMeshPeers:   8,
			minFanoutPeers: 6,
			expected: map[string]int{
				"test/subnet/1": 6, // fanout only
				"test/subnet/2": 8, // in both, mesh takes precedence
				"test/subnet/3": 8, // in both, mesh takes precedence
				"test/subnet/4": 6, // fanout only
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockDynamicSubnetFamily{
				meshSubnets:   tt.meshSubnets,
				fanoutSubnets: tt.fanoutSubnets,
				topicPrefix:   "test",
			}

			result := topicsWithMinPeerCount(mock, 0, tt.minMeshPeers, tt.minFanoutPeers)

			require.Equal(t, len(tt.expected), len(result), "result length mismatch")
			for topic, expectedCount := range tt.expected {
				actualCount, exists := result[topic]
				require.True(t, exists, "expected topic %s not found in result", topic)
				require.Equal(t, expectedCount, actualCount, "peer count mismatch for topic %s", topic)
			}
		})
	}
}
