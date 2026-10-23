package helpers_test

import (
	"fmt"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

// setHezeForkEpoch activates every fork before Heze at genesis and Heze at the given epoch.
func setHezeForkEpoch(t *testing.T, epoch primitives.Epoch) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	if fieldparams.Preset == "minimal" {
		cfg = params.MinimalSpecConfig().Copy()
	}
	cfg.AltairForkEpoch = 0
	cfg.BellatrixForkEpoch = 0
	cfg.CapellaForkEpoch = 0
	cfg.DenebForkEpoch = 0
	cfg.ElectraForkEpoch = 0
	cfg.FuluForkEpoch = 0
	cfg.GloasForkEpoch = 0
	cfg.HezeForkEpoch = epoch
	params.OverrideBeaconConfig(cfg)
	helpers.ClearCache()
}

func concatenatedCommittees(t *testing.T, st state.ReadOnlyBeaconState, slot primitives.Slot) []primitives.ValidatorIndex {
	activeCount, err := helpers.ActiveValidatorCount(t.Context(), st, slots.ToEpoch(slot))
	require.NoError(t, err)
	var indices []primitives.ValidatorIndex
	for i := range helpers.SlotCommitteeCount(activeCount) {
		committee, err := helpers.BeaconCommitteeFromState(t.Context(), st, slot, primitives.CommitteeIndex(i))
		require.NoError(t, err)
		indices = append(indices, committee...)
	}
	return indices
}

func TestInclusionListCommittee(t *testing.T) {
	size := fieldparams.InclusionListCommitteeSize
	for _, perSlot := range []uint64{2, 16, 32} {
		t.Run(fmt.Sprintf("%d validators per slot", perSlot), func(t *testing.T) {
			setHezeForkEpoch(t, 0)
			st, _ := util.DeterministicGenesisStateElectra(t, perSlot*uint64(params.BeaconConfig().SlotsPerEpoch))
			slot := primitives.Slot(3)
			concatenated := concatenatedCommittees(t, st, slot)
			require.Equal(t, int(perSlot), len(concatenated))

			got, err := helpers.InclusionListCommittee(t.Context(), st, slot)
			require.NoError(t, err)
			require.Equal(t, size, len(got))
			for i := range size {
				require.Equal(t, concatenated[i%len(concatenated)], got[i])
			}

			again, err := helpers.InclusionListCommittee(t.Context(), st, slot)
			require.NoError(t, err)
			require.DeepEqual(t, got, again)
		})
	}

	t.Run("committees smaller than the inclusion list committee", func(t *testing.T) {
		setHezeForkEpoch(t, 0)
		cfg := params.BeaconConfig().Copy()
		cfg.TargetCommitteeSize = 4
		params.OverrideBeaconConfig(cfg)
		st, _ := util.DeterministicGenesisStateElectra(t, 32*uint64(params.BeaconConfig().SlotsPerEpoch))
		slot := primitives.Slot(3)
		concatenated := concatenatedCommittees(t, st, slot)
		committee0, err := helpers.BeaconCommitteeFromState(t.Context(), st, slot, 0)
		require.NoError(t, err)
		require.Equal(t, true, len(committee0) < size)

		got, err := helpers.InclusionListCommittee(t.Context(), st, slot)
		require.NoError(t, err)
		require.DeepEqual(t, concatenated[:size], got)
	})

	t.Run("before Heze", func(t *testing.T) {
		setHezeForkEpoch(t, 5)
		st, _ := util.DeterministicGenesisStateElectra(t, 64)
		_, err := helpers.InclusionListCommittee(t.Context(), st, 0)
		require.ErrorContains(t, "before the Heze fork", err)
	})
}

