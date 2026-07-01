package stategen

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/blocks"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db/kv"
	testDB "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	doublylinkedtree "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/doubly-linked-tree"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v7/config/features"
	consensusblocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	logTest "github.com/sirupsen/logrus/hooks/test"
)

func TestMigrateToCold_CanSaveFinalizedInfo(t *testing.T) {
	ctx := t.Context()
	beaconDB := testDB.SetupDB(t)
	service := New(beaconDB, doublylinkedtree.New())
	beaconState, _ := util.DeterministicGenesisState(t, 32)
	b := util.NewBeaconBlock()
	b.Block.Slot = 1
	br, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, service.beaconDB, b)
	require.NoError(t, service.epochBoundaryStateCache.put(br, beaconState))
	require.NoError(t, service.MigrateToCold(ctx, br))

	wanted := &finalizedInfo{state: beaconState, root: br, slot: 1}
	assert.DeepEqual(t, wanted.root, service.finalizedInfo.root)
	assert.Equal(t, wanted.slot, service.finalizedInfo.slot)
	expectedHTR, err := wanted.state.HashTreeRoot(ctx)
	require.NoError(t, err)
	actualHTR, err := service.finalizedInfo.state.HashTreeRoot(ctx)
	require.NoError(t, err)
	assert.DeepEqual(t, expectedHTR, actualHTR)
}

func TestMigrateToCold_HappyPath(t *testing.T) {
	hook := logTest.NewGlobal()
	ctx := t.Context()
	beaconDB := testDB.SetupDB(t)

	service := New(beaconDB, doublylinkedtree.New())
	service.slotsPerArchivedPoint = 1
	beaconState, _ := util.DeterministicGenesisState(t, 32)
	stateSlot := primitives.Slot(1)
	require.NoError(t, beaconState.SetSlot(stateSlot))
	b := util.NewBeaconBlock()
	b.Block.Slot = 2
	fRoot, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, service.beaconDB, b)
	require.NoError(t, service.epochBoundaryStateCache.put(fRoot, beaconState))
	require.NoError(t, service.MigrateToCold(ctx, fRoot))

	gotState, err := service.beaconDB.State(ctx, fRoot)
	require.NoError(t, err)
	assert.DeepSSZEqual(t, beaconState.ToProtoUnsafe(), gotState.ToProtoUnsafe(), "Did not save state")
	gotRoot := service.beaconDB.ArchivedPointRoot(ctx, stateSlot/service.slotsPerArchivedPoint)
	assert.Equal(t, fRoot, gotRoot, "Did not save archived root")
	lastIndex, err := service.beaconDB.LastArchivedSlot(ctx)
	require.NoError(t, err)
	assert.Equal(t, primitives.Slot(1), lastIndex, "Did not save last archived index")

	require.LogsContain(t, hook, "Saved state in DB")
}

func TestMigrateToCold_RegeneratePath(t *testing.T) {
	hook := logTest.NewGlobal()
	ctx := t.Context()
	beaconDB := testDB.SetupDB(t)

	service := New(beaconDB, doublylinkedtree.New())
	service.slotsPerArchivedPoint = 1
	beaconState, pks := util.DeterministicGenesisState(t, 32)
	genesisStateRoot, err := beaconState.HashTreeRoot(ctx)
	require.NoError(t, err)
	genesis := blocks.NewGenesisBlock(genesisStateRoot[:])
	util.SaveBlock(t, ctx, beaconDB, genesis)
	gRoot, err := genesis.Block.HashTreeRoot()
	require.NoError(t, err)
	assert.NoError(t, beaconDB.SaveState(ctx, beaconState, gRoot))
	assert.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, gRoot))

	b1, err := util.GenerateFullBlock(beaconState, pks, util.DefaultBlockGenConfig(), 1)
	require.NoError(t, err)
	r1, err := b1.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, service.beaconDB, b1)
	require.NoError(t, service.beaconDB.SaveStateSummary(ctx, &ethpb.StateSummary{Slot: 1, Root: r1[:]}))

	b4, err := util.GenerateFullBlock(beaconState, pks, util.DefaultBlockGenConfig(), 4)
	require.NoError(t, err)
	r4, err := b4.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, service.beaconDB, b4)
	require.NoError(t, service.beaconDB.SaveStateSummary(ctx, &ethpb.StateSummary{Slot: 4, Root: r4[:]}))
	service.finalizedInfo = &finalizedInfo{
		slot:  0,
		root:  genesisStateRoot,
		state: beaconState,
	}

	require.NoError(t, service.MigrateToCold(ctx, r4))

	s1, err := service.beaconDB.State(ctx, r1)
	require.NoError(t, err)
	assert.Equal(t, s1.Slot(), primitives.Slot(1), "Did not save state")
	gotRoot := service.beaconDB.ArchivedPointRoot(ctx, 1/service.slotsPerArchivedPoint)
	assert.Equal(t, r1, gotRoot, "Did not save archived root")
	lastIndex, err := service.beaconDB.LastArchivedSlot(ctx)
	require.NoError(t, err)
	assert.Equal(t, primitives.Slot(1), lastIndex, "Did not save last archived index")

	require.LogsContain(t, hook, "Saved state in DB")
}

