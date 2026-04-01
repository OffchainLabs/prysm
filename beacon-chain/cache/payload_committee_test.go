//go:build !fuzz

package cache

import (
	"context"
	"strconv"
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestPayloadCommitteeCache_MissOnEmpty(t *testing.T) {
	c := NewPayloadCommitteeCache()
	seed := [32]byte{'A'}
	indices, err := c.Get(t.Context(), seed)
	require.NoError(t, err)
	assert.Equal(t, true, indices == nil, "Expected nil on empty cache")
}

func TestPayloadCommitteeCache_AddThenHit(t *testing.T) {
	c := NewPayloadCommitteeCache()
	seed := [32]byte{'A'}
	want := []primitives.ValidatorIndex{1, 2, 3, 4, 5}

	c.Add(seed, want)

	got, err := c.Get(t.Context(), seed)
	require.NoError(t, err)
	assert.DeepEqual(t, want, got)
}

func TestPayloadCommitteeCache_LRUEviction(t *testing.T) {
	c := NewPayloadCommitteeCache()

	// Fill beyond capacity.
	for i := range maxPayloadCommitteeCacheSize + 10 {
		s := bytesutil.ToBytes32([]byte(strconv.Itoa(i)))
		c.Add(s, []primitives.ValidatorIndex{primitives.ValidatorIndex(i)})
	}

	// Oldest entries should be evicted.
	s := bytesutil.ToBytes32([]byte(strconv.Itoa(0)))
	got, err := c.Get(t.Context(), s)
	require.NoError(t, err)
	assert.Equal(t, true, got == nil, "Expected oldest entry to be evicted")

	// Newest entry should still be present.
	s = bytesutil.ToBytes32([]byte(strconv.Itoa(maxPayloadCommitteeCacheSize + 9)))
	got, err = c.Get(t.Context(), s)
	require.NoError(t, err)
	assert.Equal(t, 1, len(got))
}

func TestPayloadCommitteeCache_CancelledContext(t *testing.T) {
	c := NewPayloadCommitteeCache()
	seed := [32]byte{'A'}

	// Mark in progress so Get blocks.
	require.NoError(t, c.MarkInProgress(seed))

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := c.Get(ctx, seed)
	require.ErrorIs(t, err, context.Canceled)

	require.NoError(t, c.MarkNotInProgress(seed))
}

func TestPayloadCommitteeCache_MarkInProgressDuplicate(t *testing.T) {
	c := NewPayloadCommitteeCache()
	seed := [32]byte{'A'}

	require.NoError(t, c.MarkInProgress(seed))
	err := c.MarkInProgress(seed)
	assert.Equal(t, ErrAlreadyInProgress, err)
	require.NoError(t, c.MarkNotInProgress(seed))
}

func TestPayloadCommitteeCache_Clear(t *testing.T) {
	c := NewPayloadCommitteeCache()
	seed := [32]byte{'A'}
	c.Add(seed, []primitives.ValidatorIndex{1, 2, 3})

	c.Clear()

	got, err := c.Get(t.Context(), seed)
	require.NoError(t, err)
	assert.Equal(t, true, got == nil, "Expected nil after Clear")
}
