package stategen

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/db/kv"
	testDB "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	doublylinkedtree "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/doubly-linked-tree"
	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	logTest "github.com/sirupsen/logrus/hooks/test"
)

func TestSaveState_HotStateCanBeSaved(t *testing.T) {
	ctx := t.Context()
	beaconDB := testDB.SetupDB(t)

	service := New(beaconDB, doublylinkedtree.New())
	service.slotsPerArchivedPoint = 1
	beaconState, _ := util.DeterministicGenesisState(t, 32)
	// This goes to hot section, verify it can save on epoch boundary.
	require.NoError(t, beaconState.SetSlot(params.BeaconConfig().SlotsPerEpoch))

	r := [32]byte{'a'}
	require.NoError(t, service.SaveState(ctx, r, beaconState))

	// Should save both state and state summary.
	_, ok, err := service.epochBoundaryStateCache.getByBlockRoot(r)
	require.NoError(t, err)
	assert.Equal(t, true, ok, "Should have saved the state")
	assert.Equal(t, true, service.beaconDB.HasStateSummary(ctx, r), "Should have saved the state summary")
}

func TestSaveState_HotStateCached(t *testing.T) {
	hook := logTest.NewGlobal()
	ctx := t.Context()
	beaconDB := testDB.SetupDB(t)

	service := New(beaconDB, doublylinkedtree.New())
	service.slotsPerArchivedPoint = 1
	beaconState, _ := util.DeterministicGenesisState(t, 32)
	require.NoError(t, beaconState.SetSlot(params.BeaconConfig().SlotsPerEpoch))

	// Cache the state prior.
	r := [32]byte{'a'}
	service.hotStateCache.put(r, beaconState)
	require.NoError(t, service.SaveState(ctx, r, beaconState))

	// Should not save the state and state summary.
	assert.Equal(t, false, service.beaconDB.HasState(ctx, r), "Should not have saved the state")
	assert.Equal(t, false, service.beaconDB.HasStateSummary(ctx, r), "Should have saved the state summary")
	require.LogsDoNotContain(t, hook, "Saved full state on epoch boundary")
}

func TestState_ForceCheckpoint_SavesStateToDatabase(t *testing.T) {
	ctx := t.Context()
	beaconDB := testDB.SetupDB(t)

	svc := New(beaconDB, doublylinkedtree.New())
	beaconState, _ := util.DeterministicGenesisState(t, 32)
	require.NoError(t, beaconState.SetSlot(params.BeaconConfig().SlotsPerEpoch))

	r := [32]byte{'a'}
	svc.hotStateCache.put(r, beaconState)

	require.Equal(t, false, beaconDB.HasState(ctx, r), "Database has state stored already")
	assert.NoError(t, svc.ForceCheckpoint(ctx, r[:]))
	assert.Equal(t, true, beaconDB.HasState(ctx, r), "Did not save checkpoint to database")

	// Should not panic with genesis finalized root.
	assert.NoError(t, svc.ForceCheckpoint(ctx, params.BeaconConfig().ZeroHash[:]))
}

func TestSaveState_Alreadyhas(t *testing.T) {
	hook := logTest.NewGlobal()
	ctx := t.Context()
	beaconDB := testDB.SetupDB(t)
	service := New(beaconDB, doublylinkedtree.New())

	beaconState, _ := util.DeterministicGenesisState(t, 32)
	require.NoError(t, beaconState.SetSlot(params.BeaconConfig().SlotsPerEpoch))
	r := [32]byte{'A'}

	// Pre cache the hot state.
	service.hotStateCache.put(r, beaconState)
	require.NoError(t, service.saveStateByRoot(ctx, r, beaconState))

	// Should not save the state and state summary.
	assert.Equal(t, false, service.beaconDB.HasState(ctx, r), "Should not have saved the state")
	assert.Equal(t, false, service.beaconDB.HasStateSummary(ctx, r), "Should have saved the state summary")
	require.LogsDoNotContain(t, hook, "Saved full state on epoch boundary")
}

func TestSaveState_CanSaveOnEpochBoundary(t *testing.T) {
	ctx := t.Context()
	beaconDB := testDB.SetupDB(t)
	service := New(beaconDB, doublylinkedtree.New())

	beaconState, _ := util.DeterministicGenesisState(t, 32)
	require.NoError(t, beaconState.SetSlot(params.BeaconConfig().SlotsPerEpoch))
	r := [32]byte{'A'}

	require.NoError(t, service.saveStateByRoot(ctx, r, beaconState))

	// Should save both state and state summary.
	_, ok, err := service.epochBoundaryStateCache.getByBlockRoot(r)
	require.NoError(t, err)
	require.Equal(t, true, ok, "Did not save epoch boundary state")
	assert.Equal(t, true, service.beaconDB.HasStateSummary(ctx, r), "Should have saved the state summary")
	// Should have not been saved in DB.
	require.Equal(t, false, beaconDB.HasState(ctx, r))
}

