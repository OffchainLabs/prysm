package stateutil_test

import (
	"sync"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stateutil"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func makeValidators(n int) []*ethpb.Validator {
	vals := make([]*ethpb.Validator, n)
	for i := range n {
		pubkey := make([]byte, 48)
		pubkey[0] = byte(i)
		pubkey[1] = byte(i >> 8)
		pubkey[2] = byte(i >> 16)
		vals[i] = &ethpb.Validator{PublicKey: pubkey}
	}
	return vals
}

func TestGlobalValMapHandler_BuildsFromScratch(t *testing.T) {
	stateutil.ResetGlobalValMapHandler()
	defer stateutil.ResetGlobalValMapHandler()

	vals := makeValidators(100)
	h := stateutil.GlobalValMapHandler(vals)

	require.Equal(t, 100, h.Len())
	for i := range 100 {
		key := bytesutil.ToBytes48(vals[i].PublicKey)
		idx, ok := h.Get(key)
		require.Equal(t, true, ok)
		assert.Equal(t, primitives.ValidatorIndex(i), idx)
	}
}

func TestGlobalValMapHandler_ReturnsCachedForSameSize(t *testing.T) {
	stateutil.ResetGlobalValMapHandler()
	defer stateutil.ResetGlobalValMapHandler()

	vals := makeValidators(50)
	h1 := stateutil.GlobalValMapHandler(vals)
	h2 := stateutil.GlobalValMapHandler(vals)

	// Same pointer — no rebuild.
	assert.Equal(t, h1, h2, "expected same handler pointer for same-size validator list")
}

func TestGlobalValMapHandler_ReturnsCachedForSmallerList(t *testing.T) {
	stateutil.ResetGlobalValMapHandler()
	defer stateutil.ResetGlobalValMapHandler()

	vals := makeValidators(100)
	h1 := stateutil.GlobalValMapHandler(vals)
	h2 := stateutil.GlobalValMapHandler(vals[:50])

	// Smaller list should reuse the existing global map.
	assert.Equal(t, h1, h2, "expected same handler pointer for smaller validator list")
}

func TestGlobalValMapHandler_ExtendsDelta(t *testing.T) {
	stateutil.ResetGlobalValMapHandler()
	defer stateutil.ResetGlobalValMapHandler()

	vals := makeValidators(100)
	h1 := stateutil.GlobalValMapHandler(vals[:50])
	require.Equal(t, 50, h1.Len())

	// Extend with 50 more validators.
	h2 := stateutil.GlobalValMapHandler(vals)
	assert.Equal(t, h1, h2, "expected same handler pointer after extension")
	require.Equal(t, 100, h2.Len())

	// Verify new entries are accessible.
	for i := 50; i < 100; i++ {
		key := bytesutil.ToBytes48(vals[i].PublicKey)
		idx, ok := h2.Get(key)
		require.Equal(t, true, ok)
		assert.Equal(t, primitives.ValidatorIndex(i), idx)
	}
}

func TestGlobalValMapHandler_EmptyListNotCached(t *testing.T) {
	stateutil.ResetGlobalValMapHandler()
	defer stateutil.ResetGlobalValMapHandler()

	h1 := stateutil.GlobalValMapHandler(nil)
	h2 := stateutil.GlobalValMapHandler(nil)

	// Empty lists should return fresh handlers, not pollute global.
	assert.NotEqual(t, h1, h2, "empty handlers should not be cached globally")
}

func TestGlobalValMapHandler_Concurrent(t *testing.T) {
	stateutil.ResetGlobalValMapHandler()
	defer stateutil.ResetGlobalValMapHandler()

	vals := makeValidators(200)
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Each goroutine tries different subsets.
			_ = stateutil.GlobalValMapHandler(vals[:100])
			_ = stateutil.GlobalValMapHandler(vals[:150])
			_ = stateutil.GlobalValMapHandler(vals)
		}()
	}
	wg.Wait()

	h := stateutil.GlobalValMapHandler(vals)
	require.Equal(t, 200, h.Len())
}

func TestResetGlobalValMapHandler(t *testing.T) {
	stateutil.ResetGlobalValMapHandler()

	vals := makeValidators(50)
	h1 := stateutil.GlobalValMapHandler(vals)
	require.Equal(t, 50, h1.Len())

	stateutil.ResetGlobalValMapHandler()

	// After reset, a new handler should be built.
	h2 := stateutil.GlobalValMapHandler(vals)
	assert.NotEqual(t, h1, h2, "expected different handler after reset")
	require.Equal(t, 50, h2.Len())
}
