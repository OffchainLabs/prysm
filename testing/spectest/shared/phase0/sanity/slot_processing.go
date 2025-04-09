package sanity

import (
	"context"
	"strconv"
	"testing"

	"github.com/golang/snappy"
	"github.com/google/go-cmp/cmp"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/transition"
	state_native "github.com/prysmaticlabs/prysm/v5/beacon-chain/state/state-native"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/spectest/utils"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
)

func init() {
	transition.SkipSlotCache.Disable()
}

// RunSlotProcessingTests executes "sanity/slots" tests.
func RunSlotProcessingTests(t *testing.T, config string) {
	require.NoError(t, utils.SetConfig(t, config))

	testFolders, testsFolderPath := utils.TestFolders(t, config, "phase0", "sanity/slots/pyspec_tests")
	if len(testFolders) == 0 {
		t.Fatalf("No test folders found for %s/%s/%s", config, "phase0", "sanity/slots/pyspec_tests")
	}
	for _, folder := range testFolders {
		t.Run(folder.Name(), func(t *testing.T) {
			preBeaconStateFile, err := util.BazelFileBytes(testsFolderPath, folder.Name(), "pre.ssz_snappy")
			require.NoError(t, err)
			preBeaconStateSSZ, err := snappy.Decode(nil /* dst */, preBeaconStateFile)
			require.NoError(t, err, "Failed to decompress")
			base := &ethpb.BeaconState{}
			require.NoError(t, base.UnmarshalSSZ(preBeaconStateSSZ), "Failed to unmarshal")
			beaconState, err := state_native.InitializeFromProtoPhase0(base)
			require.NoError(t, err)

			file, err := util.BazelFileBytes(testsFolderPath, folder.Name(), "slots.yaml")
			require.NoError(t, err)
			fileStr := string(file)
			slotsCount, err := strconv.ParseUint(fileStr[:len(fileStr)-5], 10, 64)
			require.NoError(t, err)

			postBeaconStateFile, err := util.BazelFileBytes(testsFolderPath, folder.Name(), "post.ssz_snappy")
			require.NoError(t, err)
			postBeaconStateSSZ, err := snappy.Decode(nil /* dst */, postBeaconStateFile)
			require.NoError(t, err, "Failed to decompress")
			postBeaconState := &ethpb.BeaconState{}
			require.NoError(t, postBeaconState.UnmarshalSSZ(postBeaconStateSSZ), "Failed to unmarshal")
			postState, err := transition.ProcessSlots(context.Background(), beaconState, beaconState.Slot().Add(slotsCount))
			require.NoError(t, err)

			pbState, err := state_native.ProtobufBeaconStatePhase0(postState.ToProto())
			require.NoError(t, err)
			if !proto.Equal(pbState, postBeaconState) {
				t.Log(cmp.Diff(postBeaconState, pbState, protocmp.Transform()))
				t.Fatal("Post state does not match expected. Diff between states")
			}
		})
	}
}
