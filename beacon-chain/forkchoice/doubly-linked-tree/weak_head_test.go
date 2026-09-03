package doublylinkedtree

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestStore_IsHeadWeak(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.ReorgHeadWeightThreshold = 20
	require.NoError(t, params.SetActive(cfg))

	tests := []struct {
		name      string
		committee uint64
		balance   uint64
		indices   []primitives.ValidatorIndex
		slashed   map[primitives.ValidatorIndex]bool
		effective []uint64
		wantWeak  bool
	}{
		{name: "below threshold", committee: 100, balance: 19, wantWeak: true},
		{name: "equal to threshold", committee: 100, balance: 20},
		{name: "above threshold", committee: 100, balance: 21},
		{name: "below floored threshold", committee: 99, balance: 18, wantWeak: true},
		{name: "equal to floored threshold", committee: 99, balance: 19},
		{name: "zero threshold", committee: 4},
		{
			name: "head committee equivocator restores threshold", committee: 100, balance: 19,
			indices: []primitives.ValidatorIndex{1}, slashed: map[primitives.ValidatorIndex]bool{1: true}, effective: []uint64{0, 1},
		},
		{
			name: "equivocator outside head committee is excluded", committee: 100, balance: 19,
			indices: []primitives.ValidatorIndex{0}, slashed: map[primitives.ValidatorIndex]bool{1: true}, effective: []uint64{0, 100}, wantWeak: true,
		},
		{
			name: "unslashed committee member is not added", committee: 100, balance: 19,
			indices: []primitives.ValidatorIndex{1}, slashed: map[primitives.ValidatorIndex]bool{2: true}, effective: []uint64{0, 100, 100}, wantWeak: true,
		},
		{
			name: "false slashed entry is not added", committee: 100, balance: 19,
			indices: []primitives.ValidatorIndex{1}, slashed: map[primitives.ValidatorIndex]bool{1: false}, effective: []uint64{0, 100}, wantWeak: true,
		},
		{
			name: "zero justified effective balance is not added", committee: 100, balance: 19,
			indices: []primitives.ValidatorIndex{1}, slashed: map[primitives.ValidatorIndex]bool{1: true}, effective: []uint64{0, 0}, wantWeak: true,
		},
		{
			name: "validator absent at justified checkpoint is excluded", committee: 100, balance: 19,
			indices: []primitives.ValidatorIndex{1}, slashed: map[primitives.ValidatorIndex]bool{1: true}, effective: []uint64{0}, wantWeak: true,
		},
		{
			name: "equivocator counts without any latest vote", committee: 100,
			indices: []primitives.ValidatorIndex{1}, slashed: map[primitives.ValidatorIndex]bool{1: true}, effective: []uint64{0, 20},
		},
		{
			name: "missing committee with equivocations is conservative", committee: 100,
			slashed: map[primitives.ValidatorIndex]bool{1: true}, effective: []uint64{0, 20},
		},
		{
			name: "loaded empty committee does not restore other validators", committee: 100, balance: 19,
			indices: []primitives.ValidatorIndex{}, slashed: map[primitives.ValidatorIndex]bool{1: true}, effective: []uint64{0, 100}, wantWeak: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &Node{root: [32]byte{'a'}, balance: tt.balance, weight: 999, slotCommittee: tt.indices}
			s := &Store{
				committeeWeight:            10000,
				weakHeadCommitteeWeight:    tt.committee,
				justifiedEffectiveBalances: tt.effective,
				slashedIndices:             tt.slashed,
			}
			require.Equal(t, tt.wantWeak, s.isHeadWeak(n))
			require.Equal(t, tt.wantWeak, s.isHeadWeak(n))
			require.Equal(t, tt.balance, n.balance)
			require.Equal(t, uint64(999), n.weight)
			require.Equal(t, uint64(10000), s.committeeWeight)
		})
	}
}

