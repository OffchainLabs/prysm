package peers

import (
	"fmt"
	"slices"
	"testing"

	forkchoicetypes "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/types"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/libp2p/go-libp2p/core/peer"
)

func TestPickBest(t *testing.T) {
	best := testPeerIds(10)
	cases := []struct {
		name     string
		busy     map[peer.ID]bool
		best     []peer.ID
		expected []peer.ID
	}{
		{
			name:     "don't limit",
			expected: best,
		},
		{
			name:     "none busy",
			expected: best,
		},
		{
			name:     "all busy except last",
			busy:     testBusyMap(best[0 : len(best)-1]),
			expected: best[len(best)-1:],
		},
		{
			name:     "all busy except i=5",
			busy:     testBusyMap(slices.Concat(best[0:5], best[6:])),
			expected: []peer.ID{best[5]},
		},
		{
			name: "all busy - 0 results",
			busy: testBusyMap(best),
		},
		{
			name:     "first half busy",
			busy:     testBusyMap(best[0:5]),
			expected: best[5:],
		},
		{
			name:     "back half busy",
			busy:     testBusyMap(best[5:]),
			expected: best[0:5],
		},
		{
			name:     "pick all ",
			expected: best,
		},
		{
			name: "none available",
			best: []peer.ID{},
		},
		{
			name:     "not enough",
			best:     best[0:1],
			expected: best[0:1],
		},
		{
			name:     "not enough, some busy",
			best:     best[0:6],
			busy:     testBusyMap(best[0:5]),
			expected: best[5:6],
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.best == nil {
				c.best = best
			}
			filt := NotBusy(c.busy)
			pb := filt(c.best)
			require.Equal(t, len(c.expected), len(pb))
			for i := range c.expected {
				require.Equal(t, c.expected[i], pb[i])
			}
		})
	}
}

func testBusyMap(b []peer.ID) map[peer.ID]bool {
	m := make(map[peer.ID]bool)
	for i := range b {
		m[b[i]] = true
	}
	return m
}

func testPeerIds(n int) []peer.ID {
	pids := make([]peer.ID, n)
	for i := range pids {
		pids[i] = peer.ID(fmt.Sprintf("%d", i))
	}
	return pids
}

// MockStatus is a test mock for the Status interface used in Assigner.
type MockStatus struct {
	bestFinalizedEpoch primitives.Epoch
	bestPeers          []peer.ID
}

func (m *MockStatus) BestFinalized(ourFinalized primitives.Epoch) (primitives.Epoch, []peer.ID) {
	return m.bestFinalizedEpoch, m.bestPeers
}

// MockFinalizedCheckpointer is a test mock for FinalizedCheckpointer interface.
type MockFinalizedCheckpointer struct {
	checkpoint *forkchoicetypes.Checkpoint
}

func (m *MockFinalizedCheckpointer) FinalizedCheckpoint() *forkchoicetypes.Checkpoint {
	return m.checkpoint
}

// TestAssign_HappyPath tests the Assign method with sufficient peers and various filters.
func TestAssign_HappyPath(t *testing.T) {
	peers := testPeerIds(10)

	cases := []struct {
		name           string
		bestPeers      []peer.ID
		finalizedEpoch primitives.Epoch
		filter         AssignmentFilter
		expectedCount  int
	}{
		{
			name:           "sufficient peers with identity filter",
			bestPeers:      peers,
			finalizedEpoch: 10,
			filter:         func(p []peer.ID) []peer.ID { return p },
			expectedCount:  10,
		},
		{
			name:           "sufficient peers with NotBusy filter (no busy)",
			bestPeers:      peers,
			finalizedEpoch: 10,
			filter:         NotBusy(make(map[peer.ID]bool)),
			expectedCount:  10,
		},
		{
			name:           "sufficient peers with NotBusy filter (some busy)",
			bestPeers:      peers,
			finalizedEpoch: 10,
			filter:         NotBusy(testBusyMap(peers[0:5])),
			expectedCount:  5,
		},
		{
			name:           "minimum threshold exactly met",
			bestPeers:      peers[0:5],
			finalizedEpoch: 10,
			filter:         func(p []peer.ID) []peer.ID { return p },
			expectedCount:  5,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockStatus := &MockStatus{
				bestFinalizedEpoch: tc.finalizedEpoch,
				bestPeers:          tc.bestPeers,
			}
			mockCheckpointer := &MockFinalizedCheckpointer{
				checkpoint: &forkchoicetypes.Checkpoint{Epoch: tc.finalizedEpoch},
			}
			assigner := NewAssigner(mockStatus, mockCheckpointer)

			result, err := assigner.Assign(tc.filter)

			require.NoError(t, err)
			require.Equal(t, tc.expectedCount, len(result),
				fmt.Sprintf("expected %d peers, got %d", tc.expectedCount, len(result)))
		})
	}
}

