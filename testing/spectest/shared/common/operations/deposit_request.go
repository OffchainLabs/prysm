package operations

import (
	"context"
	"path"
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/electra"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/spectest/utils"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/golang/snappy"
)

func RunDepositRequestsTest(t *testing.T, config string, fork string, block blockWithSSZObject, sszToState SSZToState) {
	require.NoError(t, utils.SetConfig(t, config))
	testFolders, testsFolderPath := utils.TestFolders(t, config, fork, "operations/deposit_request/pyspec_tests")
	for _, folder := range testFolders {
		t.Run(folder.Name(), func(t *testing.T) {
			folderPath := path.Join(testsFolderPath, folder.Name())
			depositRequestFile, err := util.BazelFileBytes(folderPath, "deposit_request.ssz_snappy")
			require.NoError(t, err)
			depositRequestSSZ, err := snappy.Decode(nil /* dst */, depositRequestFile)
			require.NoError(t, err, "Failed to decompress")
			blk, err := block(depositRequestSSZ)
			require.NoError(t, err)
			RunBlockOperationTest(t, folderPath, blk, sszToState, func(ctx context.Context, s state.BeaconState, b interfaces.ReadOnlySignedBeaconBlock) (state.BeaconState, error) {
				e, err := b.Block().Body().ExecutionRequests()
				require.NoError(t, err, "Failed to get execution requests")
				return electra.ProcessDepositRequests(ctx, s, e.Deposits)
			})
		})
	}
}