func TestMigrateToCold_StateExistsInDB(t *testing.T) {
	hook := logTest.NewGlobal()
	ctx := t.Context()
	beaconDB := testDB.SetupDB(t)

	service := New(beaconDB, doublylinkedtree.New())
	service.slotsPerArchivedPoint = 1
	beaconState, _ := util.DeterministicGenesisState(t, 32)
	stateSlot := primitives.Slot(1)
	require.NoError(t, beaconState.SetSlot(stateSlot))
	b := util.NewBeaconBlock()
	b.Block.Slot = 2
	fRoot, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, service.beaconDB, b)
	require.NoError(t, service.epochBoundaryStateCache.put(fRoot, beaconState))
	require.NoError(t, service.beaconDB.SaveState(ctx, beaconState, fRoot))

	service.saveHotStateDB.blockRootsOfSavedStates = [][32]byte{{1}, {2}, {3}, {4}, fRoot}
	require.NoError(t, service.MigrateToCold(ctx, fRoot))
	assert.DeepEqual(t, [][32]byte{{1}, {2}, {3}, {4}}, service.saveHotStateDB.blockRootsOfSavedStates)
	assert.LogsDoNotContain(t, hook, "Saved state in DB")
}

func TestMigrateToCold_ParallelCalls(t *testing.T) {
	hook := logTest.NewGlobal()
	ctx := t.Context()
	beaconDB := testDB.SetupDB(t)

	service := New(beaconDB, doublylinkedtree.New())
	service.slotsPerArchivedPoint = 1
	beaconState, pks := util.DeterministicGenesisState(t, 32)
	genState := beaconState.Copy()
	genesisStateRoot, err := beaconState.HashTreeRoot(ctx)
	require.NoError(t, err)
	genesis := blocks.NewGenesisBlock(genesisStateRoot[:])
	util.SaveBlock(t, ctx, beaconDB, genesis)
	gRoot, err := genesis.Block.HashTreeRoot()
	require.NoError(t, err)
	assert.NoError(t, beaconDB.SaveState(ctx, beaconState, gRoot))
	assert.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, gRoot))

	b1, err := util.GenerateFullBlock(beaconState, pks, util.DefaultBlockGenConfig(), 1)
	require.NoError(t, err)
	wB1, err := consensusblocks.NewSignedBeaconBlock(b1)
	require.NoError(t, err)
	beaconState, err = executeStateTransitionStateGen(ctx, beaconState, wB1)
	assert.NoError(t, err)
	r1, err := b1.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, service.beaconDB, b1)
	require.NoError(t, service.beaconDB.SaveStateSummary(ctx, &ethpb.StateSummary{Slot: 1, Root: r1[:]}))

	b4, err := util.GenerateFullBlock(beaconState, pks, util.DefaultBlockGenConfig(), 4)
	require.NoError(t, err)
	wB4, err := consensusblocks.NewSignedBeaconBlock(b4)
	require.NoError(t, err)
	beaconState, err = executeStateTransitionStateGen(ctx, beaconState, wB4)
	assert.NoError(t, err)
	r4, err := b4.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, service.beaconDB, b4)
	require.NoError(t, service.beaconDB.SaveStateSummary(ctx, &ethpb.StateSummary{Slot: 4, Root: r4[:]}))

	b7, err := util.GenerateFullBlock(beaconState, pks, util.DefaultBlockGenConfig(), 7)
	require.NoError(t, err)
	wB7, err := consensusblocks.NewSignedBeaconBlock(b7)
	require.NoError(t, err)
	_, err = executeStateTransitionStateGen(ctx, beaconState, wB7)
	assert.NoError(t, err)
	r7, err := b7.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, service.beaconDB, b7)
	require.NoError(t, service.beaconDB.SaveStateSummary(ctx, &ethpb.StateSummary{Slot: 7, Root: r7[:]}))

	service.finalizedInfo = &finalizedInfo{
		slot:  0,
		root:  genesisStateRoot,
		state: genState,
	}
	service.saveHotStateDB.blockRootsOfSavedStates = [][32]byte{r1, r4, r7}

	// Run the migration routines concurrently for 2 different finalized roots.
	go func() {
		require.NoError(t, service.MigrateToCold(ctx, r4))
	}()

	require.NoError(t, service.MigrateToCold(ctx, r7))

	s1, err := service.beaconDB.State(ctx, r1)
	require.NoError(t, err)
	assert.Equal(t, s1.Slot(), primitives.Slot(1), "Did not save state")
	s4, err := service.beaconDB.State(ctx, r4)
	require.NoError(t, err)
	assert.Equal(t, s4.Slot(), primitives.Slot(4), "Did not save state")

	gotRoot := service.beaconDB.ArchivedPointRoot(ctx, 1/service.slotsPerArchivedPoint)
	assert.Equal(t, r1, gotRoot, "Did not save archived root")
	gotRoot = service.beaconDB.ArchivedPointRoot(ctx, 4)
	assert.Equal(t, r4, gotRoot, "Did not save archived root")
	lastIndex, err := service.beaconDB.LastArchivedSlot(ctx)
	require.NoError(t, err)
	assert.Equal(t, primitives.Slot(4), lastIndex, "Did not save last archived index")
	assert.DeepEqual(t, [][32]byte{r7}, service.saveHotStateDB.blockRootsOfSavedStates, "Did not remove all saved hot state roots")
	require.LogsContain(t, hook, "Saved state in DB")
}

