package sanity

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"
	"testing"

	coreblocks "github.com/OffchainLabs/prysm/v7/beacon-chain/core/blocks"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/spectest/utils"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/bazelbuild/rules_go/go/tools/bazel"
	"github.com/golang/snappy"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
)

func init() {
	transition.SkipSlotCache.Disable()
}

// Config is the schema for meta.yaml in sanity/blocks tests.
type Config struct {
	BlocksCount int `json:"blocks_count"`
}

// RunBlockProcessingTest executes "sanity/blocks" tests.
func RunBlockProcessingTest(t *testing.T, config, folderPath string) {
	require.NoError(t, utils.SetConfig(t, config))

	testFolders, testsFolderPath := utils.TestFolders(t, config, "gloas", folderPath)
	for _, folder := range testFolders {
		t.Run(folder.Name(), func(t *testing.T) {
			params.SetupTestConfigCleanup(t)
			cfg := params.BeaconConfig().Copy()
			cfg.BlobSchedule = []params.BlobScheduleEntry{{MaxBlobsPerBlock: 9}}
			cfg.FuluForkEpoch = 0
			cfg.GloasForkEpoch = 0
			params.OverrideBeaconConfig(cfg)

			helpers.ClearCache()
			preBeaconStateFile, err := util.BazelFileBytes(testsFolderPath, folder.Name(), "pre.ssz_snappy")
			require.NoError(t, err)
			preBeaconStateSSZ, err := snappy.Decode(nil /* dst */, preBeaconStateFile)
			require.NoError(t, err, "Failed to decompress")
			beaconStateBase := &ethpb.BeaconStateGloas{}
			require.NoError(t, beaconStateBase.UnmarshalSSZ(preBeaconStateSSZ), "Failed to unmarshal")
			beaconState, err := state_native.InitializeFromProtoGloas(beaconStateBase)
			require.NoError(t, err)

			// If the pre-state is a genesis state, verify that the genesis block
			// Prysm would construct for this state matches the state's
			// latest_block_header (which per spec records the genesis block's body root).
			// Some spec tests reuse a slot-0 pre-state but mutate latest_execution_payload_bid
			// to simulate a post-payload scenario; skip those since their bid no longer
			// matches the genesis block encoded in latest_block_header.body_root.
			if beaconState.Slot() == params.BeaconConfig().GenesisSlot {
				bid, err := beaconState.LatestExecutionPayloadBid()
				require.NoError(t, err)
				zeroHash := params.BeaconConfig().ZeroHash
				isGenesisBid := bid == nil || bid.BlockHash() == zeroHash
				if isGenesisBid {
					genesisBlock, err := coreblocks.NewGenesisBlockForState(context.Background(), beaconState)
					require.NoError(t, err)
					genesisBodyRoot, err := genesisBlock.Block().Body().HashTreeRoot()
					require.NoError(t, err)
					expectedBodyRoot := bytesutil.ToBytes32(beaconState.LatestBlockHeader().BodyRoot)
					if genesisBodyRoot != expectedBodyRoot {
						t.Fatalf("genesis block body root does not match pre-state latest_block_header.body_root: got %#x, want %#x", genesisBodyRoot, expectedBodyRoot)
					}
				}
			}

			file, err := util.BazelFileBytes(testsFolderPath, folder.Name(), "meta.yaml")
			require.NoError(t, err)

			metaYaml := &Config{}
			require.NoError(t, utils.UnmarshalYaml(file, metaYaml), "Failed to Unmarshal")

			var transitionError error
			var processedState state.BeaconState
			var ok bool
			for i := 0; i < metaYaml.BlocksCount; i++ {
				filename := fmt.Sprintf("blocks_%d.ssz_snappy", i)
				blockFile, err := util.BazelFileBytes(testsFolderPath, folder.Name(), filename)
				require.NoError(t, err)
				blockSSZ, err := snappy.Decode(nil /* dst */, blockFile)
				require.NoError(t, err, "Failed to decompress")
				block := &ethpb.SignedBeaconBlockGloas{}
				require.NoError(t, block.UnmarshalSSZ(blockSSZ), "Failed to unmarshal")
				wsb, err := blocks.NewSignedBeaconBlock(block)
				require.NoError(t, err)
				processedState, transitionError = transition.ExecuteStateTransition(context.Background(), beaconState, wsb)
				if transitionError != nil {
					break
				}
				beaconState, ok = processedState.(*state_native.BeaconState)
				require.Equal(t, true, ok)
			}

			// If the post.ssz is not present, it means the test should fail on our end.
			postSSZFilepath, readError := bazel.Runfile(path.Join(testsFolderPath, folder.Name(), "post.ssz_snappy"))
			postSSZExists := true
			if readError != nil && strings.Contains(readError.Error(), "could not locate file") {
				postSSZExists = false
			} else if readError != nil {
				t.Fatal(readError)
			}

			if postSSZExists {
				if transitionError != nil {
					t.Errorf("Unexpected error: %v", transitionError)
				}

				postBeaconStateFile, err := os.ReadFile(postSSZFilepath) // #nosec G304
				require.NoError(t, err)
				postBeaconStateSSZ, err := snappy.Decode(nil /* dst */, postBeaconStateFile)
				require.NoError(t, err, "Failed to decompress")

				postBeaconState := &ethpb.BeaconStateGloas{}
				require.NoError(t, postBeaconState.UnmarshalSSZ(postBeaconStateSSZ), "Failed to unmarshal")
				pbState, err := state_native.ProtobufBeaconStateGloas(beaconState.ToProtoUnsafe())
				require.NoError(t, err)
				if !proto.Equal(pbState, postBeaconState) {
					t.Log(cmp.Diff(postBeaconState, pbState, protocmp.Transform()))
					t.Fatal("Post state does not match expected")
				}
			} else {
				// Note: This doesn't test anything worthwhile. It essentially tests
				// that *any* error has occurred, not any specific error.
				if transitionError == nil {
					t.Fatal("Did not fail when expected")
				}
				t.Logf("Expected failure; failure reason = %v", transitionError)
				return
			}
		})
	}
}