func TestSaveState_NoSaveNotEpochBoundary(t *testing.T) {
	hook := logTest.NewGlobal()
	ctx := t.Context()
	beaconDB := testDB.SetupDB(t)
	service := New(beaconDB, doublylinkedtree.New())

	beaconState, _ := util.DeterministicGenesisState(t, 32)
	require.NoError(t, beaconState.SetSlot(params.BeaconConfig().SlotsPerEpoch-1))
	r := [32]byte{'A'}
	b := util.NewBeaconBlock()
	util.SaveBlock(t, ctx, beaconDB, b)
	gRoot, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, gRoot))
	require.NoError(t, service.SaveState(ctx, r, beaconState))

	// Should only save state summary.
	assert.Equal(t, false, service.beaconDB.HasState(ctx, r), "Should not have saved the state")
	assert.Equal(t, true, service.beaconDB.HasStateSummary(ctx, r), "Should have saved the state summary")
	require.LogsDoNotContain(t, hook, "Saved full state on epoch boundary")
	// Should have not been saved in DB.
	require.Equal(t, false, beaconDB.HasState(ctx, r))
}

func TestSaveState_RecoverForEpochBoundary(t *testing.T) {
	ctx := t.Context()
	beaconDB := testDB.SetupDB(t)
	service := New(beaconDB, doublylinkedtree.New())

	beaconState, _ := util.DeterministicGenesisState(t, 32)
	require.NoError(t, beaconState.SetSlot(params.BeaconConfig().SlotsPerEpoch-1))
	r := [32]byte{'A'}
	boundaryRoot := [32]byte{'B'}
	require.NoError(t, beaconState.UpdateBlockRootAtIndex(0, boundaryRoot))

	b := util.NewBeaconBlock()
	util.SaveBlock(t, ctx, beaconDB, b)
	gRoot, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, gRoot))
	// Save boundary state to the hot state cache.
	boundaryState, _ := util.DeterministicGenesisState(t, 32)
	service.hotStateCache.put(boundaryRoot, boundaryState)
	require.NoError(t, service.SaveState(ctx, r, beaconState))

	rInfo, ok, err := service.epochBoundaryStateCache.getByBlockRoot(boundaryRoot)
	assert.NoError(t, err)
	assert.Equal(t, true, ok, "state does not exist in cache")
	assert.Equal(t, rInfo.root, boundaryRoot, "incorrect root of root state info")
	assert.Equal(t, rInfo.state.Slot(), primitives.Slot(0), "incorrect slot of state")
}

func TestSaveState_CanSaveHotStateToDB(t *testing.T) {
	hook := logTest.NewGlobal()
	ctx := t.Context()
	beaconDB := testDB.SetupDB(t)
	service := New(beaconDB, doublylinkedtree.New())
	service.EnableSaveHotStateToDB(ctx)
	beaconState, _ := util.DeterministicGenesisState(t, 32)
	require.NoError(t, beaconState.SetSlot(defaultHotStateDBInterval))

	r := [32]byte{'A'}
	require.NoError(t, service.saveStateByRoot(ctx, r, beaconState))

	require.LogsContain(t, hook, "Saving hot state to DB")
	// Should have saved in DB.
	require.Equal(t, true, beaconDB.HasState(ctx, r))
}

func TestEnableSaveHotStateToDB_Enabled(t *testing.T) {
	hook := logTest.NewGlobal()
	ctx := t.Context()
	beaconDB := testDB.SetupDB(t)
	service := New(beaconDB, doublylinkedtree.New())

	service.EnableSaveHotStateToDB(ctx)
	require.LogsContain(t, hook, "Entering mode to save hot states in DB")
	require.Equal(t, true, service.saveHotStateDB.enabled)
}

func TestEnableSaveHotStateToDB_AlreadyEnabled(t *testing.T) {
	hook := logTest.NewGlobal()
	ctx := t.Context()
	beaconDB := testDB.SetupDB(t)
	service := New(beaconDB, doublylinkedtree.New())
	service.saveHotStateDB.enabled = true
	service.EnableSaveHotStateToDB(ctx)
	require.LogsDoNotContain(t, hook, "Entering mode to save hot states in DB")
	require.Equal(t, true, service.saveHotStateDB.enabled)
}

