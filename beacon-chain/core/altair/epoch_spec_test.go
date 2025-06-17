package altair_test

import (
	"fmt"
	"math"
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/altair"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/epoch"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/time"
	state_native "github.com/OffchainLabs/prysm/v6/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"google.golang.org/protobuf/proto"
)

func TestProcessSyncCommitteeUpdates_CanRotate(t *testing.T) {
	s, _ := util.DeterministicGenesisStateAltair(t, params.BeaconConfig().MaxValidatorsPerCommittee)
	h := &ethpb.BeaconBlockHeader{
		StateRoot:  bytesutil.PadTo([]byte{'a'}, 32),
		ParentRoot: bytesutil.PadTo([]byte{'b'}, 32),
		BodyRoot:   bytesutil.PadTo([]byte{'c'}, 32),
	}
	require.NoError(t, s.SetLatestBlockHeader(h))
	postState, err := altair.ProcessSyncCommitteeUpdates(t.Context(), s)
	require.NoError(t, err)
	current, err := postState.CurrentSyncCommittee()
	require.NoError(t, err)
	next, err := postState.NextSyncCommittee()
	require.NoError(t, err)
	require.DeepEqual(t, current, next)

	require.NoError(t, s.SetSlot(params.BeaconConfig().SlotsPerEpoch))
	postState, err = altair.ProcessSyncCommitteeUpdates(t.Context(), s)
	require.NoError(t, err)
	c, err := postState.CurrentSyncCommittee()
	require.NoError(t, err)
	n, err := postState.NextSyncCommittee()
	require.NoError(t, err)
	require.DeepEqual(t, current, c)
	require.DeepEqual(t, next, n)

	require.NoError(t, s.SetSlot(primitives.Slot(params.BeaconConfig().EpochsPerSyncCommitteePeriod)*params.BeaconConfig().SlotsPerEpoch-1))
	postState, err = altair.ProcessSyncCommitteeUpdates(t.Context(), s)
	require.NoError(t, err)
	c, err = postState.CurrentSyncCommittee()
	require.NoError(t, err)
	n, err = postState.NextSyncCommittee()
	require.NoError(t, err)
	require.NotEqual(t, current, c)
	require.NotEqual(t, next, n)
	require.DeepEqual(t, next, c)

	// Test boundary condition.
	slot := params.BeaconConfig().SlotsPerEpoch * primitives.Slot(time.CurrentEpoch(s)+params.BeaconConfig().EpochsPerSyncCommitteePeriod)
	require.NoError(t, s.SetSlot(slot))
	boundaryCommittee, err := altair.NextSyncCommittee(t.Context(), s)
	require.NoError(t, err)
	require.DeepNotEqual(t, boundaryCommittee, n)
}

func TestProcessParticipationFlagUpdates_CanRotate(t *testing.T) {
	s, _ := util.DeterministicGenesisStateAltair(t, params.BeaconConfig().MaxValidatorsPerCommittee)
	c, err := s.CurrentEpochParticipation()
	require.NoError(t, err)
	require.DeepEqual(t, make([]byte, params.BeaconConfig().MaxValidatorsPerCommittee), c)
	p, err := s.PreviousEpochParticipation()
	require.NoError(t, err)
	require.DeepEqual(t, make([]byte, params.BeaconConfig().MaxValidatorsPerCommittee), p)

	newC := []byte{'a'}
	newP := []byte{'b'}
	require.NoError(t, s.SetCurrentParticipationBits(newC))
	require.NoError(t, s.SetPreviousParticipationBits(newP))
	c, err = s.CurrentEpochParticipation()
	require.NoError(t, err)
	require.DeepEqual(t, newC, c)
	p, err = s.PreviousEpochParticipation()
	require.NoError(t, err)
	require.DeepEqual(t, newP, p)

	s, err = altair.ProcessParticipationFlagUpdates(s)
	require.NoError(t, err)
	c, err = s.CurrentEpochParticipation()
	require.NoError(t, err)
	require.DeepEqual(t, make([]byte, params.BeaconConfig().MaxValidatorsPerCommittee), c)
	p, err = s.PreviousEpochParticipation()
	require.NoError(t, err)
	require.DeepEqual(t, newC, p)
}

func TestProcessSlashings_NotSlashed(t *testing.T) {
	base := &ethpb.BeaconStateAltair{
		Slot:       0,
		Validators: []*ethpb.Validator{{Slashed: true}},
		Balances:   []uint64{params.BeaconConfig().MaxEffectiveBalance},
		Slashings:  []uint64{0, 1e9},
	}
	s, err := state_native.InitializeFromProtoAltair(base)
	require.NoError(t, err)
	require.NoError(t, epoch.ProcessSlashings(s))
	wanted := params.BeaconConfig().MaxEffectiveBalance
	assert.Equal(t, wanted, s.Balances()[0], "Unexpected slashed balance")
}

