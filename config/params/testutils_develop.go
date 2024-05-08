//go:build develop

package params

import "testing"

// SetupTestConfigCleanup preserves configurations allowing to modify them within tests without any
// restrictions, everything is restored after the test. This locks our config when undoing our config
// change in order to satisfy the race detector.
func SetupTestConfigCleanup(t testing.TB) {
	prevDefaultBeaconConfig := mainnetBeaconConfig.Copy()
	temp := configs.getActive().Copy()
	cfgrw.Lock()
	undo, err := SetActiveWithUndo(temp)
	cfgrw.Unlock()
	if err != nil {
		t.Error(err)
	}
	prevNetworkCfg := networkConfig.Copy()
	t.Cleanup(func() {
		mainnetBeaconConfig = prevDefaultBeaconConfig
		cfgrw.Lock()
		err = undo()
		cfgrw.Unlock()
		if err != nil {
			t.Error(err)
		}
		networkConfigLock.Lock()
		networkConfig = prevNetworkCfg
		networkConfigLock.Unlock()
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
