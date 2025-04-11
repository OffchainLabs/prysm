package params_test

import (
	"path"
	"testing"

	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/bazelbuild/rules_go/go/tools/bazel"
)

func TestMaxRequestBlock(t *testing.T) {
	testCases := []struct {
		epoch            primitives.Epoch
		expectedMaxBlock uint64
		description      string
	}{
		{
			epoch:            primitives.Epoch(params.MainnetDenebForkEpoch - 1), // Assuming the fork epoch is not 0
			expectedMaxBlock: params.MainnetBeaconConfig.MaxRequestBlocks,
		},
		{
			epoch:            primitives.Epoch(params.MainnetDenebForkEpoch),
			expectedMaxBlock: params.MainnetBeaconConfig.MaxRequestBlocksDeneb,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			maxBlocks := params.MaxRequestBlock(tc.epoch)
			if maxBlocks != tc.expectedMaxBlock {
				t.Errorf("For epoch %d, expected max blocks %d, got %d", tc.epoch, tc.expectedMaxBlock, maxBlocks)
			}
		})
	}
}

func TestMainnetConfigMatchesUpstreamYaml(t *testing.T) {
	presetFPs := presetsFilePath(t, "mainnet")
	mn, err := params.ByName(params.MainnetName)
	require.NoError(t, err)
	cfg := mn.Copy()
	for _, fp := range presetFPs {
		cfg, err = params.UnmarshalConfigFile(fp, cfg)
		require.NoError(t, err)
	}
	fPath, err := bazel.Runfile("external/mainnet")
	require.NoError(t, err)
	configFP := path.Join(fPath, "metadata", "config.yaml")
	pcfg, err := params.UnmarshalConfigFile(configFP, nil)
	require.NoError(t, err)
	fields := fieldsFromYamls(t, append(presetFPs, configFP))
	assertYamlFieldsMatch(t, "mainnet", fields, pcfg, params.BeaconConfig())
}
