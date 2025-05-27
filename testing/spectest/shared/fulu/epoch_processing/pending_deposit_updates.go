package epoch_processing

import (
	"context"
	"path"
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/electra"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/spectest/utils"
)

func RunPendingDepositsTests(t *testing.T, config string) {
	require.NoError(t, utils.SetConfig(t, config))

	testFolders, testsFolderPath := utils.TestFolders(t, config, "fulu", "epoch_processing/pending_deposits/pyspec_tests")
	for _, folder := range testFolders {
		t.Run(folder.Name(), func(t *testing.T) {
			folderPath := path.Join(testsFolderPath, folder.Name())
			RunEpochOperationTest(t, folderPath, processPendingDeposits)
		})
	}
}

func processPendingDeposits(t *testing.T, st state.BeaconState) (state.BeaconState, error) {
	// The caller of this method would normally have the precompute balance values for total
	// active balance for this epoch. For ease of test setup, we will compute total active
	// balance from the given state.
	tab, err := helpers.TotalActiveBalance(st)
	require.NoError(t, err)
	return st, electra.ProcessPendingDeposits(context.TODO(), st, primitives.Gwei(tab))
}
