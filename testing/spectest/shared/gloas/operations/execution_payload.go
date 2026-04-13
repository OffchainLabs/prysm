package operations

import (
	"context"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/gloas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/spectest/utils"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/bazelbuild/rules_go/go/tools/bazel"
	"github.com/golang/snappy"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
)

type ExecutionConfig struct {
	Valid bool `json:"execution_valid"`
}

func sszToSignedExecutionPayloadEnvelope(b []byte) (interfaces.ROSignedExecutionPayloadEnvelope, error) {
	envelope := &ethpb.SignedExecutionPayloadEnvelope{}
	if err := envelope.UnmarshalSSZ(b); err != nil {
		return nil, err
	}
	return blocks.WrappedROSignedExecutionPayloadEnvelope(envelope)
}

func RunExecutionPayloadTest(t *testing.T, config string) {
	require.NoError(t, utils.SetConfig(t, config))
	cfg := params.BeaconConfig()
	params.SetGenesisFork(t, cfg, version.Fulu)
	testFolders, testsFolderPath := utils.TestFolders(t, config, "gloas", "operations/execution_payload/pyspec_tests")
	if len(testFolders) == 0 {
		t.Fatalf("No test folders found for %s/%s/%s", config, "gloas", "operations/execution_payload/pyspec_tests")
	}
	for _, folder := range testFolders {
		t.Run(folder.Name(), func(t *testing.T) {
			helpers.ClearCache()

			// Check if signed_envelope.ssz_snappy exists, skip if not
			_, err := bazel.Runfile(path.Join(testsFolderPath, folder.Name(), "signed_envelope.ssz_snappy"))
			if err != nil && strings.Contains(err.Error(), "could not locate file") {
				t.Skipf("Skipping test %s: signed_envelope.ssz_snappy not found", folder.Name())
				return
			}

			// Read the signed execution payload envelope
			envelopeFile, err := util.BazelFileBytes(testsFolderPath, folder.Name(), "signed_envelope.ssz_snappy")
			require.NoError(t, err)
			envelopeSSZ, err := snappy.Decode(nil /* dst */, envelopeFile)
			require.NoError(t, err, "Failed to decompress envelope")
			signedEnvelope, err := sszToSignedExecutionPayloadEnvelope(envelopeSSZ)
			require.NoError(t, err, "Failed to unmarshal signed envelope")

			preBeaconStateFile, err := util.BazelFileBytes(testsFolderPath, folder.Name(), "pre.ssz_snappy")
			require.NoError(t, err)
			preBeaconStateSSZ, err := snappy.Decode(nil /* dst */, preBeaconStateFile)
			require.NoError(t, err, "Failed to decompress")
			preBeaconState, err := sszToState(preBeaconStateSSZ)
			require.NoError(t, err)

			postSSZFilepath, err := bazel.Runfile(path.Join(testsFolderPath, folder.Name(), "post.ssz_snappy"))
			postSSZExists := true
			if err != nil && strings.Contains(err.Error(), "could not locate file") {
				postSSZExists = false
			} else {
				require.NoError(t, err)
			}

			file, err := util.BazelFileBytes(testsFolderPath, folder.Name(), "execution.yaml")
			require.NoError(t, err)
			config := &ExecutionConfig{}
			require.NoError(t, utils.UnmarshalYaml(file, config), "Failed to Unmarshal")
			if !config.Valid {
				t.Skip("Skipping invalid execution engine test as it's never supported")
			}

			err = gloas.VerifyExecutionPayloadEnvelope(context.Background(), preBeaconState, signedEnvelope)
			if postSSZExists {
				require.NoError(t, err)
				comparePostState(t, postSSZFilepath, preBeaconState)
			} else if config.Valid {
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

func comparePostState(t *testing.T, postSSZFilepath string, want state.BeaconState) {
	postBeaconStateFile, err := os.ReadFile(postSSZFilepath) // #nosec G304
	require.NoError(t, err)
	postBeaconStateSSZ, err := snappy.Decode(nil /* dst */, postBeaconStateFile)
	require.NoError(t, err, "Failed to decompress")
	postBeaconState, err := sszToState(postBeaconStateSSZ)
	require.NoError(t, err)
	postBeaconStatePb, ok := postBeaconState.ToProtoUnsafe().(proto.Message)
	require.Equal(t, true, ok, "post beacon state did not return a proto.Message")
	pbState, ok := want.ToProtoUnsafe().(proto.Message)
	require.Equal(t, true, ok, "beacon state did not return a proto.Message")

	if !proto.Equal(postBeaconStatePb, pbState) {
		diff := cmp.Diff(pbState, postBeaconStatePb, protocmp.Transform())
		t.Fatalf("Post state does not match expected state, diff: %s", diff)
	}
}