func TestInclusionListCommitteeAssignment(t *testing.T) {
	setHezeForkEpoch(t, 0)
	// 32 validators per slot, so the members at position 16 and later are in no committee.
	st, _ := util.DeterministicGenesisStateElectra(t, 32*uint64(params.BeaconConfig().SlotsPerEpoch))
	slot := primitives.Slot(5)
	concatenated := concatenatedCommittees(t, st, slot)
	require.Equal(t, 32, len(concatenated))

	t.Run("found", func(t *testing.T) {
		got, ok, err := helpers.InclusionListCommitteeAssignment(t.Context(), st, 0, concatenated[0])
		require.NoError(t, err)
		require.Equal(t, true, ok)
		require.Equal(t, slot, got)
	})
	t.Run("absent", func(t *testing.T) {
		_, ok, err := helpers.InclusionListCommitteeAssignment(t.Context(), st, 0, concatenated[20])
		require.NoError(t, err)
		require.Equal(t, false, ok)
	})
	t.Run("next epoch", func(t *testing.T) {
		nextSlot := params.BeaconConfig().SlotsPerEpoch + 5
		next := concatenatedCommittees(t, st, nextSlot)
		got, ok, err := helpers.InclusionListCommitteeAssignment(t.Context(), st, 1, next[0])
		require.NoError(t, err)
		require.Equal(t, true, ok)
		require.Equal(t, nextSlot, got)
	})
	t.Run("after next epoch", func(t *testing.T) {
		_, _, err := helpers.InclusionListCommitteeAssignment(t.Context(), st, 2, concatenated[0])
		require.ErrorContains(t, "after the next epoch", err)
	})
}

func TestVerifyInclusionListSignature(t *testing.T) {
	setHezeForkEpoch(t, 1)
	st, keys := util.DeterministicGenesisStateGloas(t, 64)
	// The state is still in Gloas at the lookahead slot before the first Heze epoch.
	hezeSlot := params.BeaconConfig().SlotsPerEpoch
	require.NoError(t, st.SetSlot(hezeSlot-1))
	require.DeepEqual(t, params.BeaconConfig().GloasForkVersion, st.Fork().CurrentVersion)

	txs, err := enginev1.NewProgressiveTransactionList([][]byte{[]byte("tx1")})
	require.NoError(t, err)
	valIdx := primitives.ValidatorIndex(3)
	msg := &ethpb.InclusionList{Slot: hezeSlot, ValidatorIndex: valIdx, DependentRoot: make([]byte, 32), Transactions: txs}

	hezeDomain, err := signing.ComputeDomain(params.BeaconConfig().DomainInclusionListCommittee, params.BeaconConfig().HezeForkVersion, st.GenesisValidatorsRoot())
	require.NoError(t, err)
	got, err := helpers.InclusionListSigningDomain(slots.ToEpoch(hezeSlot), st.GenesisValidatorsRoot())
	require.NoError(t, err)
	require.DeepEqual(t, hezeDomain, got)
	stateForkDomain, err := signing.Domain(st.Fork(), slots.ToEpoch(hezeSlot), params.BeaconConfig().DomainInclusionListCommittee, st.GenesisValidatorsRoot())
	require.NoError(t, err)
	require.DeepNotEqual(t, hezeDomain, stateForkDomain)

	sign := func(idx primitives.ValidatorIndex, m *ethpb.InclusionList, domain []byte) *ethpb.SignedInclusionList {
		root, err := signing.ComputeSigningRoot(m, domain)
		require.NoError(t, err)
		return &ethpb.SignedInclusionList{Message: m, Signature: keys[idx].Sign(root[:]).Marshal()}
	}
	otherSlot := &ethpb.InclusionList{Slot: hezeSlot + 1, ValidatorIndex: valIdx, DependentRoot: make([]byte, 32), Transactions: txs}

	tests := []struct {
		name   string
		signed *ethpb.SignedInclusionList
		err    string
	}{
		{name: "valid with the Heze fork version", signed: sign(valIdx, msg, hezeDomain)},
		{name: "state fork version fails", signed: sign(valIdx, msg, stateForkDomain), err: signing.ErrSigFailedToVerify.Error()},
		{name: "wrong key", signed: sign(valIdx+1, msg, hezeDomain), err: signing.ErrSigFailedToVerify.Error()},
		{name: "wrong slot", signed: &ethpb.SignedInclusionList{Message: msg, Signature: sign(valIdx, otherSlot, hezeDomain).Signature}, err: signing.ErrSigFailedToVerify.Error()},
		{name: "nil signed list", signed: nil, err: "nil inclusion list"},
		{name: "nil message", signed: &ethpb.SignedInclusionList{Signature: make([]byte, 96)}, err: "nil inclusion list"},
		{name: "short signature", signed: &ethpb.SignedInclusionList{Message: msg, Signature: make([]byte, 95)}, err: "signature length"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := helpers.VerifyInclusionListSignature(t.Context(), st, tt.signed)
			if tt.err == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, tt.err, err)
		})
	}
}
