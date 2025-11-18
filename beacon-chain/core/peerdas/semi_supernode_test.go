package peerdas

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/ethereum/go-ethereum/p2p/enode"
)

func TestSemiSupernodeCustody(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig()
	cfg.NumberOfCustodyGroups = 128
	cfg.NumberOfColumns = 128
	params.OverrideBeaconConfig(cfg)

	// Create a test node ID
	nodeID := enode.ID([32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32})

	t.Run("semi-supernode custodies exactly 64 columns", func(t *testing.T) {
		// Semi-supernode uses 64 custody groups (half of 128)
		const semiSupernodeCustodyGroupCount = 64

		// Get custody groups for semi-supernode
		custodyGroups, err := CustodyGroups(nodeID, semiSupernodeCustodyGroupCount)
		require.NoError(t, err)
		require.Equal(t, semiSupernodeCustodyGroupCount, len(custodyGroups))

		// Verify we get exactly 64 custody columns
		custodyColumns, err := CustodyColumns(custodyGroups)
		require.NoError(t, err)
		require.Equal(t, semiSupernodeCustodyGroupCount, len(custodyColumns))

		// Verify the columns are valid (within 0-127 range)
		for columnIndex := range custodyColumns {
			if columnIndex >= cfg.NumberOfColumns {
				t.Fatalf("Invalid column index %d, should be less than %d", columnIndex, cfg.NumberOfColumns)
			}
		}
	})

	t.Run("64 columns is exactly the minimum for reconstruction", func(t *testing.T) {
		minimumCount := MinimumColumnCountToReconstruct()
		require.Equal(t, uint64(64), minimumCount)
	})

	t.Run("semi-supernode vs supernode custody", func(t *testing.T) {
		// Semi-supernode (64 custody groups)
		semiSupernodeGroups, err := CustodyGroups(nodeID, 64)
		require.NoError(t, err)
		semiSupernodeColumns, err := CustodyColumns(semiSupernodeGroups)
		require.NoError(t, err)

		// Supernode (128 custody groups = all groups)
		supernodeGroups, err := CustodyGroups(nodeID, 128)
		require.NoError(t, err)
		supernodeColumns, err := CustodyColumns(supernodeGroups)
		require.NoError(t, err)

		// Verify semi-supernode has exactly half the columns of supernode
		require.Equal(t, 64, len(semiSupernodeColumns))
		require.Equal(t, 128, len(supernodeColumns))
		require.Equal(t, len(supernodeColumns)/2, len(semiSupernodeColumns))

		// Verify all semi-supernode columns are a subset of supernode columns
		for columnIndex := range semiSupernodeColumns {
			if !supernodeColumns[columnIndex] {
				t.Fatalf("Semi-supernode column %d not found in supernode columns", columnIndex)
			}
		}
	})
}
