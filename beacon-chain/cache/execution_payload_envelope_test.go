package cache

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
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
	cache.Set(envelope)
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

	// Add envelopes at different slots
	for i := primitives.Slot(1); i <= 10; i++ {
		envelope := &ethpb.ExecutionPayloadEnvelope{
			Slot:         i,
			BuilderIndex: 0,
			StateRoot:    make([]byte, 32),
		}
		cache.Set(envelope)
	}

	// Verify all are present
	for i := primitives.Slot(1); i <= 10; i++ {
		_, ok := cache.Get(i, 0)
		require.Equal(t, true, ok, "expected envelope at slot %d", i)
	}

	// Add envelope at slot 15, should prune slots < 13
	envelope := &ethpb.ExecutionPayloadEnvelope{
		Slot:         15,
		BuilderIndex: 0,
		StateRoot:    make([]byte, 32),
	}
	cache.Set(envelope)

	// Slots 1-12 should be pruned
	for i := primitives.Slot(1); i <= 12; i++ {
		_, ok := cache.Get(i, 0)
		require.Equal(t, false, ok, "expected envelope at slot %d to be pruned", i)
	}

	// Slot 15 should still be present
	_, ok := cache.Get(15, 0)
	require.Equal(t, true, ok, "expected envelope at slot 15 to be present")
}

func TestExecutionPayloadEnvelopeCache_NilEnvelope(t *testing.T) {
	cache := NewExecutionPayloadEnvelopeCache()

	// Setting nil should not panic or add entry
	cache.Set(nil)

	// Cache should still be empty
	require.Equal(t, 0, len(cache.cache))
}

func TestExecutionPayloadEnvelopeCache_MultipleBuilders(t *testing.T) {
	cache := NewExecutionPayloadEnvelopeCache()

	slot := primitives.Slot(100)

	// Add envelopes from multiple builders at same slot
	for i := primitives.BuilderIndex(0); i < 3; i++ {
		envelope := &ethpb.ExecutionPayloadEnvelope{
			Slot:         slot,
			BuilderIndex: i,
			StateRoot:    make([]byte, 32),
		}
		cache.Set(envelope)
	}

	// All should be retrievable
	for i := primitives.BuilderIndex(0); i < 3; i++ {
		retrieved, ok := cache.Get(slot, i)
		require.Equal(t, true, ok, "expected to find envelope for builder %d", i)
		require.Equal(t, i, retrieved.BuilderIndex)
	}
}