// =========================================================================
// Tests for migrateToColdHdiff (state diff migration)
// =========================================================================

// setStateDiffExponents sets state diff exponents for testing.
// Uses exponents [6, 5] which means:
// - Level 0: Every 2^6 = 64 slots (full snapshot)
// - Level 1: Every 2^5 = 32 slots (diff)
func setStateDiffExponents() {
	globalFlags := flags.GlobalFlags{
		StateDiffExponents: []int{6, 5},
	}
	flags.Init(&globalFlags)
}

// TestMigrateToColdHdiff_CanUpdateFinalizedInfo verifies that the migration
// correctly updates finalized info when migrating to slots not in the diff tree.
func TestMigrateToColdHdiff_CanUpdateFinalizedInfo(t *testing.T) {
	ctx := t.Context()
	// Set exponents and create DB first (without EnableStateDiff flag).
	setStateDiffExponents()
	beaconDB := testDB.SetupDB(t)
	// Initialize the state diff cache via the method on *kv.Store (not in interface).
	require.NoError(t, beaconDB.(*kv.Store).InitStateDiffCacheForTesting(t, 0))
	// Now enable the feature flag.
	resetCfg := features.InitWithReset(&features.Flags{EnableStateDiff: true})
	defer resetCfg()
	service := New(beaconDB, doublylinkedtree.New())

	beaconState, _ := util.DeterministicGenesisState(t, 32)
	genesisStateRoot, err := beaconState.HashTreeRoot(ctx)
	require.NoError(t, err)
	genesis := blocks.NewGenesisBlock(genesisStateRoot[:])
	util.SaveBlock(t, ctx, beaconDB, genesis)
	gRoot, err := genesis.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, gRoot))

	// Put genesis state in epoch boundary cache so migrateToColdHdiff doesn't need to retrieve from DB.
	require.NoError(t, service.epochBoundaryStateCache.put(gRoot, beaconState))

	// Set initial finalized info at genesis.
	service.finalizedInfo = &finalizedInfo{
		slot:  0,
		root:  gRoot,
		state: beaconState,
	}

	// Create finalized block at slot 10 (not in diff tree, so no intermediate states saved).
	finalizedState := beaconState.Copy()
	require.NoError(t, finalizedState.SetSlot(10))
	b := util.NewBeaconBlock()
	b.Block.Slot = 10
	fRoot, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, beaconDB, b)
	require.NoError(t, service.epochBoundaryStateCache.put(fRoot, finalizedState))

	require.NoError(t, service.MigrateToCold(ctx, fRoot))

	// Verify finalized info is updated.
	assert.Equal(t, primitives.Slot(10), service.finalizedInfo.slot)
	assert.DeepEqual(t, fRoot, service.finalizedInfo.root)
	expectedHTR, err := finalizedState.HashTreeRoot(ctx)
	require.NoError(t, err)
	actualHTR, err := service.finalizedInfo.state.HashTreeRoot(ctx)
	require.NoError(t, err)
	assert.DeepEqual(t, expectedHTR, actualHTR)
}

