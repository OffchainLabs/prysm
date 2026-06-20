package operations

import (
	"context"
	"path"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/gloas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	common "github.com/OffchainLabs/prysm/v7/testing/spectest/shared/common/operations"
	"github.com/OffchainLabs/prysm/v7/testing/spectest/utils"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/golang/snappy"
)

func RunBuilderExitRequestTest(t *testing.T, config string) {
	require.NoError(t, utils.SetConfig(t, config))
	testFolders, testsFolderPath := utils.TestFolders(t, config, version.String(version.Gloas), "operations/builder_exit_request/pyspec_tests")
	for _, folder := range testFolders {
		t.Run(folder.Name(), func(t *testing.T) {
			folderPath := path.Join(testsFolderPath, folder.Name())
			file, err := util.BazelFileBytes(folderPath, "builder_exit_request.ssz_snappy")
			require.NoError(t, err)
			ssz, err := snappy.Decode(nil /* dst */, file)
			require.NoError(t, err, "Failed to decompress")
			req := &enginev1.BuilderExitRequest{}
			require.NoError(t, req.UnmarshalSSZ(ssz))
			blk, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockGloas())
			require.NoError(t, err)
			common.RunBlockOperationTest(t, folderPath, blk, sszToState, func(ctx context.Context, s state.BeaconState, _ interfaces.ReadOnlySignedBeaconBlock) (state.BeaconState, error) {
				return s, gloas.ProcessBuilderExitRequests(ctx, s, []*enginev1.BuilderExitRequest{req})
			})
		})
	}
}
