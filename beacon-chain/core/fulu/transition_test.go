package fulu_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/fulu"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
)

func TestProcessEpoch_CanProcessFulu(t *testing.T) {
	st, _ := util.DeterministicGenesisStateElectra(t, params.BeaconConfig().MaxValidatorsPerCommittee)
	require.NoError(t, st.SetSlot(10*params.BeaconConfig().SlotsPerEpoch))
	st, err := fulu.UpgradeToFulu(t.Context(), st)
	require.NoError(t, err)
	preLookahead, err := st.ProposerLookahead()
	require.NoError(t, err)
	err = fulu.ProcessEpoch(t.Context(), st)
	require.NoError(t, err)
	postLookahead, err := st.ProposerLookahead()
	require.NoError(t, err)
	require.NotEqual(t, preLookahead[0], postLookahead[0])
	for i, v := range preLookahead[params.BeaconConfig().SlotsPerEpoch:] {
		require.Equal(t, v, postLookahead[i])
	}
}
