package minimal

import (
	"os"
	"testing"

	"github.com/OffchainLabs/prysm/v6/config/params"
)

func TestMain(m *testing.M) {
	prevConfig := params.BeaconConfig().Copy()
	defer params.OverrideBeaconConfig(prevConfig)
	c := params.BeaconConfig().Copy()
	c.MinGenesisActiveValidatorCount = 16384
	params.OverrideBeaconConfig(c)

	os.Exit(m.Run())
}
