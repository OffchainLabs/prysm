// Package cache holds in-memory caches shared across the validator client's API backends
// (beacon-api and grpc-api). It is the validator-side analog of beacon-chain/cache, which is
// walled off from the validator by Bazel visibility.
package cache

import (
	"sync"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// contents bundles the cached execution payload envelope with the raw blobs and KZG proofs so the
// stateless publish path can submit them together to the beacon node.
type contents struct {
	envelope  *ethpb.ExecutionPayloadEnvelope
	blobs     [][]byte
	kzgProofs [][]byte
}

// ExecutionPayloadEnvelopeCache is a slot-keyed cache that carries the execution payload envelope
// and its blob data from block production to the self-build envelope publisher, avoiding a second
// fetch. It is the validator-side counterpart to beacon-chain/cache's same-named BN cache.
type ExecutionPayloadEnvelopeCache struct {
	mu      sync.Mutex
	entries map[primitives.Slot]*contents
}

// NewExecutionPayloadEnvelopeCache returns an initialized ExecutionPayloadEnvelopeCache.
func NewExecutionPayloadEnvelopeCache() *ExecutionPayloadEnvelopeCache {
	return &ExecutionPayloadEnvelopeCache{entries: make(map[primitives.Slot]*contents)}
}

// Add stores an envelope and its blob data for the slot and drops entries for older slots (a
// lingering older entry belongs to an aborted proposal). No-op on a nil receiver.
func (c *ExecutionPayloadEnvelopeCache) Add(slot primitives.Slot, envelope *ethpb.ExecutionPayloadEnvelope, blobs, kzgProofs [][]byte) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.evictOlderThan(slot)
	c.entries[slot] = &contents{envelope: envelope, blobs: blobs, kzgProofs: kzgProofs}
}

// Peek returns the cached envelope and blob data for the slot without removing the entry.
func (c *ExecutionPayloadEnvelopeCache) Peek(slot primitives.Slot) (*ethpb.ExecutionPayloadEnvelope, [][]byte, [][]byte) {
	if c == nil {
		return nil, nil, nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.evictOlderThan(slot)
	entry, ok := c.entries[slot]
	if !ok {
		return nil, nil, nil
	}
	return entry.envelope, entry.blobs, entry.kzgProofs
}

// Take returns the cached envelope and blob data for the slot and removes the entry.
func (c *ExecutionPayloadEnvelopeCache) Take(slot primitives.Slot) (*ethpb.ExecutionPayloadEnvelope, [][]byte, [][]byte) {
	if c == nil {
		return nil, nil, nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.evictOlderThan(slot)
	entry, ok := c.entries[slot]
	if !ok {
		return nil, nil, nil
	}
	delete(c.entries, slot)
	return entry.envelope, entry.blobs, entry.kzgProofs
}

// evictOlderThan drops entries strictly older than slot. Caller must hold c.mu.
func (c *ExecutionPayloadEnvelopeCache) evictOlderThan(slot primitives.Slot) {
	for s := range c.entries {
		if s < slot {
			delete(c.entries, s)
		}
	}
}
