package params_test

import (
	"path"
	"testing"

	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/bazelbuild/rules_go/go/tools/bazel"
)

func TestHoodiConfigMatchesUpstreamYaml(t *testing.T) {
	presetFPs := presetsFilePath(t, "mainnet")
	mn, err := params.ByName(params.MainnetName)
	require.NoError(t, err)
	cfg := mn.Copy()
	for _, fp := range presetFPs {
		cfg, err = params.UnmarshalConfigFile(fp, cfg)
		require.NoError(t, err)
	}
	fPath, err := bazel.Runfile("external/hoodi_testnet")
	require.NoError(t, err)
	configFP := path.Join(fPath, "metadata", "config.yaml")
	pcfg, err := params.UnmarshalConfigFile(configFP, nil)
	require.NoError(t, err)
	fields := fieldsFromYamls(t, append(presetFPs, configFP))
	assertYamlFieldsMatch(t, "hoodi", fields, pcfg, params.HoodiConfig())
}
