package epoch_processing

import (
	"path"
	"testing"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/electra"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/spectest/utils"
)

// RunEffectiveBalanceUpdatesTests executes "epoch_processing/effective_balance_updates" tests.
func RunEffectiveBalanceUpdatesTests(t *testing.T, config string) {
	require.NoError(t, utils.SetConfig(t, config))

	testFolders, testsFolderPath := utils.TestFolders(t, config, "electra", "epoch_processing/effective_balance_updates/pyspec_tests")
	for _, folder := range testFolders {
		t.Run(folder.Name(), func(t *testing.T) {
			folderPath := path.Join(testsFolderPath, folder.Name())
			RunEpochOperationTest(t, folderPath, processEffectiveBalanceUpdatesWrapper)
		})
	}
}

func processEffectiveBalanceUpdatesWrapper(t *testing.T, st state.BeaconState) (state.BeaconState, error) {
	err := electra.ProcessEffectiveBalanceUpdates(st)
	require.NoError(t, err, "Could not process final updates")
	return st, nil
}
