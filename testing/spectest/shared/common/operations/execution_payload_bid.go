package operations

import (
	"context"
	"path"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/gloas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/spectest/utils"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/golang/snappy"
)

func runExecutionPayloadBidTest(t *testing.T, config string, fork string, objName string, block blockWithSSZObject, sszToState SSZToState, operationFn BlockOperation) {
	require.NoError(t, utils.SetConfig(t, config))
	cfg := params.BeaconConfig()
	params.SetGenesisFork(t, cfg, version.Fulu)

	testFolders, testsFolderPath := utils.TestFolders(t, config, fork, "operations/"+objName+"/pyspec_tests")
	if len(testFolders) == 0 {
		t.Fatalf("No test folders found for %s/%s/%s", config, fork, "operations/"+objName+"/pyspec_tests")
	}
	for _, folder := range testFolders {
		t.Run(folder.Name(), func(t *testing.T) {
			helpers.ClearCache()
			folderPath := path.Join(testsFolderPath, folder.Name())
			blockFile, err := util.BazelFileBytes(folderPath, "block.ssz_snappy")
			require.NoError(t, err)
			blockSSZ, err := snappy.Decode(nil /* dst */, blockFile)
			require.NoError(t, err, "Failed to decompress")
			blk, err := block(blockSSZ)
			require.NoError(t, err)
			RunBlockOperationTest(t, folderPath, blk, sszToState, operationFn)
		})
	}
}

func RunExecutionPayloadBidTest(t *testing.T, config string, fork string, block blockWithSSZObject, sszToState SSZToState) {
	runExecutionPayloadBidTest(t, config, fork, "execution_payload_bid", block, sszToState, func(ctx context.Context, s state.BeaconState, b interfaces.ReadOnlySignedBeaconBlock) (state.BeaconState, error) {
		err := gloas.ProcessExecutionPayloadBid(s, b.Block())
		return s, err
	})
}
