package kv

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/math"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"go.etcd.io/bbolt"
)

// createBeaconState is createState narrowed to the writable interface InitializeArchiveOrigin needs.
func createBeaconState(t *testing.T, slot primitives.Slot, v int) state.BeaconState {
	ro, _ := createState(t, slot, v)
	st, ok := ro.(state.BeaconState)
	require.Equal(t, true, ok)
	return st
}

// hasStateDiffAtSlot reports whether the tree holds an entry at the given slot's level.
func hasStateDiffAtSlot(s *Store, slot primitives.Slot) (bool, error) {
	lvl := computeLevel(s.getOffset(), slot)
	if lvl == -1 {
		return false, nil
	}
	var has bool
	err := s.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(stateDiffBucket)
		if lvl == 0 {
			has = bucket.Get(makeKeyForStateDiffTree(0, uint64(slot))) != nil
			return nil
		}
		has = hasCompleteDiffAtLevelSlot(bucket, lvl, uint64(slot))
		return nil
	})
	return has, err
}

func TestArchiveStatus_RoundTrip(t *testing.T) {
	db := setupDB(t)
	ctx := t.Context()

	_, err := db.ArchiveStatus(ctx)
	require.ErrorIs(t, err, ErrNotFound)

	want := &ArchiveStatus{
		OriginSlot:             8192,
		RegeneratedThroughSlot: 8320,
		OriginStateRoot:        [32]byte{1, 2, 3},
	}
	require.NoError(t, db.SaveArchiveStatus(ctx, want))

	got, err := db.ArchiveStatus(ctx)
	require.NoError(t, err)
	require.DeepEqual(t, want, got)

	want.Complete = true
	want.RegeneratedThroughSlot = 9000
	require.NoError(t, db.SaveArchiveStatus(ctx, want))
	got, err = db.ArchiveStatus(ctx)
	require.NoError(t, err)
	require.Equal(t, true, got.Complete)
	require.Equal(t, primitives.Slot(9000), got.RegeneratedThroughSlot)
}

func TestArchiveStatus_KeyDoesNotCollideWithTree(t *testing.T) {
	setDefaultStateDiffExponents()
	resetCfg := features.InitWithReset(&features.Flags{EnableStateDiff: true, EnableArchive: true})
	defer resetCfg()

	db := setupDB(t)
	ctx := t.Context()
	require.NoError(t, db.InitializeArchiveOrigin(ctx, createBeaconState(t, 0, version.Phase0)))
	require.NoError(t, db.SaveArchiveStatus(ctx, &ArchiveStatus{OriginSlot: 0}))

	// The offset and exponents metadata must survive an archive status write, and vice versa.
	offset, err := db.loadOffset()
	require.NoError(t, err)
	require.Equal(t, uint64(0), offset)
	_, err = db.loadStateDiffExponents()
	require.NoError(t, err)

	// The archive status key must not be mistaken for a level 0 tree entry.
	require.Equal(t, uint64(0), latestLevelZeroSlot(db, 0))
}

func TestArchivePending(t *testing.T) {
	db := setupDB(t)

	_, pending := db.archivePending()
	require.Equal(t, false, pending)

	db.setArchiveStatus(&ArchiveStatus{OriginSlot: 64, RegeneratedThroughSlot: 128})
	frontier, pending := db.archivePending()
	require.Equal(t, true, pending)
	require.Equal(t, primitives.Slot(128), frontier)

	db.setArchiveStatus(&ArchiveStatus{OriginSlot: 64, RegeneratedThroughSlot: 128, Complete: true})
	_, pending = db.archivePending()
	require.Equal(t, false, pending)
}

func TestInitializeArchiveOrigin(t *testing.T) {
	setDefaultStateDiffExponents()
	resetCfg := features.InitWithReset(&features.Flags{EnableStateDiff: true, EnableArchive: true})
	defer resetCfg()

	db := setupDB(t)
	ctx := t.Context()
	slot := primitives.Slot(math.PowerOf2(11))
	st := createBeaconState(t, slot, version.Phase0)

	require.NoError(t, db.InitializeArchiveOrigin(ctx, st))

	offset, err := db.loadOffset()
	require.NoError(t, err)
	require.Equal(t, uint64(slot), offset)

	as, err := db.ArchiveStatus(ctx)
	require.NoError(t, err)
	require.Equal(t, slot, as.OriginSlot)
	require.Equal(t, slot, as.RegeneratedThroughSlot)
	require.Equal(t, false, as.Complete)

	// The origin state must be readable back out of the tree at its own slot.
	got, err := db.StateBySlotFromDiffTree(ctx, slot)
	require.NoError(t, err)
	wantSSZ, err := st.MarshalSSZ()
	require.NoError(t, err)
	gotSSZ, err := got.MarshalSSZ()
	require.NoError(t, err)
	require.DeepSSZEqual(t, wantSSZ, gotSSZ)

	// Re-running with the same origin is a no-op.
	require.NoError(t, db.InitializeArchiveOrigin(ctx, st))
}

