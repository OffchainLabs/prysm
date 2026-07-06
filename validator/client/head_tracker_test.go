package client

import (
	"sync"
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// hexRoot returns the 0x-prefixed hex of a 32-byte root whose first byte is b.
func hexRoot(b byte) string {
	root := [32]byte{b}
	return hexutil.Encode(root[:])
}

// latestHead reads the tracked head under the read lock (test-only accessor).
func latestHead(h *headTracker) ([32]byte, primitives.Slot, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return h.root, h.slot, h.set
}

func TestHeadTracker_EmptyUntilFirstUpdate(t *testing.T) {
	h := newHeadTracker()
	_, _, ok := latestHead(h)
	assert.Equal(t, false, ok)
}

func TestHeadTracker_NewestWins(t *testing.T) {
	h := newHeadTracker()
	require.NoError(t, h.update(10, hexRoot(0xaa)))
	require.NoError(t, h.update(11, hexRoot(0xbb)))

	root, slot, ok := latestHead(h)
	assert.Equal(t, true, ok)
	assert.Equal(t, primitives.Slot(11), slot)
	assert.Equal(t, byte(0xbb), root[0])
}

func TestHeadTracker_OlderSlotIgnored(t *testing.T) {
	h := newHeadTracker()
	require.NoError(t, h.update(11, hexRoot(0xbb)))
	require.NoError(t, h.update(10, hexRoot(0xaa))) // older, must be ignored

	root, slot, _ := latestHead(h)
	assert.Equal(t, primitives.Slot(11), slot)
	assert.Equal(t, byte(0xbb), root[0])
}

func TestHeadTracker_SameSlotReorgReplaces(t *testing.T) {
	h := newHeadTracker()
	require.NoError(t, h.update(11, hexRoot(0xbb)))
	require.NoError(t, h.update(11, hexRoot(0xcc))) // same slot, different root wins

	root, _, _ := latestHead(h)
	assert.Equal(t, byte(0xcc), root[0])
}

func TestHeadTracker_MalformedRootErrors(t *testing.T) {
	h := newHeadTracker()
	require.NotNil(t, h.update(11, "0xnothex"))

	_, _, ok := latestHead(h)
	assert.Equal(t, false, ok)
}

func TestHeadTracker_ConcurrentAccess(t *testing.T) {
	h := newHeadTracker()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func(i int) { defer wg.Done(); _ = h.update(primitives.Slot(i), hexRoot(byte(i))) }(i)
		go func() { defer wg.Done(); _, _, _ = latestHead(h) }()
	}
	wg.Wait()
}
