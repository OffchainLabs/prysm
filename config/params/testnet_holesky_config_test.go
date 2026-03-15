package params_test

import (
	"path"
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	testutil "github.com/OffchainLabs/prysm/v7/testing/util"
)

func TestHoleskyConfigMatchesUpstreamYaml(t *testing.T) {
	presetFPs := presetsFilePath(t, "mainnet")
	mn, err := params.ByName(params.MainnetName)
	require.NoError(t, err)
	cfg := mn.Copy()
	for _, fp := range presetFPs {
		cfg, err = params.UnmarshalConfigFile(fp, cfg)
		require.NoError(t, err)
	}
	repoRoot, err := testutil.RepoRoot()
	require.NoError(t, err)
	fPath := path.Join(repoRoot, "external", "holesky_testnet")
	configFP := path.Join(fPath, "metadata", "config.yaml")
	pcfg, err := params.UnmarshalConfigFile(configFP, nil)
	require.NoError(t, err)
	fields := fieldsFromYamls(t, append(presetFPs, configFP))
	assertYamlFieldsMatch(t, "holesky", fields, pcfg, params.HoleskyConfig())
}
