package archive

import (
	"context"
	"testing"

	coreblocks "github.com/OffchainLabs/prysm/v7/beacon-chain/core/blocks"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db/kv"
	testDB "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

func TestNextBoundary(t *testing.T) {
	setStateDiffExponents([]int{11, 9, 5})

	require.Equal(t, primitives.Slot(32), nextBoundary(0, 0))
	require.Equal(t, primitives.Slot(32), nextBoundary(0, 1))
	require.Equal(t, primitives.Slot(32), nextBoundary(0, 31))
	require.Equal(t, primitives.Slot(64), nextBoundary(0, 32))
	require.Equal(t, primitives.Slot(2048+32), nextBoundary(0, 2048))

	// Boundaries are relative to the offset.
	require.Equal(t, primitives.Slot(1024+32), nextBoundary(1024, 1024))
	require.Equal(t, primitives.Slot(1024+64), nextBoundary(1024, 1024+32))
	// A slot below the offset resolves to the offset itself.
	require.Equal(t, primitives.Slot(1024), nextBoundary(1024, 0))

	// A coarser deepest level spaces boundaries further apart.
	setStateDiffExponents([]int{11, 9})
	require.Equal(t, primitives.Slot(512), nextBoundary(0, 0))
	require.Equal(t, primitives.Slot(512), nextBoundary(0, 511))
	require.Equal(t, primitives.Slot(1024), nextBoundary(0, 512))
}

// The walk must produce a state at every tree boundary in the range, including boundaries whose slot had no
// block: a hole in a level makes every deeper read inside that span unresolvable.
func TestWalk_SavesEveryBoundaryIncludingSkippedSlots(t *testing.T) {
	// One epoch per boundary, with a shallow tree so the test stays fast.
	setStateDiffExponents([]int{11, 9, 5})
	resetCfg := features.InitWithReset(&features.Flags{EnableStateDiff: true, EnableArchive: true})
	defer resetCfg()

	ctx := t.Context()
	store := testDB.SetupDB(t).(*kv.Store)
	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	target := 3 * slotsPerEpoch

	genesisState, keys := util.DeterministicGenesisState(t, 64)
	require.NoError(t, store.InitializeArchiveOrigin(ctx, genesisState))
	// Skip the block at the second boundary so the walk has to advance slots past a missed block.
	want := buildChain(t, ctx, store, genesisState, keys, target, map[primitives.Slot]bool{2 * slotsPerEpoch: true})

	svc := newTestService(t, store)
	require.NoError(t, svc.walk(ctx, target))

	as, err := store.ArchiveStatus(ctx)
	require.NoError(t, err)
	require.Equal(t, target, as.RegeneratedThroughSlot)

	for slot := slotsPerEpoch; slot <= target; slot += slotsPerEpoch {
		got, err := store.StateBySlotFromDiffTree(ctx, slot)
		require.NoError(t, err, "boundary slot %d", slot)
		require.Equal(t, slot, got.Slot())
		gotRoot, err := got.HashTreeRoot(ctx)
		require.NoError(t, err)
		require.Equal(t, want[slot], gotRoot, "state at boundary slot %d differs", slot)
	}
}

// A resumed walk must pick up from the recorded frontier and produce the same tree as an uninterrupted run.
func TestWalk_ResumesFromFrontier(t *testing.T) {
	setStateDiffExponents([]int{11, 9, 5})
	resetCfg := features.InitWithReset(&features.Flags{EnableStateDiff: true, EnableArchive: true})
	defer resetCfg()

	ctx := t.Context()
	store := testDB.SetupDB(t).(*kv.Store)
	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	target := 3 * slotsPerEpoch

	genesisState, keys := util.DeterministicGenesisState(t, 64)
	require.NoError(t, store.InitializeArchiveOrigin(ctx, genesisState))
	want := buildChain(t, ctx, store, genesisState, keys, target, nil)

	// Stop after the first boundary, then resume from a service that only knows the persisted status.
	require.NoError(t, newTestService(t, store).walk(ctx, slotsPerEpoch))
	as, err := store.ArchiveStatus(ctx)
	require.NoError(t, err)
	require.Equal(t, slotsPerEpoch, as.RegeneratedThroughSlot)

	require.NoError(t, newTestService(t, store).walk(ctx, target))

	for slot := slotsPerEpoch; slot <= target; slot += slotsPerEpoch {
		got, err := store.StateBySlotFromDiffTree(ctx, slot)
		require.NoError(t, err)
		gotRoot, err := got.HashTreeRoot(ctx)
		require.NoError(t, err)
		require.Equal(t, want[slot], gotRoot, "state at boundary slot %d differs after resume", slot)
	}
}

// A wrong origin state produces a state root that does not match what the first block commits to. This is the
// only verification of the operator-supplied origin state, so it must fire.
func TestWalk_RejectsWrongOriginState(t *testing.T) {
	setStateDiffExponents([]int{11, 9, 5})
	resetCfg := features.InitWithReset(&features.Flags{EnableStateDiff: true, EnableArchive: true})
	defer resetCfg()

	ctx := t.Context()
	store := testDB.SetupDB(t).(*kv.Store)
	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch

	genesisState, keys := util.DeterministicGenesisState(t, 64)
	buildChain(t, ctx, store, genesisState, keys, slotsPerEpoch, nil)

	// Anchor the tree with a state that is not the real state at slot 0.
	tampered := genesisState.Copy()
	balances := tampered.Balances()
	balances[0] += 1
	require.NoError(t, tampered.SetBalances(balances))
	require.NoError(t, store.InitializeArchiveOrigin(ctx, tampered))

	// The replay cannot succeed: the block commits to a state root derived from the real origin, so either the
	// header's parent-root check or the state-root check rejects it.
	require.NotNil(t, newTestService(t, store).walk(ctx, slotsPerEpoch))
}

func newTestService(t *testing.T, store *kv.Store) *Service {
	as, err := store.ArchiveStatus(t.Context())
	require.NoError(t, err)
	svc := New(t.Context(), store, &mockStateManager{pending: true}, nil, nil)
	svc.setStatus(as)
	return svc
}

// buildChain fills the db with a canonical chain from st up to target, skipping the given slots, and returns
// the state root at every slot, processed exactly to that slot.
func buildChain(
	t *testing.T,
	ctx context.Context,
	store *kv.Store,
	st state.BeaconState,
	keys []bls.SecretKey,
	target primitives.Slot,
	skip map[primitives.Slot]bool,
) map[primitives.Slot][32]byte {
	genesisRoot, err := st.HashTreeRoot(ctx)
	require.NoError(t, err)
	genesisBlk := coreblocks.NewGenesisBlock(genesisRoot[:])
	util.SaveBlock(t, ctx, store, genesisBlk)
	gRoot, err := genesisBlk.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, store.SaveGenesisBlockRoot(ctx, gRoot))

	roots := make(map[primitives.Slot][32]byte)
	// working stays at the slot of the most recent block: GenerateFullBlock derives the parent state root
	// from the state it is handed, so it must not be pre-advanced across a skipped slot.
	working := st.Copy()
	for slot := primitives.Slot(1); slot <= target; slot++ {
		if skip[slot] {
			advanced, err := transition.ProcessSlots(ctx, working.Copy(), slot)
			require.NoError(t, err)
			stRoot, err := advanced.HashTreeRoot(ctx)
			require.NoError(t, err)
			roots[slot] = stRoot
			continue
		}
		blk, err := util.GenerateFullBlock(working, keys, util.DefaultBlockGenConfig(), slot)
		require.NoError(t, err)
		wsb, err := blocks.NewSignedBeaconBlock(blk)
		require.NoError(t, err)
		working, err = transition.ExecuteStateTransition(ctx, working, wsb)
		require.NoError(t, err)
		require.NoError(t, store.SaveBlock(ctx, wsb))
		root, err := wsb.Block().HashTreeRoot()
		require.NoError(t, err)
		require.NoError(t, store.SaveStateSummary(ctx, &ethpb.StateSummary{Slot: slot, Root: root[:]}))
		stRoot, err := working.HashTreeRoot(ctx)
		require.NoError(t, err)
		roots[slot] = stRoot
	}
	return roots
}

func setStateDiffExponents(exponents []int) {
	flags.Init(&flags.GlobalFlags{StateDiffExponents: exponents})
}

type mockStateManager struct {
	nextArg primitives.Slot
	pending bool
	handoff bool
}

func (m *mockStateManager) ArchivePending() bool     { return m.pending }
func (m *mockStateManager) SetArchivePending(p bool) { m.pending = p }

func (m *mockStateManager) CompleteArchiveRegeneration(_ context.Context, next primitives.Slot) (bool, error) {
	m.nextArg = next
	if m.handoff {
		m.pending = false
	}
	return m.handoff, nil
}
