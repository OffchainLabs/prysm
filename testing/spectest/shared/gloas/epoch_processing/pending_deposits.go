package epoch_processing

import (
	"context"
	"path"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/electra"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/spectest/utils"
)

func RunPendingDepositsTests(t *testing.T, config string) {
	runPendingDepositsTestsAt(t, config, "epoch_processing/pending_deposits/pyspec_tests")
}

func RunPendingDepositsChurnTests(t *testing.T, config string) {
	runPendingDepositsTestsAt(t, config, "epoch_processing/pending_deposits_churn/pyspec_tests")
}

func runPendingDepositsTestsAt(t *testing.T, config, testPath string) {
	require.NoError(t, utils.SetConfig(t, config))

	testFolders, testsFolderPath := utils.TestFolders(t, config, "gloas", testPath)
	for _, folder := range testFolders {
		t.Run(folder.Name(), func(t *testing.T) {
			folderPath := path.Join(testsFolderPath, folder.Name())
			RunEpochOperationTest(t, folderPath, processPendingDeposits)
		})
	}
}

func processPendingDeposits(t *testing.T, st state.BeaconState) (state.BeaconState, error) {
	tab, err := helpers.TotalActiveBalance(st)
	require.NoError(t, err)
	return st, electra.ProcessPendingDeposits(context.TODO(), st, primitives.Gwei(tab))
}
