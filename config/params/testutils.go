//go:build !develop

package params

import "testing"

const (
	EnvNameOverrideAccept = "PRYSM_API_OVERRIDE_ACCEPT"
)

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