func TestInitializeArchiveOrigin_RejectsChangedOrigin(t *testing.T) {
	setDefaultStateDiffExponents()
	resetCfg := features.InitWithReset(&features.Flags{EnableStateDiff: true, EnableArchive: true})
	defer resetCfg()

	db := setupDB(t)
	ctx := t.Context()
	st := createBeaconState(t, primitives.Slot(math.PowerOf2(11)), version.Phase0)
	require.NoError(t, db.InitializeArchiveOrigin(ctx, st))

	other := createBeaconState(t, primitives.Slot(math.PowerOf2(12)), version.Phase0)
	err := db.InitializeArchiveOrigin(ctx, other)
	require.ErrorContains(t, "archive origin state changed", err)
}

// In archive mode only InitializeArchiveOrigin may anchor the tree. That holds before an origin exists,
// which is what lets the node defer anchoring until it has checked the origin against the sync origin, and
// after one exists, since everything below the offset becomes unrepresentable.
func TestInitializeStateDiff_ArchiveNeverAnchors(t *testing.T) {
	setDefaultStateDiffExponents()
	resetCfg := features.InitWithReset(&features.Flags{EnableStateDiff: true, EnableArchive: true})
	defer resetCfg()

	db := setupDB(t)
	ctx := t.Context()

	// Before the archive origin is known: this is what SaveGenesisData and SaveOrigin do, and neither may
	// take the offset, or the archive could never be anchored below it.
	genesisState, _ := createState(t, 0, version.Phase0)
	require.NoError(t, db.initializeStateDiff(0, genesisState))
	_, err := db.loadOffset()
	require.ErrorContains(t, "state diff offset not found", err)
	hasOffset, err := db.hasStateDiffOffset()
	require.NoError(t, err)
	require.Equal(t, false, hasOffset)

	origin := primitives.Slot(math.PowerOf2(11))
	require.NoError(t, db.InitializeArchiveOrigin(ctx, createBeaconState(t, origin, version.Phase0)))

	// And after: a later caller must not move it.
	later, _ := createState(t, primitives.Slot(math.PowerOf2(12)), version.Phase0)
	require.NoError(t, db.initializeStateDiff(later.Slot(), later))

	offset, err := db.loadOffset()
	require.NoError(t, err)
	require.Equal(t, uint64(origin), offset)
}

// Without archive mode the offset is still re-anchored by checkpoint sync, exactly as before.
func TestInitializeStateDiff_NonArchiveReanchors(t *testing.T) {
	setDefaultStateDiffExponents()
	resetCfg := features.InitWithReset(&features.Flags{EnableStateDiff: true})
	defer resetCfg()

	db := setupDB(t)
	genesisState, _ := createState(t, 0, version.Phase0)
	require.NoError(t, db.initializeStateDiff(0, genesisState))

	cpSlot := primitives.Slot(math.PowerOf2(12))
	cpState, _ := createState(t, cpSlot, version.Phase0)
	require.NoError(t, db.initializeStateDiff(cpSlot, cpState))

	offset, err := db.loadOffset()
	require.NoError(t, err)
	require.Equal(t, uint64(cpSlot), offset)
}

// While the walk is running, only the walk may write into the tree. A live write above its frontier has no
// anchor chain beneath it and would become the level maximum that startStateDiff validates against.
func TestSaveStateByDiff_RejectsWritesAboveArchiveFrontier(t *testing.T) {
	setStateDiffExponents([]int{11, 9, 5})
	resetCfg := features.InitWithReset(&features.Flags{EnableStateDiff: true, EnableArchive: true})
	defer resetCfg()

	db := setupDB(t)
	ctx := t.Context()
	require.NoError(t, db.InitializeArchiveOrigin(ctx, createBeaconState(t, 0, version.Phase0)))

	// Simulate a walk that has reached slot 64.
	as, err := db.ArchiveStatus(ctx)
	require.NoError(t, err)
	as.RegeneratedThroughSlot = 64
	require.NoError(t, db.SaveArchiveStatus(ctx, as))

	// A boundary at or below the frontier is accepted.
	within, _ := createState(t, 32, version.Phase0)
	require.NoError(t, db.saveStateByDiff(ctx, within))
	has, err := hasStateDiffAtSlot(db, 32)
	require.NoError(t, err)
	require.Equal(t, true, has)

	// The next boundary above the frontier stays open: that is the one the walk itself is about to write.
	next, _ := createState(t, 96, version.Phase0)
	require.NoError(t, db.saveStateByDiff(ctx, next))
	has, err = hasStateDiffAtSlot(db, 96)
	require.NoError(t, err)
	require.Equal(t, true, has)

	// A boundary above the frontier is refused rather than written without an anchor. It has to be an error
	// and not a skip: the caller advances its finalized info on success, so a silent skip would leave a hole
	// nothing ever comes back for.
	above, _ := createState(t, 512, version.Phase0)
	err = db.saveStateByDiff(ctx, above)
	require.ErrorIs(t, err, ErrAboveArchiveFrontier)
	has, err = hasStateDiffAtSlot(db, 512)
	require.NoError(t, err)
	require.Equal(t, false, has)

	// Once regeneration is complete the live chain writes normally again.
	as.Complete = true
	require.NoError(t, db.SaveArchiveStatus(ctx, as))
	require.NoError(t, db.saveStateByDiff(ctx, above))
	has, err = hasStateDiffAtSlot(db, 512)
	require.NoError(t, err)
	require.Equal(t, true, has)
}

