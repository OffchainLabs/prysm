package epoch_processing

import (
	"path"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/gloas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/spectest/utils"
)

func RunBuilderPendingPaymentsTests(t *testing.T, config string) {
	require.NoError(t, utils.SetConfig(t, config))
	testFolders, testsFolderPath := utils.TestFolders(t, config, "gloas", "epoch_processing/builder_pending_payments/pyspec_tests")
	for _, folder := range testFolders {
		t.Run(folder.Name(), func(t *testing.T) {
			folderPath := path.Join(testsFolderPath, folder.Name())
			RunEpochOperationTest(t, folderPath, processBuilderPendingPayments)
		})
	}
}

func processBuilderPendingPayments(t *testing.T, st state.BeaconState) (state.BeaconState, error) {
	return st, gloas.ProcessBuilderPendingPayments(t.Context(), st)
}
