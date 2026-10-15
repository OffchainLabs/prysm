package blockchain

import (
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

// The fork choice tree for the tests: G(0) <- A(10) <- B(40, head), G <- C(20), G <- E(25) <- F(50).
var (
	ilRootA = [32]byte{'a'}
	ilRootB = [32]byte{'b'}
	ilRootC = [32]byte{'c'}
	ilRootE = [32]byte{'e'}
	ilRootF = [32]byte{'f'}
)

const ilCurrentSlot = primitives.Slot(65)

type inclusionListFixture struct {
	s    *Service
	st   state.BeaconState
	keys []bls.SecretKey
}

// setHezeAtGenesis activates every fork up to Heze at genesis.
func setHezeAtGenesis(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.AltairForkEpoch = 0
	cfg.BellatrixForkEpoch = 0
	cfg.CapellaForkEpoch = 0
	cfg.DenebForkEpoch = 0
	cfg.ElectraForkEpoch = 0
	cfg.FuluForkEpoch = 0
	cfg.GloasForkEpoch = 0
	cfg.HezeForkEpoch = 0
	params.OverrideBeaconConfig(cfg)
	helpers.ClearCache()
}

// setupInclusionListService returns a service whose clock is intoSlot into ilCurrentSlot.
func setupInclusionListService(t *testing.T, intoSlot time.Duration) *inclusionListFixture {
	setHezeAtGenesis(t)
	s, _ := minimalTestService(t)
	ctx := t.Context()
	ojc := &ethpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]}
	for _, n := range []struct {
		slot         primitives.Slot
		root, parent [32]byte
	}{
		{0, [32]byte{}, [32]byte{}},
		{10, ilRootA, [32]byte{}},
		{20, ilRootC, [32]byte{}},
		{25, ilRootE, [32]byte{}},
		{40, ilRootB, ilRootA},
		{50, ilRootF, ilRootE},
	} {
		st, roblock, err := prepareForkchoiceState(ctx, n.slot, n.root, n.parent, params.BeaconConfig().ZeroHash, ojc, ojc)
		require.NoError(t, err)
		require.NoError(t, s.cfg.ForkChoiceStore.InsertNode(ctx, st, roblock))
	}

	st, keys := util.DeterministicGenesisStateElectra(t, 64)
	require.NoError(t, st.SetSlot(40))
	s.head = &head{root: ilRootB, state: st, slot: 40}
	slotDuration := params.BeaconConfig().SlotDuration()
	s.genesisTime = time.Now().Add(-time.Duration(ilCurrentSlot)*slotDuration - intoSlot)
	return &inclusionListFixture{s: s, st: st, keys: keys}
}

// committeeMember returns the first inclusion list committee member for slot.
func (f *inclusionListFixture) committeeMember(t *testing.T, slot primitives.Slot) primitives.ValidatorIndex {
	committee, err := helpers.InclusionListCommittee(t.Context(), f.st, slot)
	require.NoError(t, err)
	return committee[0]
}

// nonMember returns a validator that is not in the inclusion list committee for slot.
func (f *inclusionListFixture) nonMember(t *testing.T, slot primitives.Slot) primitives.ValidatorIndex {
	committee, err := helpers.InclusionListCommittee(t.Context(), f.st, slot)
	require.NoError(t, err)
	for idx := range primitives.ValidatorIndex(f.st.NumValidators()) {
		found := false
		for _, m := range committee {
			found = found || m == idx
		}
		if !found {
			return idx
		}
	}
	t.Fatal("every validator is a committee member")
	return 0
}

func (f *inclusionListFixture) sign(t *testing.T, signer primitives.ValidatorIndex, il *ethpb.InclusionList) *ethpb.SignedInclusionList {
	domain, err := helpers.InclusionListSigningDomain(slots.ToEpoch(il.Slot), f.st.GenesisValidatorsRoot())
	require.NoError(t, err)
	root, err := signing.ComputeSigningRoot(il, domain)
	require.NoError(t, err)
	return &ethpb.SignedInclusionList{Message: il, Signature: f.keys[signer].Sign(root[:]).Marshal()}
}

