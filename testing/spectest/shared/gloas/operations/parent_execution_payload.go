package operations

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
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

type parentExecutionPayloadMeta struct {
	BlocksCount int `json:"blocks_count"`
}

func RunParentExecutionPayloadTest(t *testing.T, config string) {
	require.NoError(t, utils.SetConfig(t, config))
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.BlobSchedule = []params.BlobScheduleEntry{{MaxBlobsPerBlock: 9}}
	cfg.FuluForkEpoch = 0
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)
	testFolders, testsFolderPath := utils.TestFolders(t, config, "gloas", "operations/parent_execution_payload/pyspec_tests")
	if len(testFolders) == 0 {
		t.Fatalf("No test folders found for %s/%s/%s", config, "gloas", "operations/parent_execution_payload/pyspec_tests")
	}
	for _, folder := range testFolders {
		t.Run(folder.Name(), func(t *testing.T) {
			helpers.ClearCache()

			preBeaconStateFile, err := util.BazelFileBytes(testsFolderPath, folder.Name(), "pre.ssz_snappy")
			require.NoError(t, err)
			preBeaconStateSSZ, err := snappy.Decode(nil, preBeaconStateFile)
			require.NoError(t, err, "Failed to decompress")
			beaconStateBase := &ethpb.BeaconStateGloas{}
			require.NoError(t, beaconStateBase.UnmarshalSSZ(preBeaconStateSSZ), "Failed to unmarshal")
			beaconState, err := state_native.InitializeFromProtoGloas(beaconStateBase)
			require.NoError(t, err)

			file, err := util.BazelFileBytes(testsFolderPath, folder.Name(), "meta.yaml")
			require.NoError(t, err)
			meta := &parentExecutionPayloadMeta{}
			require.NoError(t, utils.UnmarshalYaml(file, meta), "Failed to Unmarshal")

			var transitionError error
			var processedState state.BeaconState
			var ok bool
			for i := 0; i < meta.BlocksCount; i++ {
				filename := fmt.Sprintf("blocks_%d.ssz_snappy", i)
				blockFile, err := util.BazelFileBytes(testsFolderPath, folder.Name(), filename)
				require.NoError(t, err)
				blockSSZ, err := snappy.Decode(nil, blockFile)
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
				postBeaconStateSSZ, err := snappy.Decode(nil, postBeaconStateFile)
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
				if transitionError == nil {
					t.Fatal("Did not fail when expected")
				}
				t.Logf("Expected failure; failure reason = %v", transitionError)
			}
		})
	}
}