func TestStore_AttestationScore(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.ReorgHeadWeightThreshold = 20
	require.NoError(t, params.SetActive(cfg))

	tests := []struct {
		name      string
		boostRoot [32]byte
	}{
		{name: "no boost"},
		{name: "boost on head", boostRoot: [32]byte{'a'}},
		{name: "boost on empty branch descendant", boostRoot: [32]byte{'b'}},
		{name: "boost on full branch descendant", boostRoot: [32]byte{'c'}},
		{name: "boost outside subtree", boostRoot: [32]byte{'d'}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Node{root: [32]byte{'a'}, balance: 7, weight: 1001}
			b := &Node{root: [32]byte{'b'}, balance: 17, weight: 1002}
			c := &Node{root: [32]byte{'c'}, balance: 29, weight: 1003}
			d := &Node{root: [32]byte{'d'}, balance: 1000, weight: 1004}
			nodes := []*Node{a, b, c, d}
			payloads := []*PayloadNode{
				{node: a, balance: 11, weight: 2001, children: []*Node{b}},
				{node: a, balance: 13, weight: 2002, full: true, children: []*Node{c}},
				{node: b, balance: 19, weight: 2003},
				{node: b, balance: 23, weight: 2004, full: true},
				{node: c, balance: 31, weight: 2005},
				{node: c, balance: 37, weight: 2006, full: true},
				{node: d, balance: 1000, weight: 2007},
			}
			s := &Store{
				weakHeadCommitteeWeight: 1000,
				emptyNodeByRoot:         make(map[[32]byte]*PayloadNode),
				fullNodeByRoot:          make(map[[32]byte]*PayloadNode),
			}
			for _, pn := range payloads {
				if pn.full {
					s.fullNodeByRoot[pn.node.root] = pn
				} else {
					s.emptyNodeByRoot[pn.node.root] = pn
				}
			}
			if tt.boostRoot != [32]byte{} {
				s.previousProposerBoostRoot = tt.boostRoot
				s.previousProposerBoostScore = 40
				s.emptyNodeByRoot[tt.boostRoot].node.balance += 40
			}
			beforeNodes := make([][2]uint64, len(nodes))
			for i, n := range nodes {
				beforeNodes[i] = [2]uint64{n.balance, n.weight}
			}
			beforePayloads := make([][2]uint64, len(payloads))
			for i, pn := range payloads {
				beforePayloads[i] = [2]uint64{pn.balance, pn.weight}
			}

			require.Equal(t, uint64(187), s.attestationScore(a))
			require.Equal(t, true, s.isHeadWeak(a))
			for i, n := range nodes {
				require.Equal(t, beforeNodes[i], [2]uint64{n.balance, n.weight})
			}
			for i, pn := range payloads {
				require.Equal(t, beforePayloads[i], [2]uint64{pn.balance, pn.weight})
			}
		})
	}
}

func TestForkChoice_UpdateJustifiedBalances_RefreshesWeakHeadSnapshot(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.ReorgHeadWeightThreshold = 20
	cfg.SlotsPerEpoch = 32
	require.NoError(t, params.SetActive(cfg))

	f := New()
	n := &Node{root: [32]byte{'a'}, slot: 1, slotCommittee: []primitives.ValidatorIndex{0, 1}}
	f.store.emptyNodeByRoot[n.root] = &PayloadNode{node: n}
	f.store.slashedIndices[1] = true
	f.votes = []Vote{
		{nextRoot: n.root, nextSlot: n.slot},
		{nextRoot: n.root, nextSlot: n.slot},
	}
	filtered := []uint64{180, 0, 31800}
	oldRoot, newRoot := [32]byte{'o'}, [32]byte{'n'}
	snapshots := map[[32]byte]forkchoice.Balances{
		oldRoot: {ActiveNonSlashed: filtered, Effective: []uint64{180, 20, 31800}, TotalActive: 32000},
		newRoot: {ActiveNonSlashed: filtered, Effective: []uint64{180, 10, 31800}, TotalActive: 31990},
	}
	f.SetBalancesByRooter(func(_ context.Context, root [32]byte) (forkchoice.Balances, error) {
		balances, ok := snapshots[root]
		require.Equal(t, true, ok)
		return balances, nil
	})

	for _, tt := range []struct {
		name          string
		root          [32]byte
		weakCommittee uint64
		wantWeak      bool
	}{
		{name: "old checkpoint counts original effective balance", root: oldRoot, weakCommittee: 1000},
		{name: "new checkpoint uses reduced effective balance", root: newRoot, weakCommittee: 999, wantWeak: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, f.updateJustifiedBalances(t.Context(), tt.root))
			require.DeepEqual(t, snapshots[tt.root].Effective, f.store.justifiedEffectiveBalances)
			require.DeepEqual(t, filtered, f.justifiedBalances)
			require.Equal(t, uint64(999), f.store.committeeWeight)
			require.Equal(t, tt.weakCommittee, f.store.weakHeadCommitteeWeight)
			require.NoError(t, f.updateBalances())
			require.NoError(t, f.store.applyWeightChangesConsensusNode(t.Context(), n))
			require.DeepEqual(t, filtered, f.balances)
			require.Equal(t, uint64(180), n.balance)
			require.Equal(t, uint64(180), n.weight)
			require.Equal(t, tt.wantWeak, f.store.isHeadWeak(n))
			require.Equal(t, uint64(180), n.balance)
			require.Equal(t, uint64(180), n.weight)
			require.Equal(t, true, f.store.slashedIndices[1])
		})
	}
}

