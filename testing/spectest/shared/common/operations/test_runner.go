package operations

import (
	"context"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/build/bazel"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/golang/snappy"
)

type blockWithSSZObject func([]byte) (interfaces.SignedBeaconBlock, error)
type BlockOperation func(context.Context, state.BeaconState, interfaces.ReadOnlySignedBeaconBlock) (state.BeaconState, error)
type ProcessBlock func(context.Context, state.BeaconState, interfaces.ReadOnlyBeaconBlock) (state.BeaconState, error)
type SSZToState func([]byte) (state.BeaconState, error)

// RunBlockOperationTest takes in the prestate and the beacon block body, processes it through the
// passed in block operation function and checks the post state with the expected post state.
func RunBlockOperationTest(
	t *testing.T,
	folderPath string,
	wsb interfaces.SignedBeaconBlock,
	sszToState SSZToState,
	operationFn BlockOperation,
) {
	preBeaconStateFile, err := util.BazelFileBytes(path.Join(folderPath, "pre.ssz_snappy"))
	require.NoError(t, err)
	preBeaconStateSSZ, err := snappy.Decode(nil /* dst */, preBeaconStateFile)
	require.NoError(t, err, "Failed to decompress")
	preState, err := sszToState(preBeaconStateSSZ)
	require.NoError(t, err)

	// If the post.ssz is not present, it means the test should fail on our end.
	postSSZFilepath, err := bazel.Runfile(path.Join(folderPath, "post.ssz_snappy"))
	postSSZExists := true
	if err != nil && strings.Contains(err.Error(), "could not locate file") {
		postSSZExists = false
	} else if err != nil {
		t.Fatal(err)
	}

	helpers.ClearCache()
	_, err = operationFn(context.Background(), preState, wsb)
	if postSSZExists {
		require.NoError(t, err)
		comparePostState(t, postSSZFilepath, sszToState, preState)
	} else {
		// Note: This doesn't test anything worthwhile. It essentially tests
		// that *any* error has occurred, not any specific error.
		if err == nil {
			t.Fatal("Did not fail when expected")
		}
		t.Logf("Expected failure; failure reason = %v", err)
		return
	}
}

func comparePostState(t *testing.T, postSSZFilepath string, sszToState SSZToState, want state.BeaconState) {
	postBeaconStateFile, err := os.ReadFile(postSSZFilepath) // #nosec G304
	require.NoError(t, err)
	postBeaconStateSSZ, err := snappy.Decode(nil /* dst */, postBeaconStateFile)
	require.NoError(t, err, "Failed to decompress")
	postBeaconState, err := sszToState(postBeaconStateSSZ)
	require.NoError(t, err)
	require.DeepSSZEqual(t, postBeaconState.ToProtoUnsafe(), want.ToProtoUnsafe(), "Post state does not match expected")
}