// TestMigrateToColdHdiff_SkipsSlotsNotInDiffTree verifies that the migration
// skips slots that are not in the diff tree.
func TestMigrateToColdHdiff_SkipsSlotsNotInDiffTree(t *testing.T) {
	hook := logTest.NewGlobal()
	ctx := t.Context()
	// Set exponents and create DB first (without EnableStateDiff flag).
	setStateDiffExponents()
	beaconDB := testDB.SetupDB(t)
	// Initialize the state diff cache via the method on *kv.Store (not in interface).
	require.NoError(t, beaconDB.(*kv.Store).InitStateDiffCacheForTesting(t, 0))
	// Now enable the feature flag.
	resetCfg := features.InitWithReset(&features.Flags{EnableStateDiff: true})
	defer resetCfg()
	service := New(beaconDB, doublylinkedtree.New())

	beaconState, pks := util.DeterministicGenesisState(t, 32)
	genesisStateRoot, err := beaconState.HashTreeRoot(ctx)
	require.NoError(t, err)
	genesis := blocks.NewGenesisBlock(genesisStateRoot[:])
	util.SaveBlock(t, ctx, beaconDB, genesis)
	gRoot, err := genesis.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, gRoot))

	// Start from slot 1 to avoid slot 0 which is in the diff tree.
	service.finalizedInfo = &finalizedInfo{
		slot:  1,
		root:  gRoot,
		state: beaconState,
	}

	// Reset the log hook to ignore setup logs.
	hook.Reset()

	// Create a block at slot 20 (NOT in diff tree with exponents [6,5]).
	b20, err := util.GenerateFullBlock(beaconState, pks, util.DefaultBlockGenConfig(), 20)
	require.NoError(t, err)
	r20, err := b20.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, beaconDB, b20)
	require.NoError(t, beaconDB.SaveStateSummary(ctx, &ethpb.StateSummary{Slot: 20, Root: r20[:]}))

	// Put finalized state in cache.
	finalizedState := beaconState.Copy()
	require.NoError(t, finalizedState.SetSlot(20))
	require.NoError(t, service.epochBoundaryStateCache.put(r20, finalizedState))

	require.NoError(t, service.MigrateToCold(ctx, r20))

	// Verify NO states were saved during migration (slots 1-19 are not in diff tree).
	assert.LogsDoNotContain(t, hook, "Saved state in DB")
}

// TestMigrateToColdHdiff_MissedNonBoundarySlots verifies migration
// when all non-boundary slots are missed.
func TestMigrateToColdHdiff_MissedNonBoundarySlots(t *testing.T) {
	ctx := t.Context()
	setStateDiffExponents()
	beaconDB := testDB.SetupDB(t)
	require.NoError(t, beaconDB.(*kv.Store).InitStateDiffCacheForTesting(t, 0))
	resetCfg := features.InitWithReset(&features.Flags{EnableStateDiff: true})
	defer resetCfg()
	service := New(beaconDB, doublylinkedtree.New())

	beaconState, _ := util.DeterministicGenesisState(t, 32)
	genesisStateRoot, err := beaconState.HashTreeRoot(ctx)
	require.NoError(t, err)
	genesis := blocks.NewGenesisBlock(genesisStateRoot[:])
	util.SaveBlock(t, ctx, beaconDB, genesis)
	gRoot, err := genesis.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, gRoot))
	// Slot 0 needs to exist as the base snapshot for level-1 state diff entries.
	require.NoError(t, beaconDB.SaveState(ctx, beaconState, gRoot))

	service.finalizedInfo = &finalizedInfo{
		slot:  0,
		root:  gRoot,
		state: beaconState,
	}

	state32 := beaconState.Copy()
	require.NoError(t, state32.SetSlot(32))
	b32 := util.NewBeaconBlock()
	b32.Block.Slot = 32
	r32, err := b32.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, beaconDB, b32)
	require.NoError(t, service.epochBoundaryStateCache.put(r32, state32))

	state64 := beaconState.Copy()
	require.NoError(t, state64.SetSlot(64))
	b64 := util.NewBeaconBlock()
	b64.Block.Slot = 64
	r64, err := b64.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, beaconDB, b64)
	require.NoError(t, service.epochBoundaryStateCache.put(r64, state64))

	state96 := beaconState.Copy()
	require.NoError(t, state96.SetSlot(96))
	b96 := util.NewBeaconBlock()
	b96.Block.Slot = 96
	r96, err := b96.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, beaconDB, b96)
	require.NoError(t, service.epochBoundaryStateCache.put(r96, state96))

	finalizedState := beaconState.Copy()
	require.NoError(t, finalizedState.SetSlot(128))
	b128 := util.NewBeaconBlock()
	b128.Block.Slot = 128
	r128, err := b128.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, beaconDB, b128)
	require.NoError(t, service.epochBoundaryStateCache.put(r128, finalizedState))

	require.NoError(t, service.MigrateToCold(ctx, r128))

	assert.Equal(t, true, beaconDB.HasState(ctx, r32), "Did not save slot 32 checkpoint to database")
	assert.Equal(t, true, beaconDB.HasState(ctx, r64), "Did not save slot 64 checkpoint to database")
	assert.Equal(t, true, beaconDB.HasState(ctx, r96), "Did not save slot 96 checkpoint to database")
}

