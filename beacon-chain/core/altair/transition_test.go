package altair_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/altair"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
)

func TestProcessEpoch_CanProcess(t *testing.T) {
	st, _ := util.DeterministicGenesisStateAltair(t, params.BeaconConfig().MaxValidatorsPerCommittee)
	require.NoError(t, st.SetSlot(10*params.BeaconConfig().SlotsPerEpoch))
	err := altair.ProcessEpoch(t.Context(), st)
	require.NoError(t, err)
	require.Equal(t, uint64(0), st.Slashings()[2], "Unexpected slashed balance")

	b := st.Balances()
	require.Equal(t, params.BeaconConfig().MaxValidatorsPerCommittee, uint64(len(b)))
	require.Equal(t, uint64(31999841265), b[0])

	s, err := st.InactivityScores()
	require.NoError(t, err)
	require.Equal(t, params.BeaconConfig().MaxValidatorsPerCommittee, uint64(len(s)))

	p, err := st.PreviousEpochParticipation()
	require.NoError(t, err)
	require.Equal(t, params.BeaconConfig().MaxValidatorsPerCommittee, uint64(len(p)))

	p, err = st.CurrentEpochParticipation()
	require.NoError(t, err)
	require.Equal(t, params.BeaconConfig().MaxValidatorsPerCommittee, uint64(len(p)))

	sc, err := st.CurrentSyncCommittee()
	require.NoError(t, err)
	require.Equal(t, params.BeaconConfig().SyncCommitteeSize, uint64(len(sc.Pubkeys)))

	sc, err = st.NextSyncCommittee()
	require.NoError(t, err)
	require.Equal(t, params.BeaconConfig().SyncCommitteeSize, uint64(len(sc.Pubkeys)))
}

func TestProcessEpoch_CanProcessBellatrix(t *testing.T) {
	st, _ := util.DeterministicGenesisStateBellatrix(t, params.BeaconConfig().MaxValidatorsPerCommittee)
	require.NoError(t, st.SetSlot(10*params.BeaconConfig().SlotsPerEpoch))
	err := altair.ProcessEpoch(t.Context(), st)
	require.NoError(t, err)
	require.Equal(t, uint64(0), st.Slashings()[2], "Unexpected slashed balance")

	b := st.Balances()
	require.Equal(t, params.BeaconConfig().MaxValidatorsPerCommittee, uint64(len(b)))
	require.Equal(t, uint64(31999839993), b[0])

	s, err := st.InactivityScores()
	require.NoError(t, err)
	require.Equal(t, params.BeaconConfig().MaxValidatorsPerCommittee, uint64(len(s)))

	p, err := st.PreviousEpochParticipation()
	require.NoError(t, err)
	require.Equal(t, params.BeaconConfig().MaxValidatorsPerCommittee, uint64(len(p)))

	p, err = st.CurrentEpochParticipation()
	require.NoError(t, err)
	require.Equal(t, params.BeaconConfig().MaxValidatorsPerCommittee, uint64(len(p)))

	sc, err := st.CurrentSyncCommittee()
	require.NoError(t, err)
	require.Equal(t, params.BeaconConfig().SyncCommitteeSize, uint64(len(sc.Pubkeys)))

	sc, err = st.NextSyncCommittee()
	require.NoError(t, err)
	require.Equal(t, params.BeaconConfig().SyncCommitteeSize, uint64(len(sc.Pubkeys)))
}
