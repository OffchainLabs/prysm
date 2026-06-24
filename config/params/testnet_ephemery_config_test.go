package params_test

import (
	"path"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/bazelbuild/rules_go/go/tools/bazel"
)

func TestEphemeryConfigMatchesUpstreamYaml(t *testing.T) {
	presetFPs := presetsFilePath(t, "mainnet")
	mn, err := params.ByName(params.MainnetName)
	require.NoError(t, err)
	cfg := mn.Copy()
	for _, fp := range presetFPs {
		cfg, err = params.UnmarshalConfigFile(fp, cfg)
		require.NoError(t, err)
	}
	fPath, err := bazel.Runfile("external/ephemery_testnet")
	require.NoError(t, err)
	configFP := path.Join(fPath, "cl-config-genesis-zero.yaml")
	pcfg, err := params.UnmarshalConfigFile(configFP, nil)
	require.NoError(t, err)
	
	// Calculate the current iteration using the same logic as EphemeryConfig()
	genesisZero := int64(1393527600)
	ephemeryResetPeriod := uint64(2419200)
	now := time.Now().Unix()
	difference := now - genesisZero
	iteration := difference / int64(ephemeryResetPeriod)
	genesisDelay := uint64(600)
	genesisZeroChainID := uint64(39438000)
	
	// Adjust the parsed config to use current iteration values
	pcfg.MinGenesisTime = uint64(genesisZero+iteration*int64(ephemeryResetPeriod)) + genesisDelay
	pcfg.DepositChainID = uint64(iteration) + genesisZeroChainID
	pcfg.DepositNetworkID = uint64(iteration) + genesisZeroChainID
	
	fields := fieldsFromYamls(t, append(presetFPs, configFP))
	assertYamlFieldsMatch(t, "ephemery", fields, pcfg, params.EphemeryConfig())
}
