package epoch

import (
	"testing"

	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/fuzz"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	gofuzz "github.com/google/gofuzz"
)

func TestFuzzFinalUpdates_10000(t *testing.T) {
	fuzzer := gofuzz.NewWithSeed(0)
	base := &ethpb.BeaconState{}

	for i := range 10000 {
		fuzzer.Fuzz(base)
		s, err := state_native.InitializeFromProtoUnsafePhase0(base)
		require.NoError(t, err)
		_, err = ProcessFinalUpdates(s)
		_ = err
		fuzz.FreeMemory(i)
	}
}
