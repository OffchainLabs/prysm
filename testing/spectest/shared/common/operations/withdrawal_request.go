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

func RunWithdrawalRequestTest(t *testing.T, config string, fork string, block blockWithSSZObject, sszToState SSZToState) {
	require.NoError(t, utils.SetConfig(t, config))
	testFolders, testsFolderPath := utils.TestFolders(t, config, fork, "operations/withdrawal_request/pyspec_tests")
	if len(testFolders) == 0 {
		t.Fatalf("No test folders found for %s/%s/%s", config, fork, "operations/withdrawal_request/pyspec_tests")
	}
	for _, folder := range testFolders {
		t.Run(folder.Name(), func(t *testing.T) {
			folderPath := path.Join(testsFolderPath, folder.Name())
			withdrawalRequestFile, err := util.BazelFileBytes(folderPath, "withdrawal_request.ssz_snappy")
			require.NoError(t, err)
			withdrawalRequestSSZ, err := snappy.Decode(nil /* dst */, withdrawalRequestFile)
			require.NoError(t, err, "Failed to decompress")
			blk, err := block(withdrawalRequestSSZ)
			require.NoError(t, err)
			RunBlockOperationTest(t, folderPath, blk, sszToState, func(ctx context.Context, s state.BeaconState, b interfaces.ReadOnlySignedBeaconBlock) (state.BeaconState, error) {
				bod := b.Block().Body()
				e, err := bod.ExecutionRequests()
				require.NoError(t, err)
				return electra.ProcessWithdrawalRequests(ctx, s, e.Withdrawals)
			})
		})
	}
}