func TestEnableSaveHotStateToDB_Disabled(t *testing.T) {
	hook := logTest.NewGlobal()
	ctx := t.Context()
	beaconDB := testDB.SetupDB(t)
	service := New(beaconDB, doublylinkedtree.New())
	service.saveHotStateDB.enabled = true
	b := util.NewBeaconBlock()
	util.SaveBlock(t, ctx, beaconDB, b)
	r, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	service.saveHotStateDB.blockRootsOfSavedStates = [][32]byte{r}
	require.NoError(t, service.DisableSaveHotStateToDB(ctx))
	require.LogsContain(t, hook, "Exiting mode to save hot states in DB")
	require.Equal(t, false, service.saveHotStateDB.enabled)
	require.Equal(t, 0, len(service.saveHotStateDB.blockRootsOfSavedStates))
}

func TestEnableSaveHotStateToDB_AlreadyDisabled(t *testing.T) {
	hook := logTest.NewGlobal()
	ctx := t.Context()
	beaconDB := testDB.SetupDB(t)
	service := New(beaconDB, doublylinkedtree.New())
	require.NoError(t, service.DisableSaveHotStateToDB(ctx))
	require.LogsDoNotContain(t, hook, "Exiting mode to save hot states in DB")
	require.Equal(t, false, service.saveHotStateDB.enabled)
}

// TestSaveState_StateDiff_SavesAtDiffTreeBoundary verifies that when
// --enable-state-diff is active, saveStateByRoot persists hot states at
// diff-tree boundary slots to the DB even when saveHotStateDB is disabled.
// This caps the worst-case replay on restart to the finest diff-tree
// granularity (2^5 = 32 slots by default) instead of the entire
// finalized-to-head gap.
func TestSaveState_StateDiff_SavesAtDiffTreeBoundary(t *testing.T) {
	ctx := t.Context()
	// Use small exponents [6, 5] => level 0 every 64 slots, level 1 every 32 slots.
	globalFlags := flags.GlobalFlags{StateDiffExponents: []int{6, 5}}
	flags.Init(&globalFlags)

	beaconDB := testDB.SetupDB(t)
	require.NoError(t, beaconDB.(*kv.Store).InitStateDiffCacheForTesting(t, 0))
	resetCfg := features.InitWithReset(&features.Flags{EnableStateDiff: true})
	defer resetCfg()

	service := New(beaconDB, doublylinkedtree.New())
	// Explicitly verify saveHotStateDB is NOT enabled — this is the normal
	// initial-sync condition. The point is that diff-tree saves should happen
	// regardless.
	require.Equal(t, false, service.saveHotStateDB.enabled)

	beaconState, _ := util.DeterministicGenesisState(t, 32)
	// Slot 64 lands on a level-0 boundary (2^6 = 64). It should be saved.
	require.NoError(t, beaconState.SetSlot(64))
	r := [32]byte{'D'}
	require.NoError(t, service.saveStateByRoot(ctx, r, beaconState))

	assert.Equal(t, true, beaconDB.HasState(ctx, r),
		"State at diff-tree boundary slot should be persisted to DB when state-diff is enabled")
}

// TestSaveState_StateDiff_SkipsNonBoundary verifies that when
// --enable-state-diff is active, saveStateByRoot does NOT persist states
// at slots that are not on any diff-tree boundary.
func TestSaveState_StateDiff_SkipsNonBoundary(t *testing.T) {
	ctx := t.Context()
	globalFlags := flags.GlobalFlags{StateDiffExponents: []int{6, 5}}
	flags.Init(&globalFlags)

	beaconDB := testDB.SetupDB(t)
	require.NoError(t, beaconDB.(*kv.Store).InitStateDiffCacheForTesting(t, 0))
	resetCfg := features.InitWithReset(&features.Flags{EnableStateDiff: true})
	defer resetCfg()

	service := New(beaconDB, doublylinkedtree.New())
	require.Equal(t, false, service.saveHotStateDB.enabled)

	beaconState, _ := util.DeterministicGenesisState(t, 32)
	// Slot 50 is NOT on any diff-tree boundary (not divisible by 32 or 64).
	require.NoError(t, beaconState.SetSlot(50))
	r := [32]byte{'E'}
	require.NoError(t, service.saveStateByRoot(ctx, r, beaconState))

	assert.Equal(t, false, beaconDB.HasState(ctx, r),
		"State at non-boundary slot should NOT be persisted to DB")
}
