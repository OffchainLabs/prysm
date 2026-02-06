package cache

import (
	"sync"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// ExecutionPayloadEnvelopeKey uniquely identifies a cached execution payload envelope.
type ExecutionPayloadEnvelopeKey struct {
	Slot         primitives.Slot
	BuilderIndex primitives.BuilderIndex
}

// ExecutionPayloadEnvelopeCache stores execution payload envelopes produced during
// GLOAS block building for later retrieval by validators. When a beacon node
// produces a GLOAS block, it caches the execution payload envelope so the validator
// can retrieve it, sign it, and broadcast it separately from the beacon block.
type ExecutionPayloadEnvelopeCache struct {
	cache map[ExecutionPayloadEnvelopeKey]*ethpb.ExecutionPayloadEnvelope
	sync.RWMutex
}

// NewExecutionPayloadEnvelopeCache creates a new execution payload envelope cache.
func NewExecutionPayloadEnvelopeCache() *ExecutionPayloadEnvelopeCache {
	return &ExecutionPayloadEnvelopeCache{
		cache: make(map[ExecutionPayloadEnvelopeKey]*ethpb.ExecutionPayloadEnvelope),
	}
}

// Get retrieves an execution payload envelope by slot and builder index.
// Returns the envelope and true if found, nil and false otherwise.
func (c *ExecutionPayloadEnvelopeCache) Get(slot primitives.Slot, builderIndex primitives.BuilderIndex) (*ethpb.ExecutionPayloadEnvelope, bool) {
	c.RLock()
	defer c.RUnlock()

	key := ExecutionPayloadEnvelopeKey{
		Slot:         slot,
		BuilderIndex: builderIndex,
	}
	envelope, ok := c.cache[key]
	return envelope, ok
}

// Set stores an execution payload envelope in the cache.
// The envelope's slot and builder_index fields are used as the cache key.
// Old entries are automatically pruned to prevent unbounded growth.
func (c *ExecutionPayloadEnvelopeCache) Set(envelope *ethpb.ExecutionPayloadEnvelope) {
	if envelope == nil {
		return
	}

	c.Lock()
	defer c.Unlock()

	slot := envelope.Slot
	if slot > 2 {
		c.prune(slot - 2)
	}

	key := ExecutionPayloadEnvelopeKey{
		Slot:         slot,
		BuilderIndex: envelope.BuilderIndex,
	}
	c.cache[key] = envelope
}

// prune removes all entries with slots older than the given slot.
// Must be called with the lock held.
func (c *ExecutionPayloadEnvelopeCache) prune(slot primitives.Slot) {
	for key := range c.cache {
		if key.Slot < slot {
			delete(c.cache, key)
		}
	}
}
