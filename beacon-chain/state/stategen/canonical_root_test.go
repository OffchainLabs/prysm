package stategen

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/blocks"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db/kv"
	testDB "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	doublylinkedtree "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/doubly-linked-tree"
	"github.com/OffchainLabs/prysm/v7/config/features"
	consensusblocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

// The late-block reorg shape: slot 63 is orphaned, 63 and 64 are canonically empty, and the canonical
// block at 65 builds on 32. HighestRootsBelowSlot(65) then returns only the orphan, so the boundary state
// at 64 must be built from slot 32 instead.
func TestMigrateToColdHdiff_OrphanAtHighestPopulatedSlot(t *testing.T) {
	ctx := t.Context()
	setStateDiffExponents() // [6,5]: slot 64 is a level 0 boundary
	beaconDB := testDB.SetupDB(t)
	require.NoError(t, beaconDB.(*kv.Store).InitStateDiffCacheForTesting(t, 0))
	resetCfg := features.InitWithReset(&features.Flags{EnableStateDiff: true})
	defer resetCfg()
	service := New(beaconDB, doublylinkedtree.New())

	genesisState, pks := util.DeterministicGenesisState(t, 32)
	genesisStateRoot, err := genesisState.HashTreeRoot(ctx)
	require.NoError(t, err)
	genesis := blocks.NewGenesisBlock(genesisStateRoot[:])
	util.SaveBlock(t, ctx, beaconDB, genesis)
	gRoot, err := genesis.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, gRoot))
	require.NoError(t, beaconDB.SaveState(ctx, genesisState, gRoot))
	require.NoError(t, beaconDB.SaveStateSummary(ctx, &ethpb.StateSummary{Slot: 0, Root: gRoot[:]}))

	// Canonical block at slot 32.
	b32, err := util.GenerateFullBlock(genesisState, pks, util.DefaultBlockGenConfig(), 32)
	require.NoError(t, err)
	wsb32, err := consensusblocks.NewSignedBeaconBlock(b32)
	require.NoError(t, err)
	s32, err := executeStateTransitionStateGen(ctx, genesisState, wsb32)
	require.NoError(t, err)
	r32, err := b32.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, beaconDB, b32)
	require.NoError(t, beaconDB.SaveStateSummary(ctx, &ethpb.StateSummary{Slot: 32, Root: r32[:]}))

	// A valid child of slot 32 that lost the fork choice race. Nothing ever deletes it.
	bOrphan, err := util.GenerateFullBlock(s32.Copy(), pks, util.DefaultBlockGenConfig(), 63)
	require.NoError(t, err)
	wsbOrphan, err := consensusblocks.NewSignedBeaconBlock(bOrphan)
	require.NoError(t, err)
	sOrphan, err := executeStateTransitionStateGen(ctx, s32.Copy(), wsbOrphan)
	require.NoError(t, err)
	rOrphan, err := bOrphan.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, beaconDB, bOrphan)
	require.NoError(t, beaconDB.SaveStateSummary(ctx, &ethpb.StateSummary{Slot: 63, Root: rOrphan[:]}))

	// Builds on slot 32, so 63 is not an ancestor of it.
	b65, err := util.GenerateFullBlock(s32.Copy(), pks, util.DefaultBlockGenConfig(), 65)
	require.NoError(t, err)
	require.DeepEqual(t, r32[:], b65.Block.ParentRoot, "slot 65 must build on slot 32, making 63 an orphan")
	wsb65, err := consensusblocks.NewSignedBeaconBlock(b65)
	require.NoError(t, err)
	r65, err := b65.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, beaconDB, b65)
	require.NoError(t, beaconDB.SaveStateSummary(ctx, &ethpb.StateSummary{Slot: 65, Root: r65[:]}))
	_ = wsb65

	// The blockchain service saves the finalized checkpoint before calling MigrateToCold, so the finalized
	// index already holds the ancestry 65 -> 32 -> genesis, and never the orphan.
	require.NoError(t, beaconDB.SaveFinalizedCheckpoint(ctx, &ethpb.Checkpoint{Epoch: 2, Root: r65[:]}))

	// The db returns exactly one root below the boundary, and it is the orphan.
	high, roots, err := beaconDB.HighestRootsBelowSlot(ctx, 65)
	require.NoError(t, err)
	require.Equal(t, 1, len(roots), "a len(roots) != 1 guard would not fire")
	require.Equal(t, primitives.Slot(63), high)
	require.Equal(t, rOrphan, roots[0])
	// Retained, but not indexed as canonical.
	require.Equal(t, false, beaconDB.IsFinalizedBlock(ctx, rOrphan))

	// Slot 32 advanced with no blocks in between.
	wantState, err := transition.ProcessSlots(ctx, s32.Copy(), 64)
	require.NoError(t, err)
	wantRoot, err := wantState.HashTreeRoot(ctx)
	require.NoError(t, err)

	// What it becomes if the orphan is replayed.
	orphanState, err := transition.ProcessSlots(ctx, sOrphan.Copy(), 64)
	require.NoError(t, err)
	orphanRoot, err := orphanState.HashTreeRoot(ctx)
	require.NoError(t, err)
	require.NotEqual(t, wantRoot, orphanRoot, "the two candidates must differ for this test to mean anything")

	// Slot 64 is the only boundary in (32, 65), and is absent from the cache to force the lookup path.
	service.finalizedInfo = &finalizedInfo{slot: 32, root: r32, state: s32.Copy()}
	require.NoError(t, service.epochBoundaryStateCache.put(r32, s32.Copy()))
	require.NoError(t, service.MigrateToCold(ctx, r65))

	// State() resolves by the summary's slot, so any root mapped to 64 reads the tree at 64.
	probe := [32]byte{0xaa}
	require.NoError(t, beaconDB.SaveStateSummary(ctx, &ethpb.StateSummary{Slot: 64, Root: probe[:]}))
	got, err := beaconDB.State(ctx, probe)
	require.NoError(t, err)
	require.NotNil(t, got)
	gotRoot, err := got.HashTreeRoot(ctx)
	require.NoError(t, err)

	if gotRoot == orphanRoot {
		t.Fatalf("boundary state at slot 64 was built from the orphaned block at slot 63 "+
			"(got %#x, canonical is %#x)", gotRoot, wantRoot)
	}
	require.Equal(t, wantRoot, gotRoot)
}

