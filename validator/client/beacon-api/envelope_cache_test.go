package beacon_api

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func envelopeForSlot(slot primitives.Slot) *ethpb.ExecutionPayloadEnvelope {
	return &ethpb.ExecutionPayloadEnvelope{
		Payload: &enginev1.ExecutionPayloadGloas{SlotNumber: slot},
	}
}

func TestExecutionPayloadEnvelopeCache_Add(t *testing.T) {
	t.Run("evicts older slots", func(t *testing.T) {
		cache := newExecutionPayloadEnvelopeCache()
		cache.Add(10, envelopeForSlot(10), nil, nil)
		cache.Add(11, envelopeForSlot(11), nil, nil)

		got, _, _ := cache.Take(10)
		assert.Equal(t, (*ethpb.ExecutionPayloadEnvelope)(nil), got)
		got, _, _ = cache.Take(11)
		require.NotNil(t, got)
		assert.Equal(t, primitives.Slot(11), got.Payload.SlotNumber)
	})

	t.Run("keeps newer slot when adding for older slot", func(t *testing.T) {
		cache := newExecutionPayloadEnvelopeCache()
		cache.Add(11, envelopeForSlot(11), nil, nil)
		cache.Add(10, envelopeForSlot(10), nil, nil)

		got, _, _ := cache.Take(10)
		require.NotNil(t, got)
		got, _, _ = cache.Take(11)
		require.NotNil(t, got)
	})

	t.Run("nil receiver is a no-op", func(t *testing.T) {
		var cache *executionPayloadEnvelopeCache
		cache.Add(1, &ethpb.ExecutionPayloadEnvelope{}, nil, nil)
	})
}

func TestExecutionPayloadEnvelopeCache_Take(t *testing.T) {
	t.Run("returns stored envelope and evicts entry", func(t *testing.T) {
		cache := newExecutionPayloadEnvelopeCache()
		envelope := envelopeForSlot(10)
		blobs := [][]byte{{0xaa}}
		proofs := [][]byte{{0xbb}}
		cache.Add(10, envelope, blobs, proofs)

		got, gotBlobs, gotProofs := cache.Take(10)
		require.NotNil(t, got)
		assert.Equal(t, primitives.Slot(10), got.Payload.SlotNumber)
		assert.DeepEqual(t, blobs, gotBlobs)
		assert.DeepEqual(t, proofs, gotProofs)

		got, _, _ = cache.Take(10)
		assert.Equal(t, (*ethpb.ExecutionPayloadEnvelope)(nil), got)
	})

	t.Run("missing slot returns nils", func(t *testing.T) {
		cache := newExecutionPayloadEnvelopeCache()
		got, _, _ := cache.Take(42)
		assert.Equal(t, (*ethpb.ExecutionPayloadEnvelope)(nil), got)
	})

	t.Run("nil receiver returns nils", func(t *testing.T) {
		var cache *executionPayloadEnvelopeCache
		got, _, _ := cache.Take(1)
		assert.Equal(t, (*ethpb.ExecutionPayloadEnvelope)(nil), got)
	})
}
