package peerdas_test

import (
	"testing"

	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/peerdas"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func TestInfo(t *testing.T) {
	nodeID := enode.ID{}
	custodyGroupCount := uint64(7)

	expectedCustodyGroup := map[uint64]bool{1: true, 17: true, 19: true, 42: true, 75: true, 87: true, 102: true}
	expectedCustodyColumns := map[uint64]bool{1: true, 17: true, 19: true, 42: true, 75: true, 87: true, 102: true}
	expectedDataColumnsSubnets := map[uint64]bool{1: true, 17: true, 19: true, 42: true, 75: true, 87: true, 102: true}

	for _, cached := range []bool{false, true} {
		actual, ok, err := peerdas.Info(nodeID, custodyGroupCount)
		require.NoError(t, err)
		require.Equal(t, cached, ok)
		require.DeepEqual(t, expectedCustodyGroup, actual.CustodyGroups)
		require.DeepEqual(t, expectedCustodyColumns, actual.CustodyColumns)
		require.DeepEqual(t, expectedDataColumnsSubnets, actual.DataColumnsSubnets)
	}
}