func TestProcessSlashings_SlashedLess(t *testing.T) {
	tests := []struct {
		state *ethpb.BeaconStateAltair
		want  uint64
	}{
		{
			state: &ethpb.BeaconStateAltair{
				Validators: []*ethpb.Validator{
					{Slashed: true,
						WithdrawableEpoch: params.BeaconConfig().EpochsPerSlashingsVector / 2,
						EffectiveBalance:  params.BeaconConfig().MaxEffectiveBalance},
					{ExitEpoch: params.BeaconConfig().FarFutureEpoch, EffectiveBalance: params.BeaconConfig().MaxEffectiveBalance}},
				Balances:  []uint64{params.BeaconConfig().MaxEffectiveBalance, params.BeaconConfig().MaxEffectiveBalance},
				Slashings: []uint64{0, 1e9},
			},
			want: uint64(30000000000),
		},
		{
			state: &ethpb.BeaconStateAltair{
				Validators: []*ethpb.Validator{
					{Slashed: true,
						WithdrawableEpoch: params.BeaconConfig().EpochsPerSlashingsVector / 2,
						EffectiveBalance:  params.BeaconConfig().MaxEffectiveBalance},
					{ExitEpoch: params.BeaconConfig().FarFutureEpoch, EffectiveBalance: params.BeaconConfig().MaxEffectiveBalance},
					{ExitEpoch: params.BeaconConfig().FarFutureEpoch, EffectiveBalance: params.BeaconConfig().MaxEffectiveBalance},
				},
				Balances:  []uint64{params.BeaconConfig().MaxEffectiveBalance, params.BeaconConfig().MaxEffectiveBalance},
				Slashings: []uint64{0, 1e9},
			},
			want: uint64(31000000000),
		},
		{
			state: &ethpb.BeaconStateAltair{
				Validators: []*ethpb.Validator{
					{Slashed: true,
						WithdrawableEpoch: params.BeaconConfig().EpochsPerSlashingsVector / 2,
						EffectiveBalance:  params.BeaconConfig().MaxEffectiveBalance},
					{ExitEpoch: params.BeaconConfig().FarFutureEpoch, EffectiveBalance: params.BeaconConfig().MaxEffectiveBalance},
					{ExitEpoch: params.BeaconConfig().FarFutureEpoch, EffectiveBalance: params.BeaconConfig().MaxEffectiveBalance},
				},
				Balances:  []uint64{params.BeaconConfig().MaxEffectiveBalance, params.BeaconConfig().MaxEffectiveBalance},
				Slashings: []uint64{0, 2 * 1e9},
			},
			want: uint64(30000000000),
		},
		{
			state: &ethpb.BeaconStateAltair{
				Validators: []*ethpb.Validator{
					{Slashed: true,
						WithdrawableEpoch: params.BeaconConfig().EpochsPerSlashingsVector / 2,
						EffectiveBalance:  params.BeaconConfig().MaxEffectiveBalance - params.BeaconConfig().EffectiveBalanceIncrement},
					{ExitEpoch: params.BeaconConfig().FarFutureEpoch, EffectiveBalance: params.BeaconConfig().MaxEffectiveBalance - params.BeaconConfig().EffectiveBalanceIncrement}},
				Balances:  []uint64{params.BeaconConfig().MaxEffectiveBalance - params.BeaconConfig().EffectiveBalanceIncrement, params.BeaconConfig().MaxEffectiveBalance - params.BeaconConfig().EffectiveBalanceIncrement},
				Slashings: []uint64{0, 1e9},
			},
			want: uint64(29000000000),
		},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			helpers.ClearCache()
			original := proto.Clone(tt.state)
			s, err := state_native.InitializeFromProtoAltair(tt.state)
			require.NoError(t, err)
			require.NoError(t, epoch.ProcessSlashings(s))
			assert.Equal(t, tt.want, s.Balances()[0], "ProcessSlashings({%v}) = newState; newState.Balances[0] = %d", original, s.Balances()[0])
		})
	}
}

func TestProcessSlashings_BadValue(t *testing.T) {
	base := &ethpb.BeaconStateAltair{
		Slot:       0,
		Validators: []*ethpb.Validator{{Slashed: true}},
		Balances:   []uint64{params.BeaconConfig().MaxEffectiveBalance},
		Slashings:  []uint64{math.MaxUint64, 1e9},
	}
	s, err := state_native.InitializeFromProtoAltair(base)
	require.NoError(t, err)
	require.ErrorContains(t, "addition overflows", epoch.ProcessSlashings(s))
}