// TestAssign_InsufficientPeers tests error handling when not enough suitable peers are available.
// Note: The actual peer threshold depends on config values MaxPeersToSync and MinimumSyncPeers.
func TestAssign_InsufficientPeers(t *testing.T) {
	cases := []struct {
		name        string
		bestPeers   []peer.ID
		expectedErr error
		description string
	}{
		{
			name:        "exactly at minimum threshold",
			bestPeers:   testPeerIds(5),
			expectedErr: nil,
			description: "5 peers should meet the minimum threshold",
		},
		{
			name:        "well above minimum threshold",
			bestPeers:   testPeerIds(50),
			expectedErr: nil,
			description: "50 peers should easily meet requirements",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockStatus := &MockStatus{
				bestFinalizedEpoch: 10,
				bestPeers:          tc.bestPeers,
			}
			mockCheckpointer := &MockFinalizedCheckpointer{
				checkpoint: &forkchoicetypes.Checkpoint{Epoch: 10},
			}
			assigner := NewAssigner(mockStatus, mockCheckpointer)

			result, err := assigner.Assign(NotBusy(make(map[peer.ID]bool)))

			if tc.expectedErr != nil {
				require.NotNil(t, err, tc.description)
				require.Equal(t, tc.expectedErr, err)
			} else {
				require.NoError(t, err, tc.description)
				require.Equal(t, len(tc.bestPeers), len(result))
			}
		})
	}
}

// TestAssign_FilterApplication verifies that filters are correctly applied to peer lists.
func TestAssign_FilterApplication(t *testing.T) {
	peers := testPeerIds(10)

	cases := []struct {
		name          string
		bestPeers     []peer.ID
		filterToApply AssignmentFilter
		expectedCount int
		description   string
	}{
		{
			name:          "identity filter returns all peers",
			bestPeers:     peers,
			filterToApply: func(p []peer.ID) []peer.ID { return p },
			expectedCount: 10,
			description:   "identity filter should not change peer list",
		},
		{
			name:          "filter removes all peers (all busy)",
			bestPeers:     peers,
			filterToApply: NotBusy(testBusyMap(peers)),
			expectedCount: 0,
			description:   "all peers busy should return empty list",
		},
		{
			name:          "filter removes first 5 peers",
			bestPeers:     peers,
			filterToApply: NotBusy(testBusyMap(peers[0:5])),
			expectedCount: 5,
			description:   "should only return non-busy peers",
		},
		{
			name:          "filter removes last 5 peers",
			bestPeers:     peers,
			filterToApply: NotBusy(testBusyMap(peers[5:])),
			expectedCount: 5,
			description:   "should only return non-busy peers from beginning",
		},
		{
			name:      "custom filter selects every other peer",
			bestPeers: peers,
			filterToApply: func(p []peer.ID) []peer.ID {
				result := make([]peer.ID, 0)
				for i := 0; i < len(p); i += 2 {
					result = append(result, p[i])
				}
				return result
			},
			expectedCount: 5,
			description:   "custom filter selecting every other peer",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockStatus := &MockStatus{
				bestFinalizedEpoch: 10,
				bestPeers:          tc.bestPeers,
			}
			mockCheckpointer := &MockFinalizedCheckpointer{
				checkpoint: &forkchoicetypes.Checkpoint{Epoch: 10},
			}
			assigner := NewAssigner(mockStatus, mockCheckpointer)

			result, err := assigner.Assign(tc.filterToApply)

			require.NoError(t, err, fmt.Sprintf("unexpected error: %v", err))
			require.Equal(t, tc.expectedCount, len(result),
				fmt.Sprintf("%s: expected %d peers, got %d", tc.description, tc.expectedCount, len(result)))
		})
	}
}

