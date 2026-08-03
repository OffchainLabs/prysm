package stategen

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/blocks"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db/kv"
	testDB "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	doublylinkedtree "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/doubly-linked-tree"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

// While the archive walk owns the tree, the live chain must not migrate cold states into it: the anchors the
// live region needs lie in the not yet regenerated past.
func TestMigrateToCold_SuppressedWhileArchivePending(t *testing.T) {
	ctx := t.Context()
	setStateDiffExponents()
	beaconDB := testDB.SetupDB(t)
	require.NoError(t, beaconDB.(*kv.Store).InitStateDiffCacheForTesting(t, 0))
	resetCfg := features.InitWithReset(&features.Flags{EnableStateDiff: true})
	defer resetCfg()

	service := New(beaconDB, doublylinkedtree.New())
	service.SetArchivePending(true)
	require.Equal(t, true, service.ArchivePending())

	// A finalized root that does not exist in the db: MigrateToCold would fail looking it up, so returning
	// nil proves it never got that far.
	require.NoError(t, service.MigrateToCold(ctx, [32]byte{1}))

	service.SetArchivePending(false)
	require.NotNil(t, service.MigrateToCold(ctx, [32]byte{1}))
}

// ForceCheckpoint runs on every graceful shutdown. Against a re-based offset the tree write would either fail
// or clear the walk's anchors, so it must persist by root instead.
func TestForceCheckpoint_WritesSnapshotWhileArchivePending(t *testing.T) {
	ctx := t.Context()
	setStateDiffExponents()
	beaconDB := testDB.SetupDB(t)
	require.NoError(t, beaconDB.(*kv.Store).InitStateDiffCacheForTesting(t, 0))
	resetCfg := features.InitWithReset(&features.Flags{EnableStateDiff: true})
	defer resetCfg()

	service := New(beaconDB, doublylinkedtree.New())
	service.SetArchivePending(true)

	st, _ := util.DeterministicGenesisState(t, 32)
	require.NoError(t, st.SetSlot(64))
	root := [32]byte{2}
	service.hotStateCache.put(root, st)

	require.NoError(t, service.ForceCheckpoint(ctx, root[:]))
	require.Equal(t, true, beaconDB.(*kv.Store).HasHotStateSnapshot(ctx, root))
}

// Clearing the bucket would drop the checkpoint origin state and the resume snapshots that backfill and the
// next boot depend on.
func TestDisableSaveHotStateToDB_KeepsSnapshotsWhileArchivePending(t *testing.T) {
	ctx := t.Context()
	setStateDiffExponents()
	beaconDB := testDB.SetupDB(t)
	require.NoError(t, beaconDB.(*kv.Store).InitStateDiffCacheForTesting(t, 0))
	resetCfg := features.InitWithReset(&features.Flags{EnableStateDiff: true})
	defer resetCfg()

	service := New(beaconDB, doublylinkedtree.New())
	service.SetArchivePending(true)
	service.EnableSaveHotStateToDB(ctx)

	st, _ := util.DeterministicGenesisState(t, 32)
	root := [32]byte{3}
	require.NoError(t, beaconDB.(*kv.Store).SaveHotStateSnapshot(ctx, st, root))

	require.NoError(t, service.DisableSaveHotStateToDB(ctx))
	require.Equal(t, true, beaconDB.(*kv.Store).HasHotStateSnapshot(ctx, root))

	// Once regeneration is done the normal clearing behavior returns.
	service.SetArchivePending(false)
	service.EnableSaveHotStateToDB(ctx)
	require.NoError(t, service.DisableSaveHotStateToDB(ctx))
	require.Equal(t, false, beaconDB.(*kv.Store).HasHotStateSnapshot(ctx, root))
}

