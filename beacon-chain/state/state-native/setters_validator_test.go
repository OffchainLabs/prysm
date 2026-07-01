package state_native_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func BenchmarkAppendBalance(b *testing.B) {
	st, err := state_native.InitializeFromProtoPhase0(&ethpb.BeaconState{})
	require.NoError(b, err)

	max := uint64(16777216)
	for i := uint64(0); i < max-2; i++ {
		require.NoError(b, st.AppendBalance(i))
	}

	ref := st.Copy()

	for i := 0; b.Loop(); i++ {
		require.NoError(b, ref.AppendBalance(uint64(i)))
		ref = st.Copy()
	}
}

func BenchmarkAppendInactivityScore(b *testing.B) {
	st, err := state_native.InitializeFromProtoCapella(&ethpb.BeaconStateCapella{})
	require.NoError(b, err)

	max := uint64(16777216)
	for i := uint64(0); i < max-2; i++ {
		require.NoError(b, st.AppendInactivityScore(i))
	}

	ref := st.Copy()

	for i := 0; b.Loop(); i++ {
		require.NoError(b, ref.AppendInactivityScore(uint64(i)))
		ref = st.Copy()
	}
}

// BenchmarkApplyToEveryValidator measures the per-validator cost of iterating the
// registry through the ReadOnlyValidator wrapper.
func BenchmarkApplyToEveryValidator(b *testing.B) {
	const n = 2_300_000 // ~ number of validators on mainnet at the time of writing

	vals := make([]*ethpb.Validator, n)
	for i := range vals {
		pk := make([]byte, 48)
		wc := make([]byte, 32)
		pk[0] = byte(i)
		vals[i] = &ethpb.Validator{
			PublicKey:             pk,
			WithdrawalCredentials: wc,
			EffectiveBalance:      32_000_000_000,
			ExitEpoch:             100,
			ActivationEpoch:       1,
		}
	}
	st, err := state_native.InitializeFromProtoUnsafeDeneb(&ethpb.BeaconStateDeneb{Validators: vals})
	require.NoError(b, err)

	b.ReportAllocs()
	for b.Loop() {
		// Returning nil signals "no change", isolating the per-validator wrapper cost
		// from the validator-update path.
		require.NoError(b, st.ApplyToEveryValidator(func(_ int, v state.ReadOnlyValidator) (*ethpb.Validator, error) {
			_ = v.EffectiveBalance()
			return nil, nil
		}))
	}
}