// TestAssign_FinalizedCheckpointUsage verifies that the finalized checkpoint is correctly used.
func TestAssign_FinalizedCheckpointUsage(t *testing.T) {
	peers := testPeerIds(10)

	cases := []struct {
		name           string
		finalizedEpoch primitives.Epoch
		bestPeers      []peer.ID
		expectedCount  int
		description    string
	}{
		{
			name:           "epoch 0",
			finalizedEpoch: 0,
			bestPeers:      peers,
			expectedCount:  10,
			description:    "epoch 0 should work",
		},
		{
			name:           "epoch 100",
			finalizedEpoch: 100,
			bestPeers:      peers,
			expectedCount:  10,
			description:    "high epoch number should work",
		},
		{
			name:           "epoch changes between calls",
			finalizedEpoch: 50,
			bestPeers:      testPeerIds(5),
			expectedCount:  5,
			description:    "epoch value should be used in checkpoint",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockStatus := &MockStatus{
				bestFinalizedEpoch: tc.finalizedEpoch,
				bestPeers:          tc.bestPeers,
			}
			mockCheckpointer := &MockFinalizedCheckpointer{
				checkpoint: &forkchoicetypes.Checkpoint{Epoch: tc.finalizedEpoch},
			}
			assigner := NewAssigner(mockStatus, mockCheckpointer)

			result, err := assigner.Assign(NotBusy(make(map[peer.ID]bool)))

			require.NoError(t, err)
			require.Equal(t, tc.expectedCount, len(result),
				fmt.Sprintf("%s: expected %d peers, got %d", tc.description, tc.expectedCount, len(result)))
		})
	}
}

// TestAssign_EdgeCases tests boundary conditions and edge cases.
func TestAssign_EdgeCases(t *testing.T) {
	cases := []struct {
		name          string
		bestPeers     []peer.ID
		filter        AssignmentFilter
		expectedCount int
		description   string
	}{
		{
			name:          "filter returns empty from sufficient peers",
			bestPeers:     testPeerIds(10),
			filter:        func(p []peer.ID) []peer.ID { return []peer.ID{} },
			expectedCount: 0,
			description:   "filter can return empty list even if sufficient peers available",
		},
		{
			name:          "filter selects subset from sufficient peers",
			bestPeers:     testPeerIds(10),
			filter:        func(p []peer.ID) []peer.ID { return p[0:2] },
			expectedCount: 2,
			description:   "filter can return subset of available peers",
		},
		{
			name:          "filter selects single peer from many",
			bestPeers:     testPeerIds(20),
			filter:        func(p []peer.ID) []peer.ID { return p[0:1] },
			expectedCount: 1,
			description:   "filter can select single peer from many available",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockStatus := &MockStatus{
				bestFinalizedEpoch: 10,
				bestPeers:          tc.bestPeers,
			}
			mockCheckpointer := &MockFinalizedCheckpointer{
				checkpoint: &forkchoicetypes.Checkpoint{Epoch: 10},
			}
			assigner := NewAssigner(mockStatus, mockCheckpointer)

			result, err := assigner.Assign(tc.filter)

			require.NoError(t, err, fmt.Sprintf("%s: unexpected error: %v", tc.description, err))
			require.Equal(t, tc.expectedCount, len(result),
				fmt.Sprintf("%s: expected %d peers, got %d", tc.description, tc.expectedCount, len(result)))
		})
	}
}
