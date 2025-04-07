package fork

import (
	"path"
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/capella"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/helpers"
	state_native "github.com/OffchainLabs/prysm/v6/beacon-chain/state/state-native"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/spectest/utils"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/golang/snappy"
	"google.golang.org/protobuf/proto"
)

// RunUpgradeToCapella is a helper function that runs capella's fork spec tests.
// It unmarshals a pre- and post-state to check `UpgradeToCapella` comply with spec implementation.
func RunUpgradeToCapella(t *testing.T, config string) {
	require.NoError(t, utils.SetConfig(t, config))

	testFolders, testsFolderPath := utils.TestFolders(t, config, "capella", "fork/fork/pyspec_tests")
	if len(testFolders) == 0 {
		t.Fatalf("No test folders found for %s/%s/%s", config, "capella", "fork/fork/pyspec_tests")
	}
	for _, folder := range testFolders {
		t.Run(folder.Name(), func(t *testing.T) {
			helpers.ClearCache()
			folderPath := path.Join(testsFolderPath, folder.Name())

			preStateFile, err := util.BazelFileBytes(path.Join(folderPath, "pre.ssz_snappy"))
			require.NoError(t, err)
			preStateSSZ, err := snappy.Decode(nil /* dst */, preStateFile)
			require.NoError(t, err, "Failed to decompress")
			preStateBase := &ethpb.BeaconStateBellatrix{}
			if err := preStateBase.UnmarshalSSZ(preStateSSZ); err != nil {
				t.Fatalf("Failed to unmarshal: %v", err)
			}
			preState, err := state_native.InitializeFromProtoBellatrix(preStateBase)
			require.NoError(t, err)
			postState, err := capella.UpgradeToCapella(preState)
			require.NoError(t, err)
			postStateFromFunction, err := state_native.ProtobufBeaconStateCapella(postState.ToProtoUnsafe())
			require.NoError(t, err)

			postStateFile, err := util.BazelFileBytes(path.Join(folderPath, "post.ssz_snappy"))
			require.NoError(t, err)
			postStateSSZ, err := snappy.Decode(nil /* dst */, postStateFile)
			require.NoError(t, err, "Failed to decompress")
			postStateFromFile := &ethpb.BeaconStateCapella{}
			if err := postStateFromFile.UnmarshalSSZ(postStateSSZ); err != nil {
				t.Fatalf("Failed to unmarshal: %v", err)
			}

			if !proto.Equal(postStateFromFile, postStateFromFunction) {
				t.Fatal("Post state does not match expected")
			}
		})
	}
}
