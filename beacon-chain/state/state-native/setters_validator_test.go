package state_native_test

import (
	"testing"

	state_native "github.com/OffchainLabs/prysm/v6/beacon-chain/state/state-native"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func BenchmarkAppendBalance(b *testing.B) {
	st, err := state_native.InitializeFromProtoPhase0(&ethpb.BeaconState{})
	require.NoError(b, err)

	max := uint64(16777216)
	for i := uint64(0); i < max-2; i++ {
		require.NoError(b, st.AppendBalance(i))
	}

	ref := st.Copy()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
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
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		require.NoError(b, ref.AppendInactivityScore(uint64(i)))
		ref = st.Copy()
	}
}
