package cache

import (
	"sync"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// ExecutionPayloadEnvelopeKey uniquely identifies a cached execution payload envelope.
type ExecutionPayloadEnvelopeKey struct {
	Slot         primitives.Slot
	BuilderIndex primitives.BuilderIndex
}

// executionPayloadEnvelopeCacheEntry holds an execution payload envelope and
// the associated blobs bundle from the EL. The blobs bundle is needed later
// when proposing the block to build and broadcast blob sidecars.
type executionPayloadEnvelopeCacheEntry struct {
	envelope    *ethpb.ExecutionPayloadEnvelope
	blobsBundle enginev1.BlobsBundler
}

// ExecutionPayloadEnvelopeCache stores execution payload envelopes produced during
// GLOAS block building for later retrieval by validators. When a beacon node
// produces a GLOAS block, it caches the execution payload envelope so the validator
// can retrieve it, sign it, and broadcast it separately from the beacon block.
// The blobs bundle from the EL is also cached alongside, since blobs are only
// persisted to the DB after they are broadcast as sidecars during block proposal.
type ExecutionPayloadEnvelopeCache struct {
	cache map[ExecutionPayloadEnvelopeKey]*executionPayloadEnvelopeCacheEntry
	sync.RWMutex
}

// NewExecutionPayloadEnvelopeCache creates a new execution payload envelope cache.
func NewExecutionPayloadEnvelopeCache() *ExecutionPayloadEnvelopeCache {
	return &ExecutionPayloadEnvelopeCache{
		cache: make(map[ExecutionPayloadEnvelopeKey]*executionPayloadEnvelopeCacheEntry),
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
	entry, ok := c.cache[key]
	if !ok {
		return nil, false
	}
	return entry.envelope, true
}

// GetBlobsBundle retrieves a cached blobs bundle by slot and builder index.
// Returns the blobs bundle and true if found, nil and false otherwise.
func (c *ExecutionPayloadEnvelopeCache) GetBlobsBundle(slot primitives.Slot, builderIndex primitives.BuilderIndex) (enginev1.BlobsBundler, bool) {
	c.RLock()
	defer c.RUnlock()

	key := ExecutionPayloadEnvelopeKey{
		Slot:         slot,
		BuilderIndex: builderIndex,
	}
	entry, ok := c.cache[key]
	if !ok {
		return nil, false
	}
	return entry.blobsBundle, true
}

// Set stores an execution payload envelope and its associated blobs bundle in the cache.
// The envelope's slot and builder_index fields are used as the cache key.
// Old entries are automatically pruned to prevent unbounded growth.
func (c *ExecutionPayloadEnvelopeCache) Set(envelope *ethpb.ExecutionPayloadEnvelope, blobsBundle enginev1.BlobsBundler) {
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
	c.cache[key] = &executionPayloadEnvelopeCacheEntry{
		envelope:    envelope,
		blobsBundle: blobsBundle,
	}
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
