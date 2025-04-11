package beacon

import (
	"os"
	"testing"

	"github.com/OffchainLabs/prysm/v6/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v6/config/params"
)

func TestMain(m *testing.M) {
	// Use minimal config to reduce test setup time.
	prevConfig := params.BeaconConfig().Copy()
	defer params.OverrideBeaconConfig(prevConfig)
	params.OverrideBeaconConfig(params.MinimalSpecConfig())

	resetFlags := flags.Get()
	flags.Init(&flags.GlobalFlags{
		MinimumSyncPeers: 30,
	})
	defer func() {
		flags.Init(resetFlags)
	}()

	os.Exit(m.Run())
}
