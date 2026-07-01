package operations

import (
	"context"
	"path"
	"testing"

	coreblocks "github.com/OffchainLabs/prysm/v7/beacon-chain/core/blocks"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/validators"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	common "github.com/OffchainLabs/prysm/v7/testing/spectest/shared/common/operations"
	"github.com/OffchainLabs/prysm/v7/testing/spectest/utils"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/golang/snappy"
)

func RunVoluntaryExitChurnTest(t *testing.T, config string) {
	require.NoError(t, utils.SetConfig(t, config))
	testFolders, testsFolderPath := utils.TestFolders(t, config, version.String(version.Gloas), "operations/voluntary_exit_churn/pyspec_tests")
	if len(testFolders) == 0 {
		t.Fatalf("No test folders found for voluntary_exit_churn")
	}
	for _, folder := range testFolders {
		t.Run(folder.Name(), func(t *testing.T) {
			folderPath := path.Join(testsFolderPath, folder.Name())
			exitFile, err := util.BazelFileBytes(folderPath, "voluntary_exit.ssz_snappy")
			require.NoError(t, err)
			exitSSZ, err := snappy.Decode(nil /* dst */, exitFile)
			require.NoError(t, err, "Failed to decompress")
			blk, err := blockWithVoluntaryExit(exitSSZ)
			require.NoError(t, err)
			common.RunBlockOperationTest(t, folderPath, blk, sszToState, func(ctx context.Context, s state.BeaconState, b interfaces.ReadOnlySignedBeaconBlock) (state.BeaconState, error) {
				return coreblocks.ProcessVoluntaryExits(ctx, s, b.Block().Body().VoluntaryExits(), validators.ExitInformation(s))
			})
		})
	}
}
