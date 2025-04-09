package cache

import (
	"testing"

	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func TestValidatorPayloadIDsCache_GetAndSaveValidatorPayloadIDs(t *testing.T) {
	cache := NewPayloadIDCache()
	var r [32]byte
	p, ok := cache.PayloadID(0, r)
	require.Equal(t, false, ok)
	require.Equal(t, primitives.PayloadID{}, p)

	slot := primitives.Slot(1234)
	pid := primitives.PayloadID{1, 2, 3, 3, 7, 8, 7, 8}
	r = [32]byte{1, 2, 3}
	cache.Set(slot, r, pid)
	p, ok = cache.PayloadID(slot, r)
	require.Equal(t, true, ok)
	require.Equal(t, pid, p)

	slot = primitives.Slot(9456456)
	r = [32]byte{4, 5, 6}
	cache.Set(slot, r, primitives.PayloadID{})
	p, ok = cache.PayloadID(slot, r)
	require.Equal(t, true, ok)
	require.Equal(t, primitives.PayloadID{}, p)

	// reset cache without pid
	slot = primitives.Slot(9456456)
	r = [32]byte{7, 8, 9}
	pid = [8]byte{3, 2, 3, 33, 72, 8, 7, 8}
	cache.Set(slot, r, pid)
	p, ok = cache.PayloadID(slot, r)
	require.Equal(t, true, ok)
	require.Equal(t, pid, p)

	// Forked chain
	r = [32]byte{1, 2, 3}
	p, ok = cache.PayloadID(slot, r)
	require.Equal(t, false, ok)
	require.Equal(t, primitives.PayloadID{}, p)

	// existing pid - change the cache
	slot = primitives.Slot(9456456)
	r = [32]byte{7, 8, 9}
	newPid := primitives.PayloadID{1, 2, 3, 33, 72, 8, 7, 1}
	cache.Set(slot, r, newPid)
	p, ok = cache.PayloadID(slot, r)
	require.Equal(t, true, ok)
	require.Equal(t, newPid, p)

	// remove cache entry
	cache.prune(slot + 1)
	p, ok = cache.PayloadID(slot, r)
	require.Equal(t, false, ok)
	require.Equal(t, primitives.PayloadID{}, p)
}
