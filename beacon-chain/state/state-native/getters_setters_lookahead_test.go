package state_native_test

import (
	"testing"

	state_native "github.com/OffchainLabs/prysm/v6/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func TestProposerLookahead(t *testing.T) {
	t.Run("Fulu expected values", func(t *testing.T) {
		lookahead := make([]uint64, int(params.BeaconConfig().MinSeedLookahead+1)*int(params.BeaconConfig().SlotsPerEpoch))
		want := make([]primitives.ValidatorIndex, int(params.BeaconConfig().MinSeedLookahead+1)*int(params.BeaconConfig().SlotsPerEpoch))
		st, err := state_native.InitializeFromProtoFulu(&ethpb.BeaconStateFulu{
			ProposerLookahead: lookahead,
		})
		require.NoError(t, err)
		got, err := st.ProposerLookahead()
		require.NoError(t, err)
		require.Equal(t, len(want), len(got))
		for i, w := range want {
			require.Equal(t, w, got[i], "index %d", i)
		}
	})

	t.Run("Fulu error on invalid size", func(t *testing.T) {
		lookahead := make([]primitives.ValidatorIndex, int(params.BeaconConfig().MinSeedLookahead+1)*int(params.BeaconConfig().SlotsPerEpoch)+1)
		st, err := state_native.InitializeFromProtoFulu(&ethpb.BeaconStateFulu{})
		require.NoError(t, err)
		require.ErrorContains(t, "invalid size for proposer lookahead", st.SetProposerLookahead(lookahead))
	})

	t.Run("earlier than electra returns error", func(t *testing.T) {
		st, err := state_native.InitializeFromProtoDeneb(&ethpb.BeaconStateDeneb{})
		require.NoError(t, err)
		_, err = st.ProposerLookahead()
		require.ErrorContains(t, "is not supported", err)
		lookahead := make([]primitives.ValidatorIndex, int(params.BeaconConfig().MinSeedLookahead+1)*int(params.BeaconConfig().SlotsPerEpoch))
		require.ErrorContains(t, "is not supported", st.SetProposerLookahead(lookahead))
	})
}