func ilTransactions(t *testing.T, txs ...[]byte) *enginev1.ProgressiveTransactionList {
	l, err := enginev1.NewProgressiveTransactionList(txs)
	require.NoError(t, err)
	return l
}

func TestOnInclusionList_Checks(t *testing.T) {
	f := setupInclusionListService(t, time.Second)
	member := f.committeeMember(t, ilCurrentSlot)
	tooLarge := make([]byte, params.BeaconConfig().MaxTransactionsBytesPerInclusionList+1)
	newIL := func(mod func(*ethpb.InclusionList)) *ethpb.InclusionList {
		il := &ethpb.InclusionList{Slot: ilCurrentSlot, ValidatorIndex: member, DependentRoot: ilRootA[:], Transactions: ilTransactions(t, []byte("tx"))}
		if mod != nil {
			mod(il)
		}
		return il
	}
	signed := func(mod func(*ethpb.InclusionList)) *ethpb.SignedInclusionList {
		il := newIL(mod)
		return f.sign(t, il.ValidatorIndex, il)
	}

	tests := []struct {
		name   string
		signed *ethpb.SignedInclusionList
		err    error
	}{
		{name: "nil", signed: nil, err: ErrNilInclusionList},
		{name: "short dependent root", signed: &ethpb.SignedInclusionList{Message: newIL(func(il *ethpb.InclusionList) { il.DependentRoot = make([]byte, 31) }), Signature: make([]byte, 96)}, err: ErrInclusionListDependentRootLength},
		{name: "future slot", signed: signed(func(il *ethpb.InclusionList) { il.Slot = ilCurrentSlot + 1 }), err: ErrInclusionListOutOfWindow},
		{name: "slot before the retention window", signed: signed(func(il *ethpb.InclusionList) { il.Slot = ilCurrentSlot - 2 }), err: ErrInclusionListOutOfWindow},
		{name: "no transactions", signed: signed(func(il *ethpb.InclusionList) { il.Transactions = ilTransactions(t) }), err: ErrInclusionListNoTransactions},
		{name: "nil transactions", signed: signed(func(il *ethpb.InclusionList) { il.Transactions = nil }), err: ErrInclusionListNoTransactions},
		{name: "too large", signed: signed(func(il *ethpb.InclusionList) { il.Transactions = ilTransactions(t, tooLarge) }), err: ErrInclusionListTooLarge},
		{name: "empty transaction", signed: signed(func(il *ethpb.InclusionList) { il.Transactions = ilTransactions(t, []byte("tx"), []byte{}) }), err: ErrInclusionListEmptyTransaction},
		{name: "unknown dependent block", signed: signed(func(il *ethpb.InclusionList) { il.DependentRoot = make([]byte, 32); il.DependentRoot[0] = 'z' }), err: ErrInclusionListUnknownDependentBlock},
		{name: "dependent block after the dependent slot", signed: signed(func(il *ethpb.InclusionList) { il.DependentRoot = ilRootB[:] }), err: ErrInclusionListDependentBlockTooLate},
		{name: "not a possible dependent block", signed: signed(func(il *ethpb.InclusionList) { il.DependentRoot = ilRootC[:] }), err: ErrInclusionListInvalidDependentRoot},
		{name: "dependent state not cached off the head branch", signed: signed(func(il *ethpb.InclusionList) { il.DependentRoot = ilRootE[:] }), err: ErrInclusionListDependentStateUnavailable},
		{name: "not in the committee", signed: signed(func(il *ethpb.InclusionList) { il.ValidatorIndex = f.nonMember(t, ilCurrentSlot) }), err: ErrInclusionListNotInCommittee},
		{name: "bad signature", signed: &ethpb.SignedInclusionList{Message: newIL(nil), Signature: f.sign(t, member, newIL(func(il *ethpb.InclusionList) { il.Transactions = ilTransactions(t, []byte("other")) })).Signature}, err: ErrInclusionListInvalidSignature},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.ErrorIs(t, f.s.OnInclusionList(t.Context(), tt.signed), tt.err)
			for _, slot := range []primitives.Slot{ilCurrentSlot - 2, ilCurrentSlot - 1, ilCurrentSlot, ilCurrentSlot + 1} {
				for _, root := range [][32]byte{ilRootA, ilRootB, ilRootC, ilRootE, {}} {
					require.Equal(t, 0, len(f.s.InclusionListStore().Transactions(slot, root, false)))
				}
			}
		})
	}

	t.Run("pre-Heze slot", func(t *testing.T) {
		cfg := params.BeaconConfig().Copy()
		cfg.HezeForkEpoch = 3
		params.OverrideBeaconConfig(cfg)
		defer func() {
			cfg := params.BeaconConfig().Copy()
			cfg.HezeForkEpoch = 0
			params.OverrideBeaconConfig(cfg)
		}()
		require.ErrorIs(t, f.s.OnInclusionList(t.Context(), signed(nil)), ErrInclusionListPreHeze)
	})
}

