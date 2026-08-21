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
	"github.com/OffchainLabs/prysm/v7/time/slots"
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

// The late-block reorg shape: the highest slot holding any block at or below a boundary holds only a block
// that lost fork choice. Orphans are never removed from the slot index, so the boundary state has to be built
// from the canonical block below it. Picking the orphan writes a state no chain ever had, and the walk then
// dies on the next boundary because the canonical chain does not descend from it.
func TestWalk_SkipsOrphanAtHighestPopulatedSlot(t *testing.T) {
	setStateDiffExponents([]int{11, 9, 5})
	resetCfg := features.InitWithReset(&features.Flags{EnableStateDiff: true, EnableArchive: true})
	defer resetCfg()

	ctx := t.Context()
	store := testDB.SetupDB(t).(*kv.Store)

	genesisState, keys := util.DeterministicGenesisState(t, 64)
	require.NoError(t, store.InitializeArchiveOrigin(ctx, genesisState))
	saveGenesisBlock(t, ctx, store, genesisState)

	// Canonical chain up to slot 30.
	working := genesisState.Copy()
	for slot := primitives.Slot(1); slot <= 30; slot++ {
		working, _ = applyBlock(t, ctx, store, working, keys, slot)
	}
	stateAt30 := working.Copy()

	// A valid child of slot 30 that lost the fork choice race, at slot 31. Nothing ever deletes it, and it is
	// the only block in (30, 32]: slots 31 and 32 are canonically empty.
	orphan, err := util.GenerateFullBlock(stateAt30.Copy(), keys, util.DefaultBlockGenConfig(), 31)
	require.NoError(t, err)
	orphanRoot := saveBlock(t, ctx, store, orphan)

	// The canonical branch resumes at 33, building on slot 30.
	canonical := stateAt30.Copy()
	var wantRoot64, tipRoot [32]byte
	var tipSlot primitives.Slot
	for slot := primitives.Slot(33); slot <= 70; slot++ {
		canonical, tipRoot = applyBlock(t, ctx, store, canonical, keys, slot)
		tipSlot = slot
		if slot == 64 {
			wantRoot64, err = canonical.HashTreeRoot(ctx)
			require.NoError(t, err)
		}
	}
	// Slot 31 has to sit below the finalized epoch: IsFinalizedBlock reports every block inside the finalized
	// epoch as finalized whether or not it is canonical.
	indexFinalized(t, ctx, store, tipSlot, tipRoot)
	require.Equal(t, false, store.IsFinalizedBlock(ctx, orphanRoot))

	// Boundary 32 is a canonically empty slot, so its state is slot 30's advanced to 32.
	wantAt32, err := transition.ProcessSlots(ctx, stateAt30.Copy(), 32)
	require.NoError(t, err)
	wantRoot32, err := wantAt32.HashTreeRoot(ctx)
	require.NoError(t, err)

	// Both boundaries must regenerate, which they cannot do if the orphan is replayed.
	require.NoError(t, newTestService(t, store).walk(ctx, 64))

	got32, err := store.StateBySlotFromDiffTree(ctx, 32)
	require.NoError(t, err)
	gotRoot32, err := got32.HashTreeRoot(ctx)
	require.NoError(t, err)
	require.Equal(t, wantRoot32, gotRoot32, "boundary 32 was not built from the canonical block at slot 30")

	got64, err := store.StateBySlotFromDiffTree(ctx, 64)
	require.NoError(t, err)
	gotRoot64, err := got64.HashTreeRoot(ctx)
	require.NoError(t, err)
	require.Equal(t, wantRoot64, gotRoot64, "boundary 64 differs from the canonical chain")
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

	// Anchor the tree with a state that is not the real state at slot 0. Anchored before the chain is built
	// because buildChain finalizes the tip, and marking a checkpoint finalized reads the tree.
	tampered := genesisState.Copy()
	balances := tampered.Balances()
	balances[0] += 1
	require.NoError(t, tampered.SetBalances(balances))
	require.NoError(t, store.InitializeArchiveOrigin(ctx, tampered))

	buildChain(t, ctx, store, genesisState, keys, slotsPerEpoch, nil)

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
	saveGenesisBlock(t, ctx, store, st)

	roots := make(map[primitives.Slot][32]byte)
	// working stays at the slot of the most recent block: GenerateFullBlock derives the parent state root
	// from the state it is handed, so it must not be pre-advanced across a skipped slot.
	working := st.Copy()
	var tipRoot [32]byte
	var tipSlot primitives.Slot
	for slot := primitives.Slot(1); slot <= target; slot++ {
		if skip[slot] {
			advanced, err := transition.ProcessSlots(ctx, working.Copy(), slot)
			require.NoError(t, err)
			stRoot, err := advanced.HashTreeRoot(ctx)
			require.NoError(t, err)
			roots[slot] = stRoot
			continue
		}
		working, tipRoot = applyBlock(t, ctx, store, working, keys, slot)
		tipSlot = slot
		stRoot, err := working.HashTreeRoot(ctx)
		require.NoError(t, err)
		roots[slot] = stRoot
	}
	// The walk resolves canonical blocks through the finalized index, so it has to be populated. On a real
	// node the live chain and BackfillFinalizedIndex do this.
	indexFinalized(t, ctx, store, tipSlot, tipRoot)
	return roots
}

func saveGenesisBlock(t *testing.T, ctx context.Context, store *kv.Store, st state.BeaconState) {
	stRoot, err := st.HashTreeRoot(ctx)
	require.NoError(t, err)
	blk := coreblocks.NewGenesisBlock(stRoot[:])
	util.SaveBlock(t, ctx, store, blk)
	root, err := blk.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, store.SaveGenesisBlockRoot(ctx, root))
}

// applyBlock generates the block at the given slot on top of st, saves it, and returns the post state and the
// block root. st is advanced in place, as ExecuteStateTransition does.
func applyBlock(
	t *testing.T,
	ctx context.Context,
	store *kv.Store,
	st state.BeaconState,
	keys []bls.SecretKey,
	slot primitives.Slot,
) (state.BeaconState, [32]byte) {
	blk, err := util.GenerateFullBlock(st, keys, util.DefaultBlockGenConfig(), slot)
	require.NoError(t, err)
	root := saveBlock(t, ctx, store, blk)
	wsb, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	post, err := transition.ExecuteStateTransition(ctx, st, wsb)
	require.NoError(t, err)
	return post, root
}

// saveBlock stores a block along with the state summary that makes its root addressable, and returns the root.
func saveBlock(t *testing.T, ctx context.Context, store *kv.Store, blk *ethpb.SignedBeaconBlock) [32]byte {
	wsb, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	require.NoError(t, store.SaveBlock(ctx, wsb))
	root, err := wsb.Block().HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, store.SaveStateSummary(ctx, &ethpb.StateSummary{Slot: blk.Block.Slot, Root: root[:]}))
	return root
}

// indexFinalized marks the chain ending at root as finalized, which is what puts its blocks in the
// finalized index. Blocks in the checkpoint's own epoch are indexed wholesale, canonical or not.
func indexFinalized(t *testing.T, ctx context.Context, store *kv.Store, slot primitives.Slot, root [32]byte) {
	require.NoError(t, store.SaveFinalizedCheckpoint(ctx, &ethpb.Checkpoint{
		Epoch: slots.ToEpoch(slot),
		Root:  root[:],
	}))
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
