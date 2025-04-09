package fork

import (
	"path"
	"testing"

	"github.com/golang/snappy"
	"github.com/google/go-cmp/cmp"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/electra"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/helpers"
	state_native "github.com/prysmaticlabs/prysm/v5/beacon-chain/state/state-native"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/spectest/utils"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
)

// RunUpgradeToElectra is a helper function that runs Electra's fork spec tests.
// It unmarshals a pre- and post-state to check `UpgradeToElectra` comply with spec implementation.
func RunUpgradeToElectra(t *testing.T, config string) {
	require.NoError(t, utils.SetConfig(t, config))

	testFolders, testsFolderPath := utils.TestFolders(t, config, "electra", "fork/fork/pyspec_tests")
	for _, folder := range testFolders {
		t.Run(folder.Name(), func(t *testing.T) {
			helpers.ClearCache()
			folderPath := path.Join(testsFolderPath, folder.Name())

			preStateFile, err := util.BazelFileBytes(path.Join(folderPath, "pre.ssz_snappy"))
			require.NoError(t, err)
			preStateSSZ, err := snappy.Decode(nil /* dst */, preStateFile)
			require.NoError(t, err, "Failed to decompress")
			preStateBase := &ethpb.BeaconStateDeneb{}
			if err := preStateBase.UnmarshalSSZ(preStateSSZ); err != nil {
				t.Fatalf("Failed to unmarshal: %v", err)
			}
			preState, err := state_native.InitializeFromProtoDeneb(preStateBase)
			require.NoError(t, err)
			postState, err := electra.UpgradeToElectra(preState)
			require.NoError(t, err)
			postStateFromFunction, err := state_native.ProtobufBeaconStateElectra(postState.ToProtoUnsafe())
			require.NoError(t, err)

			postStateFile, err := util.BazelFileBytes(path.Join(folderPath, "post.ssz_snappy"))
			require.NoError(t, err)
			postStateSSZ, err := snappy.Decode(nil /* dst */, postStateFile)
			require.NoError(t, err, "Failed to decompress")
			postStateFromFile := &ethpb.BeaconStateElectra{}
			if err := postStateFromFile.UnmarshalSSZ(postStateSSZ); err != nil {
				t.Fatalf("Failed to unmarshal: %v", err)
			}

			if !proto.Equal(postStateFromFile, postStateFromFunction) {
				t.Log(cmp.Diff(postStateFromFile, postStateFromFunction, protocmp.Transform()))
				t.Fatal("Post state does not match expected")
			}
		})
	}
}