// TestMigrateToColdHdiff_MissedNonBoundarySlots verifies migration
// when all non-boundary slots are missed, and the epoch boundary cache
// is also missed for slot 96.
func TestMigrateToColdHdiff_MissedNonBoundarySlots_BoundaryCacheMissed(t *testing.T) {
	ctx := t.Context()
	setStateDiffExponents()
	beaconDB := testDB.SetupDB(t)
	require.NoError(t, beaconDB.(*kv.Store).InitStateDiffCacheForTesting(t, 0))
	resetCfg := features.InitWithReset(&features.Flags{EnableStateDiff: true})
	defer resetCfg()
	service := New(beaconDB, doublylinkedtree.New())

	beaconState, _ := util.DeterministicGenesisState(t, 32)
	genesisStateRoot, err := beaconState.HashTreeRoot(ctx)
	require.NoError(t, err)
	genesis := blocks.NewGenesisBlock(genesisStateRoot[:])
	util.SaveBlock(t, ctx, beaconDB, genesis)
	gRoot, err := genesis.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, gRoot))
	// Slot 0 needs to exist as the base snapshot for level-1 state diff entries.
	require.NoError(t, beaconDB.SaveState(ctx, beaconState, gRoot))

	service.finalizedInfo = &finalizedInfo{
		slot:  0,
		root:  gRoot,
		state: beaconState,
	}

	state32 := beaconState.Copy()
	require.NoError(t, state32.SetSlot(32))
	b32 := util.NewBeaconBlock()
	b32.Block.Slot = 32
	r32, err := b32.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, beaconDB, b32)
	require.NoError(t, service.epochBoundaryStateCache.put(r32, state32))

	state64 := beaconState.Copy()
	require.NoError(t, state64.SetSlot(64))
	b64 := util.NewBeaconBlock()
	b64.Block.Slot = 64
	r64, err := b64.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, beaconDB, b64)
	require.NoError(t, service.epochBoundaryStateCache.put(r64, state64))

	state96 := beaconState.Copy()
	require.NoError(t, state96.SetSlot(96))
	b96 := util.NewBeaconBlock()
	b96.Block.Slot = 96
	r96, err := b96.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, beaconDB, b96)
	// Simulate epoch boundary cache miss for slot 96 while hot cache still has the state.
	// this makes sure the call to StateByRoot doesn't fail here.
	service.hotStateCache.put(r96, state96)

	finalizedState := beaconState.Copy()
	require.NoError(t, finalizedState.SetSlot(128))
	b128 := util.NewBeaconBlock()
	b128.Block.Slot = 128
	r128, err := b128.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, beaconDB, b128)
	require.NoError(t, service.epochBoundaryStateCache.put(r128, finalizedState))

	require.NoError(t, service.MigrateToCold(ctx, r128))

	assert.Equal(t, true, beaconDB.HasState(ctx, r32), "Did not save slot 32 checkpoint to database")
	assert.Equal(t, true, beaconDB.HasState(ctx, r64), "Did not save slot 64 checkpoint to database")
	assert.Equal(t, true, beaconDB.HasState(ctx, r96), "Did not save slot 96 checkpoint to database")
}

