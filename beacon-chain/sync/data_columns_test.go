package sync

import (
	"testing"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func TestSelectPeersToFetchDataColumnsFrom(t *testing.T) {
	testCases := []struct {
		name string

		// Inputs
		neededDataColumns map[uint64]bool
		dataColumnsByPeer map[peer.ID]map[uint64]bool

		// Expected outputs
		dataColumnsToFetchByPeer map[peer.ID][]uint64
		err                      error
	}{
		{
			name:              "no data columns needed",
			neededDataColumns: map[uint64]bool{},
			dataColumnsByPeer: map[peer.ID]map[uint64]bool{
				peer.ID("peer1"): {1: true, 2: true},
				peer.ID("peer2"): {3: true, 4: true},
			},
			dataColumnsToFetchByPeer: map[peer.ID][]uint64{},
			err:                      nil,
		},
		{
			name:              "one peer has all data columns needed",
			neededDataColumns: map[uint64]bool{1: true, 3: true, 5: true},
			dataColumnsByPeer: map[peer.ID]map[uint64]bool{
				peer.ID("peer1"): {2: true, 4: true},
				peer.ID("peer2"): {1: true, 3: true, 5: true, 7: true, 9: true},
				peer.ID("peer3"): {6: true},
			},
			dataColumnsToFetchByPeer: map[peer.ID][]uint64{
				peer.ID("peer2"): {1, 3, 5},
			},
			err: nil,
		},
		{
			name:              "multiple peers are needed - 1",
			neededDataColumns: map[uint64]bool{1: true, 3: true, 5: true, 7: true, 9: true},
			dataColumnsByPeer: map[peer.ID]map[uint64]bool{
				peer.ID("peer1"): {3: true, 7: true},
				peer.ID("peer2"): {1: true, 3: true, 5: true, 9: true, 10: true},
				peer.ID("peer3"): {6: true, 10: true, 12: true, 14: true, 16: true, 18: true, 20: true},
				peer.ID("peer4"): {9: true},
			},
			dataColumnsToFetchByPeer: map[peer.ID][]uint64{
				peer.ID("peer2"): {1, 3, 5, 9},
				peer.ID("peer1"): {7},
			},
			err: nil,
		},
		{
			name:              "multiple peers are needed - 2",
			neededDataColumns: map[uint64]bool{1: true, 3: true, 5: true, 7: true, 9: true},
			dataColumnsByPeer: map[peer.ID]map[uint64]bool{
				peer.ID("peer1"): {9: true, 10: true},
				peer.ID("peer2"): {3: true, 7: true},
				peer.ID("peer3"): {1: true, 5: true},
			},
			dataColumnsToFetchByPeer: map[peer.ID][]uint64{
				peer.ID("peer1"): {9},
				peer.ID("peer2"): {3, 7},
				peer.ID("peer3"): {1, 5},
			},
			err: nil,
		},
		{
			name:              "some columns are not owned by any peer",
			neededDataColumns: map[uint64]bool{1: true, 3: true, 5: true, 7: true, 9: true},
			dataColumnsByPeer: map[peer.ID]map[uint64]bool{
				peer.ID("peer1"): {9: true, 10: true},
				peer.ID("peer2"): {2: true, 6: true},
				peer.ID("peer3"): {1: true, 5: true},
			},
			dataColumnsToFetchByPeer: map[peer.ID][]uint64{
				peer.ID("peer1"): {9},
				peer.ID("peer3"): {1, 5},
			},
			err: errors.New("no peer to fetch the following data columns: [3 7]"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := SelectPeersToFetchDataColumnsFrom(tc.neededDataColumns, tc.dataColumnsByPeer)

			if tc.err != nil {
				require.Equal(t, tc.err.Error(), err.Error())
			} else {
				require.NoError(t, err)
			}

			expected := tc.dataColumnsToFetchByPeer
			require.Equal(t, len(expected), len(actual))

			for peerID, expectedDataColumns := range expected {
				actualDataColumns, ok := actual[peerID]
				require.Equal(t, true, ok)
				require.DeepSSZEqual(t, expectedDataColumns, actualDataColumns)
			}
		})
	}

}
