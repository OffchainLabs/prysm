package operations

import (
	"context"
	"path"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/gloas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	common "github.com/OffchainLabs/prysm/v7/testing/spectest/shared/common/operations"
	"github.com/OffchainLabs/prysm/v7/testing/spectest/utils"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/golang/snappy"
)

func RunParentExecutionPayloadTest(t *testing.T, config string) {
	require.NoError(t, utils.SetConfig(t, config))
	testFolders, testsFolderPath := utils.TestFolders(t, config, version.String(version.Gloas), "operations/parent_execution_payload/pyspec_tests")
	if len(testFolders) == 0 {
		t.Fatalf("No test folders found for %s/%s/%s", config, version.String(version.Gloas), "operations/parent_execution_payload/pyspec_tests")
	}
	for _, folder := range testFolders {
		t.Run(folder.Name(), func(t *testing.T) {
			helpers.ClearCache()
			folderPath := path.Join(testsFolderPath, folder.Name())
			blockFile, err := util.BazelFileBytes(folderPath, "block.ssz_snappy")
			require.NoError(t, err)
			blockSSZ, err := snappy.Decode(nil, blockFile)
			require.NoError(t, err, "Failed to decompress")
			blk, err := sszToBlock(blockSSZ)
			require.NoError(t, err)

			common.RunBlockOperationTest(t, folderPath, blk, sszToState, func(ctx context.Context, s state.BeaconState, b interfaces.ReadOnlySignedBeaconBlock) (state.BeaconState, error) {
				if err := gloas.ProcessParentExecutionPayload(ctx, s, b.Block()); err != nil {
					return nil, err
				}
				return s, nil
			})
		})
	}
}