// TestMigrateToColdHdiff_BoundaryCacheMiss_UseTargetSlotRoot verifies that a
// cache miss at a diff-tree slot still migrates using the block root at that
// slot (or an equivalent <= slot selection), rather than strictly below it.
func TestMigrateToColdHdiff_BoundaryCacheMiss_UseTargetSlotRoot(t *testing.T) {
	ctx := t.Context()
	setStateDiffExponents()
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
	// Slot 0 base snapshot for state-diff.
	require.NoError(t, beaconDB.SaveState(ctx, genesisState, gRoot))
	require.NoError(t, beaconDB.SaveStateSummary(ctx, &ethpb.StateSummary{Slot: 0, Root: gRoot[:]}))

	service.finalizedInfo = &finalizedInfo{
		slot:  0,
		root:  gRoot,
		state: genesisState,
	}

	current := genesisState
	var (
		r32, r64, r96, r128 [32]byte
		s32, s64, s96, s128 state.BeaconState
	)
	for _, slot := range []primitives.Slot{32, 64, 96, 128} {
		b, err := util.GenerateFullBlock(current, pks, util.DefaultBlockGenConfig(), slot)
		require.NoError(t, err)
		wsb, err := consensusblocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		nextState, err := executeStateTransitionStateGen(ctx, current, wsb)
		require.NoError(t, err)
		root, err := b.Block.HashTreeRoot()
		require.NoError(t, err)
		util.SaveBlock(t, ctx, beaconDB, b)
		require.NoError(t, beaconDB.SaveStateSummary(ctx, &ethpb.StateSummary{Slot: slot, Root: root[:]}))

		current = nextState
		switch slot {
		case 32:
			r32 = root
			s32 = nextState.Copy()
		case 64:
			r64 = root
			s64 = nextState.Copy()
		case 96:
			r96 = root
			s96 = nextState.Copy()
		case 128:
			r128 = root
			s128 = nextState.Copy()
		}
	}

	// Simulate cache eviction for slot 96 only: keep 32/64 and finalized 128.
	require.NoError(t, service.epochBoundaryStateCache.put(r32, s32))
	require.NoError(t, service.epochBoundaryStateCache.put(r64, s64))
	require.NoError(t, service.epochBoundaryStateCache.put(r128, s128))

	require.NoError(t, service.MigrateToCold(ctx, r128))

	// State by the slot-96 root should remain reconstructible after migration.
	got96, err := beaconDB.State(ctx, r96)
	require.NoError(t, err)
	assert.DeepSSZEqual(t, s96.ToProtoUnsafe(), got96.ToProtoUnsafe(), "slot 96 state mismatch")
}

// TestMigrateToColdHdiff_NoOpWhenFinalizedSlotNotAdvanced verifies that
// migration is a no-op when the finalized slot has not advanced.
func TestMigrateToColdHdiff_NoOpWhenFinalizedSlotNotAdvanced(t *testing.T) {
	ctx := t.Context()
	// Set exponents and create DB first (without EnableStateDiff flag).
	setStateDiffExponents()
	beaconDB := testDB.SetupDB(t)
	// Initialize the state diff cache via the method on *kv.Store (not in interface).
	require.NoError(t, beaconDB.(*kv.Store).InitStateDiffCacheForTesting(t, 0))
	// Now enable the feature flag.
	resetCfg := features.InitWithReset(&features.Flags{EnableStateDiff: true})
	defer resetCfg()
	service := New(beaconDB, doublylinkedtree.New())

	beaconState, _ := util.DeterministicGenesisState(t, 32)
	genesisStateRoot, err := beaconState.HashTreeRoot(ctx)
	require.NoError(t, err)
	genesis := blocks.NewGenesisBlock(genesisStateRoot[:])
	util.SaveBlock(t, ctx, beaconDB, genesis)
	gRoot, err := genesis.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, gRoot))

	// Set finalized info already at slot 50.
	finalizedState := beaconState.Copy()
	require.NoError(t, finalizedState.SetSlot(50))
	service.finalizedInfo = &finalizedInfo{
		slot:  50,
		root:  gRoot,
		state: finalizedState,
	}

	// Create block at same slot 50.
	b := util.NewBeaconBlock()
	b.Block.Slot = 50
	fRoot, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, beaconDB, b)
	require.NoError(t, service.epochBoundaryStateCache.put(fRoot, finalizedState))

	// Migration should be a no-op (finalized slot not advancing).
	require.NoError(t, service.MigrateToCold(ctx, fRoot))
}