// Resume snapshots are written independently of saveHotStateDB, since checkSaveHotStateDB turns that mode off
// on every block while finality is healthy. Only the newest is kept.
func TestSaveArchiveResumeSnapshot_KeepsOnlyNewest(t *testing.T) {
	ctx := t.Context()
	setStateDiffExponents()
	beaconDB := testDB.SetupDB(t)
	require.NoError(t, beaconDB.(*kv.Store).InitStateDiffCacheForTesting(t, 0))
	resetCfg := features.InitWithReset(&features.Flags{EnableStateDiff: true})
	defer resetCfg()

	service := New(beaconDB, doublylinkedtree.New())
	service.SetArchivePending(true)

	st, _ := util.DeterministicGenesisState(t, 32)
	first := [32]byte{4}
	require.NoError(t, st.SetSlot(archiveResumeSnapshotInterval))
	require.NoError(t, service.saveArchiveResumeSnapshot(ctx, first, st))
	require.Equal(t, true, beaconDB.(*kv.Store).HasHotStateSnapshot(ctx, first))

	// A slot that is not on the interval is skipped.
	skipped := [32]byte{5}
	require.NoError(t, st.SetSlot(archiveResumeSnapshotInterval+1))
	require.NoError(t, service.saveArchiveResumeSnapshot(ctx, skipped, st))
	require.Equal(t, false, beaconDB.(*kv.Store).HasHotStateSnapshot(ctx, skipped))

	second := [32]byte{6}
	require.NoError(t, st.SetSlot(2*archiveResumeSnapshotInterval))
	require.NoError(t, service.saveArchiveResumeSnapshot(ctx, second, st))
	require.Equal(t, true, beaconDB.(*kv.Store).HasHotStateSnapshot(ctx, second))
	require.Equal(t, false, beaconDB.(*kv.Store).HasHotStateSnapshot(ctx, first))
}

// The handoff must not happen until every boundary at or below the finalized slot has been written, otherwise
// a live write can resolve to an anchor the walk never produced.
func TestCompleteArchiveRegeneration(t *testing.T) {
	ctx := t.Context()
	setStateDiffExponents()
	beaconDB := testDB.SetupDB(t)
	require.NoError(t, beaconDB.(*kv.Store).InitStateDiffCacheForTesting(t, 0))
	resetCfg := features.InitWithReset(&features.Flags{EnableStateDiff: true})
	defer resetCfg()

	service := New(beaconDB, doublylinkedtree.New())
	service.SetArchivePending(true)

	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	fEpoch := primitives.Epoch(4)
	fSlot := primitives.Slot(fEpoch) * slotsPerEpoch

	st, _ := util.DeterministicGenesisState(t, 32)
	genesisStateRoot, err := st.HashTreeRoot(ctx)
	require.NoError(t, err)
	genesisBlk := blocks.NewGenesisBlock(genesisStateRoot[:])
	util.SaveBlock(t, ctx, beaconDB, genesisBlk)
	gRoot, err := genesisBlk.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, gRoot))

	require.NoError(t, st.SetSlot(fSlot))
	stRoot, err := st.HashTreeRoot(ctx)
	require.NoError(t, err)
	blk := util.NewBeaconBlock()
	blk.Block.Slot = fSlot
	blk.Block.ParentRoot = gRoot[:]
	blk.Block.StateRoot = stRoot[:]
	util.SaveBlock(t, ctx, beaconDB, blk)
	fRoot, err := blk.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, beaconDB.SaveStateSummary(ctx, &ethpb.StateSummary{Slot: fSlot, Root: fRoot[:]}))
	require.NoError(t, beaconDB.SaveState(ctx, st, fRoot))
	require.NoError(t, beaconDB.SaveFinalizedCheckpoint(ctx, &ethpb.Checkpoint{Epoch: fEpoch, Root: fRoot[:]}))

	// Not yet: the next unwritten boundary is still below the finalized slot.
	done, err := service.CompleteArchiveRegeneration(ctx, fSlot-1)
	require.NoError(t, err)
	require.Equal(t, false, done)
	require.Equal(t, true, service.ArchivePending())

	done, err = service.CompleteArchiveRegeneration(ctx, fSlot+1)
	require.NoError(t, err)
	require.Equal(t, true, done)
	require.Equal(t, false, service.ArchivePending())
	require.Equal(t, fSlot, service.finalizedInfo.slot)
	require.Equal(t, fRoot, service.finalizedInfo.root)
}