func TestOnInclusionList_Timeliness(t *testing.T) {
	due := func() time.Duration {
		cfg := params.BeaconConfig()
		return cfg.SlotComponentDuration(cfg.InclusionListDueBPS)
	}
	tests := []struct {
		name       string
		intoSlot   func() time.Duration
		slot       primitives.Slot
		wantTimely bool
	}{
		{name: "current slot before the deadline", intoSlot: func() time.Duration { return due() - 2*time.Second }, slot: ilCurrentSlot, wantTimely: true},
		{name: "current slot after the deadline", intoSlot: func() time.Duration { return due() + time.Second }, slot: ilCurrentSlot},
		{name: "previous slot", intoSlot: func() time.Duration { return time.Second }, slot: ilCurrentSlot - 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := setupInclusionListService(t, tt.intoSlot())
			member := f.committeeMember(t, tt.slot)
			il := &ethpb.InclusionList{Slot: tt.slot, ValidatorIndex: member, DependentRoot: ilRootA[:], Transactions: ilTransactions(t, []byte("tx"))}
			require.NoError(t, f.s.OnInclusionList(t.Context(), f.sign(t, member, il)))
			_, timely, ok := f.s.InclusionListStore().Get(tt.slot, ilRootA, member)
			require.Equal(t, true, ok)
			require.Equal(t, tt.wantTimely, timely)
			require.Equal(t, 1, len(f.s.InclusionListStore().Transactions(tt.slot, ilRootA, false)))
			want := 0
			if tt.wantTimely {
				want = 1
			}
			require.Equal(t, want, len(f.s.InclusionListStore().Transactions(tt.slot, ilRootA, true)))
		})
	}
}

func TestOnInclusionList_NextSlotWithinClockDisparity(t *testing.T) {
	slotDuration := params.BeaconConfig().SlotDuration()
	disparity := params.BeaconConfig().MaximumGossipClockDisparityDuration()
	tests := []struct {
		name     string
		intoSlot time.Duration
		err      error
	}{
		{name: "next slot within the clock disparity", intoSlot: slotDuration - disparity/2},
		{name: "next slot beyond the clock disparity", intoSlot: slotDuration - 2*time.Second, err: ErrInclusionListOutOfWindow},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := setupInclusionListService(t, tt.intoSlot)
			next := ilCurrentSlot + 1
			member := f.committeeMember(t, next)
			il := &ethpb.InclusionList{Slot: next, ValidatorIndex: member, DependentRoot: ilRootA[:], Transactions: ilTransactions(t, []byte("tx"))}
			err := f.s.OnInclusionList(t.Context(), f.sign(t, member, il))
			// The check must run in the current slot. A slow machine would otherwise pass through the current-slot path.
			require.Equal(t, ilCurrentSlot, slots.At(f.s.genesisTime, time.Now()))
			if tt.err != nil {
				require.ErrorIs(t, err, tt.err)
				require.Equal(t, 0, len(f.s.InclusionListStore().Transactions(next, ilRootA, false)))
				return
			}
			require.NoError(t, err)
			_, timely, ok := f.s.InclusionListStore().Get(next, ilRootA, member)
			require.Equal(t, true, ok)
			require.Equal(t, true, timely)
		})
	}
}

