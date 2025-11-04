package epoch_processing

import (
	"path"
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/fulu"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/spectest/utils"
)

// RunProposerLookaheadTests executes "epoch_processing/proposer_lookahead" tests.
func RunProposerLookaheadTests(t *testing.T, config string) {
	require.NoError(t, utils.SetConfig(t, config))

	testFolders, testsFolderPath := utils.TestFolders(t, config, "fulu", "epoch_processing/proposer_lookahead/pyspec_tests")
	for _, folder := range testFolders {
		t.Run(folder.Name(), func(t *testing.T) {
			folderPath := path.Join(testsFolderPath, folder.Name())
			RunEpochOperationTest(t, folderPath, processProposerLookaheadWrapper)
		})
	}
}

func processProposerLookaheadWrapper(t *testing.T, st state.BeaconState) (state.BeaconState, error) {
	ctx := t.Context()
	err := fulu.ProcessProposerLookahead(ctx, st)
	if err != nil {
		return nil, err
	}
	return st, nil
}