func TestForkChoice_CacheWeakHeadCommittees(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.ReorgHeadWeightThreshold = 20
	require.NoError(t, params.SetActive(cfg))

	t.Run("lazy branch-specific committees for current and previous slots", func(t *testing.T) {
		f := New()
		driftGenesisTime(f, 10, params.BeaconConfig().SlotDuration()/2)
		a := &Node{root: [32]byte{'a'}, slot: 10, balance: 19}
		b := &Node{root: [32]byte{'b'}, slot: 10, balance: 19}
		previous := &Node{root: [32]byte{'p'}, slot: 9}
		older := &Node{root: [32]byte{'o'}, slot: 8}
		future := &Node{root: [32]byte{'f'}, slot: 11}
		cached := &Node{root: [32]byte{'c'}, slot: 10, slotCommittee: []primitives.ValidatorIndex{4}}
		for _, n := range []*Node{a, b, previous, older, future, cached} {
			f.store.emptyNodeByRoot[n.root] = &PayloadNode{node: n}
		}
		committees := map[[32]byte][][]primitives.ValidatorIndex{
			a.root:        {{1}, {2}},
			b.root:        {{2, 3}},
			previous.root: {},
		}
		calls := make(map[[32]byte]int)
		ctx := t.Context()
		f.SetCommitteesByRooter(func(gotCtx context.Context, root [32]byte, slot primitives.Slot, st state.ReadOnlyBeaconState) ([][]primitives.ValidatorIndex, error) {
			require.Equal(t, ctx, gotCtx)
			require.Equal(t, nil, st)
			expected, ok := committees[root]
			require.Equal(t, true, ok)
			require.Equal(t, f.store.emptyNodeByRoot[root].node.slot, slot)
			calls[root]++
			return expected, nil
		})

		require.NoError(t, f.cacheWeakHeadCommittees(ctx))
		require.Equal(t, 0, len(calls))
		require.Equal(t, true, a.slotCommittee == nil)
		require.Equal(t, true, b.slotCommittee == nil)
		f.store.slashedIndices[1] = true
		require.NoError(t, f.cacheWeakHeadCommittees(ctx))
		require.DeepEqual(t, []primitives.ValidatorIndex{1, 2}, a.slotCommittee)
		require.DeepEqual(t, []primitives.ValidatorIndex{2, 3}, b.slotCommittee)
		require.DeepEqual(t, []primitives.ValidatorIndex{}, previous.slotCommittee)
		require.Equal(t, true, older.slotCommittee == nil)
		require.Equal(t, true, future.slotCommittee == nil)
		require.DeepEqual(t, []primitives.ValidatorIndex{4}, cached.slotCommittee)
		require.DeepEqual(t, map[[32]byte]int{a.root: 1, b.root: 1, previous.root: 1}, calls)

		require.NoError(t, f.cacheWeakHeadCommittees(ctx))
		require.DeepEqual(t, map[[32]byte]int{a.root: 1, b.root: 1, previous.root: 1}, calls)
		f.store.weakHeadCommitteeWeight = 100
		f.store.justifiedEffectiveBalances = []uint64{0, 1, 100, 100, 100}
		require.Equal(t, false, f.store.isHeadWeak(a))
		require.Equal(t, true, f.store.isHeadWeak(b))
	})

	t.Run("cached committees expire outside the two-slot window", func(t *testing.T) {
		f := New()
		f.store.slashedIndices[1] = true
		current := &Node{root: [32]byte{'c'}, slot: 10}
		cached := &Node{root: [32]byte{'a'}, slot: 10, slotCommittee: []primitives.ValidatorIndex{1}}
		empty := &Node{root: [32]byte{'e'}, slot: 10, slotCommittee: []primitives.ValidatorIndex{}}
		previous := &Node{root: [32]byte{'p'}, slot: 9, slotCommittee: []primitives.ValidatorIndex{2}}
		older := &Node{root: [32]byte{'o'}, slot: 8, slotCommittee: []primitives.ValidatorIndex{3}}
		olderEmpty := &Node{root: [32]byte{'z'}, slot: 8, slotCommittee: []primitives.ValidatorIndex{}}
		future := &Node{root: [32]byte{'f'}, slot: 11, slotCommittee: []primitives.ValidatorIndex{4}}
		for _, n := range []*Node{current, cached, empty, previous, older, olderEmpty, future} {
			f.store.emptyNodeByRoot[n.root] = &PayloadNode{node: n}
		}
		calls := 0
		f.SetCommitteesByRooter(func(_ context.Context, root [32]byte, slot primitives.Slot, _ state.ReadOnlyBeaconState) ([][]primitives.ValidatorIndex, error) {
			require.Equal(t, current.root, root)
			require.Equal(t, current.slot, slot)
			calls++
			return [][]primitives.ValidatorIndex{{5}}, nil
		})

		for _, slot := range []primitives.Slot{10, 11, 12, 13} {
			driftGenesisTime(f, slot, params.BeaconConfig().SlotDuration()/2)
			require.NoError(t, f.cacheWeakHeadCommittees(t.Context()))
			require.Equal(t, 1, calls)
			for _, n := range []*Node{current, cached, empty, previous, older, olderEmpty, future} {
				expired := n.slot <= slot && slot-n.slot > 1
				require.Equal(t, expired, n.slotCommittee == nil)
			}
			if slot < 12 {
				require.DeepEqual(t, []primitives.ValidatorIndex{5}, current.slotCommittee)
				require.DeepEqual(t, []primitives.ValidatorIndex{1}, cached.slotCommittee)
				require.DeepEqual(t, []primitives.ValidatorIndex{}, empty.slotCommittee)
			}
			if slot < 13 {
				require.DeepEqual(t, []primitives.ValidatorIndex{4}, future.slotCommittee)
			}
		}
	})

	t.Run("cached committees expire before the first slashing", func(t *testing.T) {
		f := New()
		n := &Node{root: [32]byte{'a'}, slot: 10, slotCommittee: []primitives.ValidatorIndex{1}}
		f.store.emptyNodeByRoot[n.root] = &PayloadNode{node: n}
		driftGenesisTime(f, 12, params.BeaconConfig().SlotDuration()/2)
		require.NoError(t, f.cacheWeakHeadCommittees(t.Context()))
		require.Equal(t, true, n.slotCommittee == nil)
	})

	t.Run("missing provider", func(t *testing.T) {
		tests := []struct {
			name       string
			slot       primitives.Slot
			hasSlashes bool
			cached     bool
			wantErr    bool
		}{
			{name: "current slot", slot: 10, hasSlashes: true, wantErr: true},
			{name: "previous slot", slot: 9, hasSlashes: true, wantErr: true},
			{name: "older slot", slot: 8, hasSlashes: true},
			{name: "future slot", slot: 11, hasSlashes: true},
			{name: "already cached", slot: 10, hasSlashes: true, cached: true},
			{name: "no equivocations", slot: 10},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				f := New()
				driftGenesisTime(f, 10, params.BeaconConfig().SlotDuration()/2)
				n := &Node{root: [32]byte{'a'}, slot: tt.slot}
				if tt.cached {
					n.slotCommittee = []primitives.ValidatorIndex{}
				}
				f.store.emptyNodeByRoot[n.root] = &PayloadNode{node: n}
				if tt.hasSlashes {
					f.store.slashedIndices[1] = true
				}
				err := f.cacheWeakHeadCommittees(t.Context())
				if tt.wantErr {
					require.ErrorContains(t, "missing committee provider", err)
					require.Equal(t, true, n.slotCommittee == nil)
				} else {
					require.NoError(t, err)
				}
			})
		}
	})

	t.Run("unavailable recent branch does not block head computation", func(t *testing.T) {
		f := setupGloas(t, 0, 0)
		ctx := t.Context()
		f.justifiedBalances = []uint64{10, 10}
		f.store.weakHeadCommitteeWeight = 100
		parentRoot, headRoot, alternateRoot := [32]byte{'p'}, [32]byte{'h'}, [32]byte{'a'}
		driftGenesisTime(f, 10, params.BeaconConfig().SlotDuration()/2)
		_, blk, err := prepareGloasForkchoiceState(ctx, 9, parentRoot, [32]byte{}, [32]byte{'P'}, [32]byte{}, 0, 0)
		require.NoError(t, err)
		// InsertChain adds nodes without post-states or eager committee snapshots.
		_, err = f.store.insert(ctx, blk, 0, 0)
		require.NoError(t, err)
		for _, root := range [][32]byte{headRoot, alternateRoot} {
			_, blk, err = prepareGloasForkchoiceState(ctx, 10, root, parentRoot, root, [32]byte{}, 0, 0)
			require.NoError(t, err)
			_, err = f.store.insert(ctx, blk, 0, 0)
			require.NoError(t, err)
		}
		f.ProcessAttestation(ctx, []uint64{0}, headRoot, 10, false)
		f.InsertSlashedIndex(ctx, 1)
		available := false
		calls := make(map[[32]byte]int)
		f.SetCommitteesByRooter(func(_ context.Context, root [32]byte, _ primitives.Slot, _ state.ReadOnlyBeaconState) ([][]primitives.ValidatorIndex, error) {
			calls[root]++
			if root == alternateRoot && !available {
				return nil, nil
			}
			return [][]primitives.ValidatorIndex{}, nil
		})
		alternate := f.store.emptyNodeByRoot[alternateRoot].node
		for range 2 {
			root, err := f.Head(ctx)
			require.NoError(t, err)
			require.Equal(t, headRoot, root)
			require.Equal(t, true, alternate.slotCommittee == nil)
			require.Equal(t, false, f.store.isHeadWeak(alternate))
		}
		require.DeepEqual(t, map[[32]byte]int{parentRoot: 1, headRoot: 1, alternateRoot: 2}, calls)

		available = true
		root, err := f.Head(ctx)
		require.NoError(t, err)
		require.Equal(t, headRoot, root)
		require.DeepEqual(t, []primitives.ValidatorIndex{}, alternate.slotCommittee)
		require.Equal(t, true, f.store.isHeadWeak(alternate))
		require.Equal(t, 3, calls[alternateRoot])
		_, err = f.Head(ctx)
		require.NoError(t, err)
		require.Equal(t, 3, calls[alternateRoot])
	})

	t.Run("provider error is propagated and can be retried", func(t *testing.T) {
		for _, slot := range []primitives.Slot{9, 10} {
			t.Run(fmt.Sprintf("slot %d", slot), func(t *testing.T) {
				f := New()
				driftGenesisTime(f, 10, params.BeaconConfig().SlotDuration()/2)
				n := &Node{root: [32]byte{'a'}, slot: slot}
				f.store.emptyNodeByRoot[n.root] = &PayloadNode{node: n}
				f.store.slashedIndices[1] = true
				wantErr := errors.New("committee lookup failed")
				calls := 0
				f.SetCommitteesByRooter(func(context.Context, [32]byte, primitives.Slot, state.ReadOnlyBeaconState) ([][]primitives.ValidatorIndex, error) {
					calls++
					if calls == 1 {
						return nil, wantErr
					}
					return [][]primitives.ValidatorIndex{{1}}, nil
				})
				require.ErrorIs(t, f.cacheWeakHeadCommittees(t.Context()), wantErr)
				require.Equal(t, true, n.slotCommittee == nil)
				require.NoError(t, f.cacheWeakHeadCommittees(t.Context()))
				require.DeepEqual(t, []primitives.ValidatorIndex{1}, n.slotCommittee)
				require.Equal(t, 2, calls)
			})
		}
	})
}

