package operations

import (
	"context"
	"path"
	"testing"

	"github.com/golang/snappy"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/electra"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/spectest/utils"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
)

func RunDepositRequestsTest(t *testing.T, config string, fork string, block blockWithSSZObject, sszToState SSZToState) {
	require.NoError(t, utils.SetConfig(t, config))
	testFolders, testsFolderPath := utils.TestFolders(t, config, "electra", "operations/deposit_request/pyspec_tests")
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
