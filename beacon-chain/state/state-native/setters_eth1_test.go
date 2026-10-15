package state_native_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v7/config/params"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

func BenchmarkAppendEth1DataVotes(b *testing.B) {
	st, err := state_native.InitializeFromProtoPhase0(&ethpb.BeaconState{})
	require.NoError(b, err)

	max := params.BeaconConfig().Eth1DataVotesLength()

	if max < 2 {
		b.Fatalf("Eth1DataVotesLength is less than 2")
	}

	for i := uint64(0); i < max-2; i++ {
		err := st.AppendEth1DataVotes(&ethpb.Eth1Data{
			DepositCount: i,
			DepositRoot:  make([]byte, 64),
			BlockHash:    make([]byte, 64),
		})
		require.NoError(b, err)
	}

	ref := st.Copy()

	for i := 0; b.Loop(); i++ {
		err := ref.AppendEth1DataVotes(&ethpb.Eth1Data{DepositCount: uint64(i)})
		require.NoError(b, err)
		ref = st.Copy()
	}
}

func TestEth1Setters_Heze(t *testing.T) {
	setters := map[string]func(st state.BeaconState) error{
		"SetEth1Data":                  func(st state.BeaconState) error { return st.SetEth1Data(&ethpb.Eth1Data{}) },
		"SetEth1DataVotes":             func(st state.BeaconState) error { return st.SetEth1DataVotes([]*ethpb.Eth1Data{{}}) },
		"AppendEth1DataVotes":          func(st state.BeaconState) error { return st.AppendEth1DataVotes(&ethpb.Eth1Data{}) },
		"SetEth1DepositIndex":          func(st state.BeaconState) error { return st.SetEth1DepositIndex(1) },
		"SetDepositRequestsStartIndex": func(st state.BeaconState) error { return st.SetDepositRequestsStartIndex(1) },
	}
	for name, set := range setters {
		t.Run(name, func(t *testing.T) {
			hezeState, err := util.NewBeaconStateHeze()
			require.NoError(t, err)
			require.ErrorContains(t, "not supported", set(hezeState))
			_, err = hezeState.HashTreeRoot(t.Context())
			require.NoError(t, err)

			gloasState, err := util.NewBeaconStateGloas()
			require.NoError(t, err)
			require.NoError(t, set(gloasState))
		})
	}
}
