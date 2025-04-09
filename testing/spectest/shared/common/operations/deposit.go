package operations

import (
	"context"
	"path"
	"testing"

	"github.com/golang/snappy"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/spectest/utils"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
)

type ProcessDeposits func(ctx context.Context, beaconState state.BeaconState, deposits []*ethpb.Deposit) (state.BeaconState, error)

func RunDepositTest(t *testing.T, config string, fork string, blockWithDeposit blockWithSSZObject, process ProcessDeposits, sszToState SSZToState) {
	require.NoError(t, utils.SetConfig(t, config))
	testFolders, testsFolderPath := utils.TestFolders(t, config, fork, "operations/deposit/pyspec_tests")
	for _, folder := range testFolders {
		t.Run(folder.Name(), func(t *testing.T) {
			folderPath := path.Join(testsFolderPath, folder.Name())
			depositFile, err := util.BazelFileBytes(folderPath, "deposit.ssz_snappy")
			require.NoError(t, err)
			depositSSZ, err := snappy.Decode(nil /* dst */, depositFile)
			require.NoError(t, err, "Failed to decompress")
			blk, err := blockWithDeposit(depositSSZ)
			require.NoError(t, err)
			RunBlockOperationTest(t, folderPath, blk, sszToState, func(ctx context.Context, s state.BeaconState, b interfaces.ReadOnlySignedBeaconBlock) (state.BeaconState, error) {
				return process(ctx, s, b.Block().Body().Deposits())
			})
		})
	}
}