func TestForkChoice_InsertNode_CachesWeakHeadCommittee(t *testing.T) {
	for _, tt := range []struct {
		name string
		slot primitives.Slot
		want bool
	}{
		{name: "old slot", slot: 8},
		{name: "previous slot", slot: 9, want: true},
		{name: "current slot", slot: 10, want: true},
		{name: "future slot", slot: 11, want: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := setup(0, 0)
			ctx := t.Context()
			driftGenesisTime(f, 10, params.BeaconConfig().SlotDuration()/2)
			root := [32]byte{'a'}
			st, blk, err := prepareForkchoiceState(ctx, tt.slot, root, [32]byte{}, [32]byte{'A'}, 0, 0)
			require.NoError(t, err)
			calls := 0
			f.SetCommitteesByRooter(func(gotCtx context.Context, gotRoot [32]byte, slot primitives.Slot, loaded state.ReadOnlyBeaconState) ([][]primitives.ValidatorIndex, error) {
				require.Equal(t, ctx, gotCtx)
				require.Equal(t, root, gotRoot)
				require.Equal(t, tt.slot, slot)
				require.Equal(t, st, loaded)
				require.Equal(t, false, f.HasNode(root))
				require.Equal(t, 0, len(f.store.slashedIndices))
				calls++
				return [][]primitives.ValidatorIndex{{1, 2}, {3}}, nil
			})
			require.NoError(t, f.InsertNode(ctx, st, blk))
			require.NoError(t, f.InsertNode(ctx, st, blk))
			if tt.want {
				require.Equal(t, 1, calls)
				require.DeepEqual(t, []primitives.ValidatorIndex{1, 2, 3}, f.store.emptyNodeByRoot[root].node.slotCommittee)
			} else {
				require.Equal(t, 0, calls)
				require.Equal(t, true, f.store.emptyNodeByRoot[root].node.slotCommittee == nil)
			}
		})
	}
}
