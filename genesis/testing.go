package genesis

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
)

// StoreDuringTest temporarily replaces the package level GenesisData with the provided GenesisData
func StoreDuringTest(t *testing.T, gd GenesisData) {
	prev := getPkgVar()
	t.Cleanup(func() {
		setPkgVar(prev, prev.initialized)
	})
	setPkgVar(gd, true)
}

// StoreEmbeddedDuringTest sets the named embedded genesis file as the genesis data for the lifecycle of the current test.
func StoreEmbeddedDuringTest(t *testing.T, name string) {
	gd, ok := embeddedGenesisData[name]
	if !ok {
		t.Fatalf("embedded genesis data for %s not found", name)
	}
	StoreDuringTest(t, gd)
}

// StoreStateDuringTest creates and stores genesis data from a beacon state for the duration of a test.
// This is essential for testing components that depend on genesis information being globally available,
// The function automatically cleans up after the test completes, restoring the previous
// genesis state to prevent test interference. Without this setup, many blockchain
// components would fail during testing due to uninitialized genesis data.
func StoreStateDuringTest(t *testing.T, st state.BeaconState) {
	gd, err := newGenesisData(st, "testdata")
	if err != nil {
		t.Fatalf("failed to create genesis data: %v", err)
	}
	StoreDuringTest(t, gd)
}
