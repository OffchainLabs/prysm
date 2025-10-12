package params

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/runtime/version"
)

const (
	EnvNameOverrideAccept = "PRYSM_API_OVERRIDE_ACCEPT"
)

func SetGenesisFork(t *testing.T, cfg *BeaconChainConfig, fork int) {
	setGenesisUpdateEpochs(cfg, fork)
	OverrideBeaconConfig(cfg)
}

func setGenesisUpdateEpochs(b *BeaconChainConfig, fork int) {
	switch fork {
	case version.Fulu:
		b.FuluForkEpoch = 0
		setGenesisUpdateEpochs(b, version.Electra)
	case version.Electra:
		b.ElectraForkEpoch = 0
		setGenesisUpdateEpochs(b, version.Deneb)
	case version.Deneb:
		b.DenebForkEpoch = 0
		setGenesisUpdateEpochs(b, version.Capella)
	case version.Capella:
		b.CapellaForkEpoch = 0
		setGenesisUpdateEpochs(b, version.Bellatrix)
	case version.Bellatrix:
		b.BellatrixForkEpoch = 0
		setGenesisUpdateEpochs(b, version.Altair)
	case version.Altair:
		b.AltairForkEpoch = 0
	}
}

// SetupTestConfigCleanup preserves configurations allowing to modify them within tests without any
// restrictions, everything is restored after the test.
func SetupTestConfigCleanup(t testing.TB) {
	prevDefaultBeaconConfig := mainnetBeaconConfig.Copy()
	temp := configs.getActive().Copy()
	undo, err := SetActiveWithUndo(temp)
	if err != nil {
		t.Fatal(err)
	}
	prevNetworkCfg := networkConfig.Copy()
	t.Cleanup(func() {
		mainnetBeaconConfig = prevDefaultBeaconConfig
		err = undo()
		if err != nil {
			t.Fatal(err)
		}
		networkConfig = prevNetworkCfg
	})
}

// SetActiveTestCleanup sets an active config,
// and adds a test cleanup hook to revert to the default config after the test completes.
func SetActiveTestCleanup(t *testing.T, cfg *BeaconChainConfig) {
	undo, err := SetActiveWithUndo(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		err = undo()
		if err != nil {
			t.Fatal(err)
		}
	})
}
