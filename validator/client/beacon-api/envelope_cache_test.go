package beacon_api

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestExecutionPayloadEnvelopeCache_AddTake(t *testing.T) {
	cache := newExecutionPayloadEnvelopeCache()
	envelope := &ethpb.ExecutionPayloadEnvelope{Slot: 10}

	cache.Add(10, envelope)

	got := cache.Take(10)
	require.NotNil(t, got)
	assert.Equal(t, primitives.Slot(10), got.Slot)
}

func TestExecutionPayloadEnvelopeCache_TakeEvicts(t *testing.T) {
	cache := newExecutionPayloadEnvelopeCache()
	cache.Add(10, &ethpb.ExecutionPayloadEnvelope{Slot: 10})

	require.NotNil(t, cache.Take(10))
	assert.Equal(t, (*ethpb.ExecutionPayloadEnvelope)(nil), cache.Take(10))
}

func TestExecutionPayloadEnvelopeCache_TakeMissing(t *testing.T) {
	cache := newExecutionPayloadEnvelopeCache()
	assert.Equal(t, (*ethpb.ExecutionPayloadEnvelope)(nil), cache.Take(42))
}

func TestExecutionPayloadEnvelopeCache_AddEvictsOlderSlots(t *testing.T) {
	cache := newExecutionPayloadEnvelopeCache()
	cache.Add(10, &ethpb.ExecutionPayloadEnvelope{Slot: 10})
	cache.Add(11, &ethpb.ExecutionPayloadEnvelope{Slot: 11})

	assert.Equal(t, (*ethpb.ExecutionPayloadEnvelope)(nil), cache.Take(10))
	got := cache.Take(11)
	require.NotNil(t, got)
	assert.Equal(t, primitives.Slot(11), got.Slot)
}

func TestExecutionPayloadEnvelopeCache_AddKeepsNewerSlot(t *testing.T) {
	cache := newExecutionPayloadEnvelopeCache()
	cache.Add(11, &ethpb.ExecutionPayloadEnvelope{Slot: 11})
	// Adding for an older slot must not evict the newer entry.
	cache.Add(10, &ethpb.ExecutionPayloadEnvelope{Slot: 10})

	require.NotNil(t, cache.Take(10))
	require.NotNil(t, cache.Take(11))
}

func TestExecutionPayloadEnvelopeCache_NilReceiver(t *testing.T) {
	var cache *executionPayloadEnvelopeCache
	cache.Add(1, &ethpb.ExecutionPayloadEnvelope{})
	assert.Equal(t, (*ethpb.ExecutionPayloadEnvelope)(nil), cache.Take(1))
}
