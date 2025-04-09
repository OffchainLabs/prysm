package epoch_processing

import (
	"path"
	"testing"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/electra"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/spectest/utils"
)

// RunHistoricalSummariesUpdateTests executes "epoch_processing/historical_Summaries_update" tests.
func RunHistoricalSummariesUpdateTests(t *testing.T, config string) {
	require.NoError(t, utils.SetConfig(t, config))

	testFolders, testsFolderPath := utils.TestFolders(t, config, "electra", "epoch_processing/historical_summaries_update/pyspec_tests")
	for _, folder := range testFolders {
		t.Run(folder.Name(), func(t *testing.T) {
			folderPath := path.Join(testsFolderPath, folder.Name())
			RunEpochOperationTest(t, folderPath, processHistoricalSummariesUpdateWrapper)
		})
	}
}

func processHistoricalSummariesUpdateWrapper(t *testing.T, st state.BeaconState) (state.BeaconState, error) {
	st, err := electra.ProcessHistoricalDataUpdate(st)
	require.NoError(t, err, "Could not process final updates")
	return st, nil
}
