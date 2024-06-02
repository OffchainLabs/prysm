package params

import "testing"

const (
	EnvNameOverrideAccept = "PRYSM_API_OVERRIDE_ACCEPT"
)

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