func TestOnInclusionList_DuplicateAndEquivocation(t *testing.T) {
	f := setupInclusionListService(t, time.Second)
	member := f.committeeMember(t, ilCurrentSlot)
	il := &ethpb.InclusionList{Slot: ilCurrentSlot, ValidatorIndex: member, DependentRoot: ilRootA[:], Transactions: ilTransactions(t, []byte("tx"))}
	store := f.s.InclusionListStore()

	require.NoError(t, f.s.OnInclusionList(t.Context(), f.sign(t, member, il)))
	t.Run("identical duplicate is ignored", func(t *testing.T) {
		require.NoError(t, f.s.OnInclusionList(t.Context(), f.sign(t, member, il)))
		require.Equal(t, false, store.IsEquivocator(ilCurrentSlot, ilRootA, member))
		require.Equal(t, 1, len(store.Transactions(ilCurrentSlot, ilRootA, true)))
	})
	t.Run("different list marks the equivocator", func(t *testing.T) {
		other := &ethpb.InclusionList{Slot: ilCurrentSlot, ValidatorIndex: member, DependentRoot: ilRootA[:], Transactions: ilTransactions(t, []byte("other"))}
		require.NoError(t, f.s.OnInclusionList(t.Context(), f.sign(t, member, other)))
		require.Equal(t, true, store.IsEquivocator(ilCurrentSlot, ilRootA, member))
		require.Equal(t, 0, len(store.Transactions(ilCurrentSlot, ilRootA, true)))
	})
}

func TestOnInclusionList_CachedOffHeadState(t *testing.T) {
	f := setupInclusionListService(t, time.Second)
	offHead := f.st.Copy()
	require.NoError(t, offHead.SetSlot(25))
	require.NoError(t, f.s.cfg.StateGen.SaveState(t.Context(), ilRootE, offHead))
	member := f.committeeMember(t, ilCurrentSlot)
	il := &ethpb.InclusionList{Slot: ilCurrentSlot, ValidatorIndex: member, DependentRoot: ilRootE[:], Transactions: ilTransactions(t, []byte("tx"))}
	require.NoError(t, f.s.OnInclusionList(t.Context(), f.sign(t, member, il)))
	_, _, ok := f.s.InclusionListStore().Get(ilCurrentSlot, ilRootE, member)
	require.Equal(t, true, ok)
}

func TestIsValidDependentRoot(t *testing.T) {
	f := setupInclusionListService(t, time.Second)
	tests := []struct {
		name string
		root [32]byte
		want bool
	}{
		{name: "head", root: ilRootB, want: true},
		{name: "dependent root of the head", root: ilRootA, want: true},
		{name: "dependent root of another tip", root: ilRootE, want: true},
		{name: "tip without a later child", root: ilRootC},
		{name: "unknown root", root: [32]byte{'z'}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, f.s.isValidDependentRoot(tt.root, 1))
		})
	}
}

func TestLookaheadState(t *testing.T) {
	f := setupInclusionListService(t, time.Second)
	t.Run("head branch uses the head state", func(t *testing.T) {
		st, err := f.s.LookaheadState(t.Context(), ilRootA, 1)
		require.NoError(t, err)
		require.Equal(t, primitives.Slot(40), st.Slot())
	})
	t.Run("not a possible dependent block", func(t *testing.T) {
		_, err := f.s.LookaheadState(t.Context(), ilRootC, 1)
		require.ErrorIs(t, err, ErrInvalidDependentRoot)
	})
	t.Run("off the head branch without a cached state", func(t *testing.T) {
		_, err := f.s.LookaheadState(t.Context(), ilRootE, 1)
		require.ErrorIs(t, err, ErrDependentStateUnavailable)
	})
	t.Run("cached state off the head branch is advanced to the lookahead start", func(t *testing.T) {
		offHead := f.st.Copy()
		require.NoError(t, offHead.SetSlot(25))
		require.NoError(t, f.s.cfg.StateGen.SaveState(t.Context(), ilRootE, offHead))
		st, err := f.s.LookaheadState(t.Context(), ilRootE, 1)
		require.NoError(t, err)
		require.Equal(t, params.BeaconConfig().SlotsPerEpoch, st.Slot())
	})
	t.Run("state more than one epoch before the lookahead epoch", func(t *testing.T) {
		// The head state is at epoch 1, so epoch 3 would need two epochs of slot processing.
		_, err := f.s.LookaheadState(t.Context(), ilRootB, 3)
		require.ErrorIs(t, err, ErrDependentStateUnavailable)
	})
}