// A canonical block must win over a reorged sibling at the same slot rather than the ambiguity failing.
func TestCanonicalRootAtOrBelow(t *testing.T) {
	ctx := t.Context()
	beaconDB := testDB.SetupDB(t)
	service := New(beaconDB, doublylinkedtree.New())

	genesisState, pks := util.DeterministicGenesisState(t, 32)
	genesisStateRoot, err := genesisState.HashTreeRoot(ctx)
	require.NoError(t, err)
	genesis := blocks.NewGenesisBlock(genesisStateRoot[:])
	util.SaveBlock(t, ctx, beaconDB, genesis)
	gRoot, err := genesis.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, gRoot))

	b32, err := util.GenerateFullBlock(genesisState, pks, util.DefaultBlockGenConfig(), 32)
	require.NoError(t, err)
	wsb32, err := consensusblocks.NewSignedBeaconBlock(b32)
	require.NoError(t, err)
	s32, err := executeStateTransitionStateGen(ctx, genesisState, wsb32)
	require.NoError(t, err)
	r32, err := b32.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, beaconDB, b32)

	// A canonical block at slot 40 and a reorged sibling at the same slot.
	b40, err := util.GenerateFullBlock(s32.Copy(), pks, util.DefaultBlockGenConfig(), 40)
	require.NoError(t, err)
	wsb40, err := consensusblocks.NewSignedBeaconBlock(b40)
	require.NoError(t, err)
	r40, err := b40.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, beaconDB, b40)

	sibling, err := util.GenerateFullBlock(s32.Copy(), pks, util.DefaultBlockGenConfig(), 40)
	require.NoError(t, err)
	sibling.Block.Body.Graffiti = bytesutil.PadTo([]byte("sibling"), 32)
	rSibling, err := sibling.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NotEqual(t, r40, rSibling)
	util.SaveBlock(t, ctx, beaconDB, sibling)

	// Two epochs above the contested slot: IsFinalizedBlock reports non-canonical blocks inside the
	// finalized epoch as finalized, so slot 40 has to sit below that window. migrateToColdHdiff only
	// migrates slots below the checkpoint slot, which is exactly that window.
	s40, err := executeStateTransitionStateGen(ctx, s32.Copy(), wsb40)
	require.NoError(t, err)
	b64, err := util.GenerateFullBlock(s40.Copy(), pks, util.DefaultBlockGenConfig(), 64)
	require.NoError(t, err)
	r64, err := b64.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, beaconDB, b64)
	require.NoError(t, beaconDB.SaveFinalizedCheckpoint(ctx, &ethpb.Checkpoint{Epoch: 2, Root: r64[:]}))
	require.Equal(t, false, beaconDB.IsFinalizedBlock(ctx, rSibling))
	require.Equal(t, true, beaconDB.IsFinalizedBlock(ctx, r40))

	// Ambiguous in the slot index, unambiguous in the finalized index.
	got, err := service.canonicalRootAtOrBelow(ctx, 48, 0)
	require.NoError(t, err)
	require.Equal(t, r40, got)

	// Still returns the highest canonical block at or below the slot.
	got, err = service.canonicalRootAtOrBelow(ctx, 39, 0)
	require.NoError(t, err)
	require.Equal(t, r32, got)

	// A floor above every candidate errors rather than walking down to genesis.
	_, err = service.canonicalRootAtOrBelow(ctx, 39, 39)
	require.ErrorIs(t, err, errUnknownBlock)
}
