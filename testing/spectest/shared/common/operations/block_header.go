package operations

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/blocks"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/spectest/utils"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/golang/snappy"
)

type SSZToBlock func([]byte) (interfaces.SignedBeaconBlock, error)

func RunBlockHeaderTest(t *testing.T, config string, fork string, sszToBlock SSZToBlock, sszToState SSZToState) {
	require.NoError(t, utils.SetConfig(t, config))
	testFolders, testsFolderPath := utils.TestFolders(t, config, fork, "operations/block_header/pyspec_tests")
	if len(testFolders) == 0 {
		t.Fatalf("No test folders found for %s/%s/%s", config, fork, "operations/block_header/pyspec_tests")
	}

	for _, folder := range testFolders {
		t.Run(folder.Name(), func(t *testing.T) {
			helpers.ClearCache()

			blockFile, err := util.BazelFileBytes(testsFolderPath, folder.Name(), "block.ssz_snappy")
			require.NoError(t, err)
			blockSSZ, err := snappy.Decode(nil /* dst */, blockFile)
			require.NoError(t, err, "Failed to decompress")
			block, err := sszToBlock(blockSSZ)
			require.NoError(t, err, "Failed to unmarshal")

			preBeaconStateFile, err := util.BazelFileBytes(testsFolderPath, folder.Name(), "pre.ssz_snappy")
			require.NoError(t, err)
			preBeaconStateSSZ, err := snappy.Decode(nil /* dst */, preBeaconStateFile)
			require.NoError(t, err, "Failed to decompress")
			preBeaconState, err := sszToState(preBeaconStateSSZ)
			require.NoError(t, err)

			// If the post.ssz is not present, it means the test should fail on our end.
			postSSZFilepath, err := filepath.Abs(path.Join(testsFolderPath, folder.Name(), "post.ssz_snappy"))
			require.NoError(t, err)
			postSSZExists := true
			if _, err := os.Stat(postSSZFilepath); os.IsNotExist(err) {
				postSSZExists = false
			}

			// Spectest blocks are not signed, so we'll call NoVerify to skip sig verification.
			bodyRoot, err := block.Block().Body().HashTreeRoot()
			require.NoError(t, err)
			pr := block.Block().ParentRoot()
			_, err = blocks.ProcessBlockHeaderNoVerify(context.Background(), preBeaconState, block.Block().Slot(), block.Block().ProposerIndex(), pr[:], bodyRoot[:])
			if postSSZExists {
				require.NoError(t, err)
				comparePostState(t, postSSZFilepath, sszToState, preBeaconState)
			} else {
				// Note: This doesn't test anything worthwhile. It essentially tests
				// that *any* error has occurred, not any specific error.
				if err == nil {
					t.Fatal("Did not fail when expected")
				}
				t.Logf("Expected failure; failure reason = %v", err)
				return
			}
		})
	}
}
