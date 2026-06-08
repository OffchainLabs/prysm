package mainnet

import (
	"os"
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/config/params"
)

func TestMain(m *testing.M) {
	featureCfg := *features.Get()
	featureCfg.EnableProgressiveSSZ = true
	features.Init(&featureCfg)

	prevConfig := params.BeaconConfig().Copy()
	defer params.OverrideBeaconConfig(prevConfig)
	c := params.BeaconConfig().Copy()
	c.MinGenesisActiveValidatorCount = 16384
	params.OverrideBeaconConfig(c)

	os.Exit(m.Run())
}
