package cache

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestExecutionPayloadEnvelopeCache_GetSet(t *testing.T) {
	cache := NewExecutionPayloadEnvelopeCache()

	// Test empty cache returns false
	_, ok := cache.Get(1, 0)
	require.Equal(t, false, ok, "expected empty cache to return false")

	// Create test envelope
	envelope := &ethpb.ExecutionPayloadEnvelope{
		Slot:         primitives.Slot(100),
		BuilderIndex: primitives.BuilderIndex(5),
		StateRoot:    make([]byte, 32),
	}

	// Set and retrieve
	cache.Set(envelope, nil)
	retrieved, ok := cache.Get(100, 5)
	require.Equal(t, true, ok, "expected to find cached envelope")
	require.Equal(t, envelope.Slot, retrieved.Slot)
	require.Equal(t, envelope.BuilderIndex, retrieved.BuilderIndex)

	// Different builder index should not find it
	_, ok = cache.Get(100, 6)
	require.Equal(t, false, ok, "expected different builder index to return false")

	// Different slot should not find it
	_, ok = cache.Get(101, 5)
	require.Equal(t, false, ok, "expected different slot to return false")
}

func TestExecutionPayloadEnvelopeCache_Prune(t *testing.T) {
	cache := NewExecutionPayloadEnvelopeCache()

	// Add envelopes at slots 10, 11, 12 (close enough that none get pruned).
	// Prune removes entries with slot < (new_slot - 2), so inserting 10, 11, 12
	// keeps all three: slot 12 prunes < 10, but 10 is not < 10.
	for i := primitives.Slot(10); i <= 12; i++ {
		envelope := &ethpb.ExecutionPayloadEnvelope{
			Slot:         i,
			BuilderIndex: 0,
			StateRoot:    make([]byte, 32),
		}
		cache.Set(envelope, nil)
	}

	// Verify all are present
	for i := primitives.Slot(10); i <= 12; i++ {
		_, ok := cache.Get(i, 0)
		require.Equal(t, true, ok, "expected envelope at slot %d", i)
	}

	// Add envelope at slot 20, should prune slots < 18
	envelope := &ethpb.ExecutionPayloadEnvelope{
		Slot:         20,
		BuilderIndex: 0,
		StateRoot:    make([]byte, 32),
	}
	cache.Set(envelope, nil)

	// Slots 10-12 should be pruned (all < 18)
	for i := primitives.Slot(10); i <= 12; i++ {
		_, ok := cache.Get(i, 0)
		require.Equal(t, false, ok, "expected envelope at slot %d to be pruned", i)
	}

	// Slot 20 should still be present
	_, ok := cache.Get(20, 0)
	require.Equal(t, true, ok, "expected envelope at slot 20 to be present")
}

func TestExecutionPayloadEnvelopeCache_NilEnvelope(t *testing.T) {
	cache := NewExecutionPayloadEnvelopeCache()

	// Setting nil should not panic or add entry
	cache.Set(nil, nil)

	// Cache should still be empty
	require.Equal(t, 0, len(cache.cache))
}

func TestExecutionPayloadEnvelopeCache_MultipleBuilders(t *testing.T) {
	cache := NewExecutionPayloadEnvelopeCache()

	slot := primitives.Slot(100)

	// Add envelopes from multiple builders at same slot
	for i := range primitives.BuilderIndex(3) {
		envelope := &ethpb.ExecutionPayloadEnvelope{
			Slot:         slot,
			BuilderIndex: i,
			StateRoot:    make([]byte, 32),
		}
		cache.Set(envelope, nil)
	}

	// All should be retrievable
	for i := range primitives.BuilderIndex(3) {
		retrieved, ok := cache.Get(slot, i)
		require.Equal(t, true, ok, "expected to find envelope for builder %d", i)
		require.Equal(t, i, retrieved.BuilderIndex)
	}
}

func TestExecutionPayloadEnvelopeCache_BlobsBundle(t *testing.T) {
	cache := NewExecutionPayloadEnvelopeCache()

	slot := primitives.Slot(100)
	builderIndex := primitives.BuilderIndex(5)

	envelope := &ethpb.ExecutionPayloadEnvelope{
		Slot:         slot,
		BuilderIndex: builderIndex,
		StateRoot:    make([]byte, 32),
	}
	bundle := &enginev1.BlobsBundle{
		KzgCommitments: [][]byte{{1, 2, 3}},
		Proofs:         [][]byte{{4, 5, 6}},
		Blobs:          [][]byte{{7, 8, 9}},
	}

	cache.Set(envelope, bundle)

	// Retrieve blobs bundle
	retrieved, ok := cache.GetBlobsBundle(slot, builderIndex)
	require.Equal(t, true, ok, "expected to find cached blobs bundle")
	require.NotNil(t, retrieved)
	b, ok := retrieved.(*enginev1.BlobsBundle)
	require.Equal(t, true, ok)
	require.Equal(t, 1, len(b.KzgCommitments))
	require.DeepEqual(t, []byte{1, 2, 3}, b.KzgCommitments[0])

	// Nil blobs bundle for missing key
	_, ok = cache.GetBlobsBundle(slot, 99)
	require.Equal(t, false, ok, "expected missing key to return false")
}
