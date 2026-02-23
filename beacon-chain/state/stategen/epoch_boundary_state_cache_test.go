package stategen

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

func TestEpochBoundaryStateCache_BadSlotKey(t *testing.T) {
	_, err := slotKeyFn("sushi")
	assert.ErrorContains(t, errNotSlotRootInfo.Error(), err, "Did not get wanted error")
}

func TestEpochBoundaryStateCache_BadRootKey(t *testing.T) {
	_, err := rootKeyFn("noodle")
	assert.ErrorContains(t, errNotRootStateInfo.Error(), err, "Did not get wanted error")
}

func TestEpochBoundaryStateCache_CanSaveAndDelete(t *testing.T) {
	e := newBoundaryStateCache()
	s, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, s.SetSlot(1))
	r := [32]byte{'a'}
	require.NoError(t, e.put(r, s))

	got, exists, err := e.getByBlockRoot([32]byte{'b'})
	require.NoError(t, err)
	assert.Equal(t, false, exists, "Should not exist")
	assert.Equal(t, (*rootStateInfo)(nil), got, "Should not exist")

	got, exists, err = e.getByBlockRoot([32]byte{'a'})
	require.NoError(t, err)
	assert.Equal(t, true, exists, "Should exist")
	assert.DeepSSZEqual(t, s.ToProtoUnsafe(), got.state.ToProtoUnsafe(), "Should have the same state")

	got, exists, err = e.getBySlot(2)
	require.NoError(t, err)
	assert.Equal(t, false, exists, "Should not exist")
	assert.Equal(t, (*rootStateInfo)(nil), got, "Should not exist")

	got, exists, err = e.getBySlot(1)
	require.NoError(t, err)
	assert.Equal(t, true, exists, "Should exist")
	assert.DeepSSZEqual(t, s.ToProtoUnsafe(), got.state.ToProtoUnsafe(), "Should have the same state")

	require.NoError(t, e.delete(r))
	got, exists, err = e.getByBlockRoot([32]byte{'b'})
	require.NoError(t, err)
	assert.Equal(t, false, exists, "Should not exist")
	assert.Equal(t, (*rootStateInfo)(nil), got, "Should not exist")

	got, exists, err = e.getBySlot(1)
	require.NoError(t, err)
	assert.Equal(t, false, exists, "Should not exist")
	assert.Equal(t, (*rootStateInfo)(nil), got, "Should not exist")
}

func TestEpochBoundaryStateCache_CanTrim(t *testing.T) {
	e := newBoundaryStateCache()
	offSet := primitives.Slot(10)
	for i := primitives.Slot(0); i < offSet.Add(maxCacheSize); i++ {
		s, err := util.NewBeaconState()
		require.NoError(t, err)
		require.NoError(t, s.SetSlot(i))
		r := [32]byte{byte(i)}
		require.NoError(t, e.put(r, s))
	}

	assert.Equal(t, int(maxCacheSize), len(e.rootStateCache.ListKeys()), "Did not trim to the correct amount")
	assert.Equal(t, int(maxCacheSize), len(e.slotRootCache.ListKeys()), "Did not trim to the correct amount")
	for _, l := range e.rootStateCache.List() {
		i, ok := l.(*rootStateInfo)
		require.Equal(t, true, ok, "Bad type assertion")
		if i.state.Slot() < offSet {
			t.Error("Did not trim the correct state")
		}
	}
}

func TestEpochBoundaryStateCache_ExpandAndCompress(t *testing.T) {
	e := newBoundaryStateCache()

	// Reset maxCacheSize to default at the start (in case other tests modified it).
	maxCacheSize = defaultEpochBoundaryCacheSize
	t.Cleanup(func() { maxCacheSize = defaultEpochBoundaryCacheSize })

	// Fill with more entries than default but less than expanded.
	for i := primitives.Slot(0); i < primitives.Slot(expandedEpochBoundaryCacheSize); i++ {
		s, err := util.NewBeaconState()
		require.NoError(t, err)
		require.NoError(t, s.SetSlot(i))
		r := [32]byte{byte(i)}
		require.NoError(t, e.put(r, s))
	}

	// Default size should trim to defaultEpochBoundaryCacheSize.
	assert.Equal(t, int(defaultEpochBoundaryCacheSize), len(e.rootStateCache.ListKeys()))

	// Expand the cache.
	e.ExpandEpochBoundaryCache()
	assert.Equal(t, expandedEpochBoundaryCacheSize, maxCacheSize)

	// Calling expand again is a no-op.
	e.ExpandEpochBoundaryCache()
	assert.Equal(t, expandedEpochBoundaryCacheSize, maxCacheSize)

	// Now fill to expanded size.
	for i := primitives.Slot(0); i < primitives.Slot(expandedEpochBoundaryCacheSize); i++ {
		s, err := util.NewBeaconState()
		require.NoError(t, err)
		require.NoError(t, s.SetSlot(100+i))
		r := [32]byte{byte(100 + i)}
		require.NoError(t, e.put(r, s))
	}
	assert.Equal(t, int(expandedEpochBoundaryCacheSize), len(e.rootStateCache.ListKeys()))

	// Compress should trim back to default.
	e.CompressEpochBoundaryCache()
	assert.Equal(t, defaultEpochBoundaryCacheSize, maxCacheSize)
	assert.Equal(t, int(defaultEpochBoundaryCacheSize), len(e.rootStateCache.ListKeys()))
	assert.Equal(t, int(defaultEpochBoundaryCacheSize), len(e.slotRootCache.ListKeys()))

	// Calling compress again is a no-op.
	e.CompressEpochBoundaryCache()
	assert.Equal(t, defaultEpochBoundaryCacheSize, maxCacheSize)
}
