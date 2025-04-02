package operations

import (
	"path"
	"strings"
	"testing"

	"github.com/bazelbuild/rules_go/go/tools/bazel"
	"github.com/golang/snappy"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/blocks"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/helpers"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/spectest/utils"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
)

type SSZToBlockBody func([]byte) (interfaces.ReadOnlyBeaconBlockBody, error)

func RunExecutionPayloadTest(t *testing.T, config string, fork string, sszToBlockBody SSZToBlockBody, sszToState SSZToState) {
	require.NoError(t, utils.SetConfig(t, config))
	testFolders, testsFolderPath := utils.TestFolders(t, config, fork, "operations/execution_payload/pyspec_tests")
	if len(testFolders) == 0 {
		t.Fatalf("No test folders found for %s/%s/%s", config, fork, "operations/execution_payload/pyspec_tests")
	}
	for _, folder := range testFolders {
		t.Run(folder.Name(), func(t *testing.T) {
			helpers.ClearCache()

			blockBodyFile, err := util.BazelFileBytes(testsFolderPath, folder.Name(), "body.ssz_snappy")
			require.NoError(t, err)
			bodySSZ, err := snappy.Decode(nil /* dst */, blockBodyFile)
			require.NoError(t, err, "Failed to decompress")
			body, err := sszToBlockBody(bodySSZ)
			require.NoError(t, err, "Failed to unmarshal")

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

			err = blocks.ProcessPayload(preBeaconState, body)
			if postSSZExists {
				require.NoError(t, err)
				comparePostState(t, postSSZFilepath, sszToState, preBeaconState)
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

type ExecutionConfig struct {
	Valid bool `json:"execution_valid"`
}