// latestLevelZeroSlot must track the newest snapshot so the cached level 0 anchor is the one live writes
// resolve to once the tree spans more than 2^exponents[0] slots.
func TestLatestLevelZeroSlot(t *testing.T) {
	setStateDiffExponents([]int{11, 9, 5})
	resetCfg := features.InitWithReset(&features.Flags{EnableStateDiff: true})
	defer resetCfg()

	db := setupDB(t)
	ctx := t.Context()
	require.NoError(t, setOffsetInDB(db, 0))

	st, _ := createState(t, 0, version.Phase0)
	require.NoError(t, db.saveStateByDiff(ctx, st))
	require.Equal(t, uint64(0), latestLevelZeroSlot(db, 0))

	span := math.PowerOf2(11)
	st, _ = createState(t, primitives.Slot(span), version.Phase0)
	require.NoError(t, db.saveStateByDiff(ctx, st))
	require.Equal(t, span, latestLevelZeroSlot(db, 0))

	// A snapshot that is not a level 0 boundary for this offset is ignored.
	require.Equal(t, uint64(1), latestLevelZeroSlot(db, 1))
}

// The memo must serve repeated reads of the same anchor and be dropped when the anchor is replaced.
func TestStateDiff_AnchorMemo(t *testing.T) {
	setDefaultStateDiffExponents()
	resetCfg := features.InitWithReset(&features.Flags{EnableStateDiff: true})
	defer resetCfg()

	db := setupDB(t)
	ctx := t.Context()
	require.NoError(t, setOffsetInDB(db, 0))
	st, _ := createState(t, 0, version.Phase0)
	require.NoError(t, db.saveStateByDiff(ctx, st))

	first := db.stateDiffCache.getAnchor(0)
	require.NotNil(t, first)
	second := db.stateDiffCache.getAnchor(0)
	// Same object: the second read did not deserialize again.
	require.Equal(t, true, first == second)

	replacement, _ := createState(t, primitives.Slot(math.PowerOf2(18)), version.Phase0)
	require.NoError(t, db.stateDiffCache.setAnchor(0, replacement))
	third := db.stateDiffCache.getAnchor(0)
	require.NotNil(t, third)
	require.Equal(t, false, first == third)
	require.Equal(t, primitives.Slot(math.PowerOf2(18)), third.Slot())

	db.stateDiffCache.clearAnchors()
	require.IsNil(t, db.stateDiffCache.getAnchor(0))
}

// The memo trades memory for deserializations, so it must stay bounded: anchors are held compressed
// precisely so that a seven-level tree does not pin six full states. Least recently used is evicted.
func TestStateDiff_AnchorMemoIsBounded(t *testing.T) {
	setDefaultStateDiffExponents()
	resetCfg := features.InitWithReset(&features.Flags{EnableStateDiff: true})
	defer resetCfg()

	db := setupDB(t)
	require.NoError(t, setOffsetInDB(db, 0))
	cache := db.stateDiffCache
	require.Equal(t, true, len(cache.anchors) > anchorMemoSize)

	for lvl := range anchorMemoSize + 1 {
		st, _ := createState(t, primitives.Slot(math.PowerOf2(18)*uint64(lvl+1)), version.Phase0)
		require.NoError(t, cache.setAnchor(lvl, st))
	}

	// Touch every level once; each is a miss, so the oldest is evicted as we go.
	seen := make([]state.ReadOnlyBeaconState, anchorMemoSize+1)
	for lvl := range anchorMemoSize + 1 {
		seen[lvl] = cache.getAnchor(lvl)
		require.NotNil(t, seen[lvl])
	}

	// The most recent anchorMemoSize levels are still memoized.
	for lvl := 1; lvl < anchorMemoSize+1; lvl++ {
		require.Equal(t, true, seen[lvl] == cache.getAnchor(lvl), "level %d should be memoized", lvl)
	}
	// Level 0 fell out and is deserialized afresh.
	require.Equal(t, false, seen[0] == cache.getAnchor(0))
}
