package epoch_processing

import (
	"path"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/gloas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/spectest/utils"
)

// RunPTCWindowTests executes "epoch_processing/ptc_window" tests.
func RunPTCWindowTests(t *testing.T, config string) {
	require.NoError(t, utils.SetConfig(t, config))

	testFolders, testsFolderPath := utils.TestFolders(t, config, "gloas", "epoch_processing/ptc_window/pyspec_tests")
	for _, folder := range testFolders {
		t.Run(folder.Name(), func(t *testing.T) {
			folderPath := path.Join(testsFolderPath, folder.Name())
			RunEpochOperationTest(t, folderPath, processPTCWindowWrapper)
		})
	}
}

func processPTCWindowWrapper(t *testing.T, st state.BeaconState) (state.BeaconState, error) {
	ctx := t.Context()
	if err := gloas.ProcessPTCWindow(ctx, st); err != nil {
		return nil, err
	}
	return st, nil
}
