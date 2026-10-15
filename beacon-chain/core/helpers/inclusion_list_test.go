package helpers_test

import (
	"fmt"
	"testing"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
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
	// The minimal build has minimal field sizes, so it needs the minimal config too.
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
	t.Run("before Heze", func(t *testing.T) {
		setHezeForkEpoch(t, 1)
		st, _ := util.DeterministicGenesisStateElectra(t, 64)
		_, _, err := helpers.InclusionListCommitteeAssignment(t.Context(), st, 0, 0)
		require.ErrorContains(t, "before the Heze fork", err)
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
	unknownValidator := &ethpb.InclusionList{Slot: hezeSlot, ValidatorIndex: 1000, DependentRoot: make([]byte, 32), Transactions: txs}
	undecodable := make([]byte, 96)
	for i := range undecodable {
		undecodable[i] = 0xff
	}

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
		{name: "validator index out of range", signed: sign(valIdx, unknownValidator, hezeDomain), err: "could not get validator"},
		{name: "undecodable signature", signed: &ethpb.SignedInclusionList{Message: msg, Signature: undecodable}, err: "could not convert signature"},
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

func TestProposalInclusionLists(t *testing.T) {
	newView := func(t *testing.T, store *cache.InclusionListStore, slot primitives.Slot) *helpers.ProposalInclusionLists {
		st, _ := util.DeterministicGenesisStateElectra(t, 16*uint64(params.BeaconConfig().SlotsPerEpoch))
		require.NoError(t, st.SetSlot(slot))
		view, err := helpers.NewProposalInclusionLists(t.Context(), nil, store, st, slot)
		require.NoError(t, err)
		return view
	}

	t.Run("no view without inclusion lists at slot - 1", func(t *testing.T) {
		for _, tt := range []struct {
			name      string
			hezeEpoch primitives.Epoch
			slot      primitives.Slot
		}{
			{name: "slot 0", hezeEpoch: 0, slot: 0},
			{name: "before Heze", hezeEpoch: 5, slot: params.BeaconConfig().SlotsPerEpoch + 1},
			// slot - 1 is the last Gloas slot.
			{name: "first Heze slot", hezeEpoch: 2, slot: 2 * params.BeaconConfig().SlotsPerEpoch},
		} {
			t.Run(tt.name, func(t *testing.T) {
				setHezeForkEpoch(t, tt.hezeEpoch)
				view := newView(t, cache.NewInclusionListStore(), tt.slot)
				require.Equal(t, true, view == nil)
				require.DeepEqual(t, [][]byte{}, view.Transactions())
				bits, err := view.Bits(t.Context())
				require.NoError(t, err)
				require.DeepEqual(t, bitfield.NewBitvector16(), bits)
				inclusive, err := view.IsBitsInclusive(t.Context(), bitfield.NewBitvector16())
				require.NoError(t, err)
				require.Equal(t, true, inclusive)
			})
		}
	})

	t.Run("lists of slot - 1 with only_timely=False", func(t *testing.T) {
		setHezeForkEpoch(t, 0)
		slot := 2*params.BeaconConfig().SlotsPerEpoch + 1
		st, _ := util.DeterministicGenesisStateElectra(t, 16*uint64(params.BeaconConfig().SlotsPerEpoch))
		require.NoError(t, st.SetSlot(slot))
		dependentRoot := [32]byte{'d'}
		dependentSlot, err := slots.ShufflingDependentSlot(slots.ToEpoch(slot - 1))
		require.NoError(t, err)
		require.NoError(t, st.UpdateBlockRootAtIndex(uint64(dependentSlot%params.BeaconConfig().SlotsPerHistoricalRoot), dependentRoot))
		committee, err := helpers.InclusionListCommittee(t.Context(), st, slot-1)
		require.NoError(t, err)

		store := cache.NewInclusionListStore()
		for _, l := range []struct {
			member int
			root   [32]byte
			timely bool
		}{
			{member: 0, root: dependentRoot, timely: true},
			{member: 1, root: dependentRoot, timely: false},
			{member: 2, root: [32]byte{'o'}, timely: true},
		} {
			txs, err := enginev1.NewProgressiveTransactionList([][]byte{{byte(l.member)}})
			require.NoError(t, err)
			il := &ethpb.InclusionList{Slot: slot - 1, ValidatorIndex: committee[l.member], DependentRoot: l.root[:], Transactions: txs}
			store.Process(&ethpb.SignedInclusionList{Message: il, Signature: make([]byte, fieldparams.BLSSignatureLength)}, l.timely)
		}
		view, err := helpers.NewProposalInclusionLists(t.Context(), nil, store, st, slot)
		require.NoError(t, err)
		require.NotNil(t, view)

		// The late list counts, and the list of another dependent root does not.
		want := [][]byte{{0}, {1}}
		if committee[1] < committee[0] {
			want[0], want[1] = want[1], want[0]
		}
		require.DeepEqual(t, want, view.Transactions())

		bits, err := view.Bits(t.Context())
		require.NoError(t, err)
		want01 := bitfield.NewBitvector16()
		want01.SetBitAt(0, true)
		want01.SetBitAt(1, true)
		require.DeepEqual(t, want01, bits)

		only0 := bitfield.NewBitvector16()
		only0.SetBitAt(0, true)
		extra := bitfield.NewBitvector16()
		extra.SetBitAt(0, true)
		extra.SetBitAt(1, true)
		extra.SetBitAt(2, true)
		for _, tt := range []struct {
			name string
			bits bitfield.Bitvector16
			want bool
		}{
			{name: "same bits", bits: want01, want: true},
			{name: "extra bits", bits: extra, want: true},
			{name: "missing bit", bits: only0, want: false},
		} {
			t.Run(tt.name, func(t *testing.T) {
				inclusive, err := view.IsBitsInclusive(t.Context(), tt.bits)
				require.NoError(t, err)
				require.Equal(t, tt.want, inclusive)
			})
		}
	})
}
