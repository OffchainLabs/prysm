package beacon_api

import (
	"sync"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// envelopeContents bundles the cached execution payload envelope with the raw
// blobs and KZG proofs returned from /eth/v4/validator/blocks?include_payload=true,
// so the stateless publish path can submit them together to the beacon node.
type envelopeContents struct {
	envelope  *ethpb.ExecutionPayloadEnvelope
	blobs     [][]byte
	kzgProofs [][]byte
}

// executionPayloadEnvelopeCache is a small slot-keyed cache used by the
// stateless block production path to carry the execution payload envelope and
// its associated blob data from the /eth/v4/validator/blocks response to the
// self-build envelope publisher, avoiding a redundant
// /eth/v1/validator/execution_payload_envelope fetch.
type executionPayloadEnvelopeCache struct {
	mu      sync.Mutex
	entries map[primitives.Slot]*envelopeContents
}

func newExecutionPayloadEnvelopeCache() *executionPayloadEnvelopeCache {
	return &executionPayloadEnvelopeCache{
		entries: make(map[primitives.Slot]*envelopeContents),
	}
}

// Add stores an envelope and its blob data for the given slot and drops any
// entry for an older slot — once we're writing for a newer slot, any lingering
// entry belongs to an aborted proposal and will never be consumed. No-op on a
// nil receiver so callers that construct the client without initializing the
// cache (e.g. tests exercising unrelated paths) do not panic.
func (c *executionPayloadEnvelopeCache) Add(slot primitives.Slot, envelope *ethpb.ExecutionPayloadEnvelope, blobs, kzgProofs [][]byte) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.evictOlderThan(slot)
	c.entries[slot] = &envelopeContents{
		envelope:  envelope,
		blobs:     blobs,
		kzgProofs: kzgProofs,
	}
}

// peek returns the cached envelope and blob data for the slot without removing
// the entry. Used by the envelope-fetch path so the entry stays available for
// the subsequent publish call.
func (c *executionPayloadEnvelopeCache) peek(slot primitives.Slot) (*ethpb.ExecutionPayloadEnvelope, [][]byte, [][]byte) {
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

// Take returns the cached envelope and blob data for the given slot and removes
// the entry from the cache. Returns nils if no entry is present or the receiver
// is nil.
func (c *executionPayloadEnvelopeCache) Take(slot primitives.Slot) (*ethpb.ExecutionPayloadEnvelope, [][]byte, [][]byte) {
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

// evictOlderThan drops any entries strictly older than slot. Caller must hold c.mu.
func (c *executionPayloadEnvelopeCache) evictOlderThan(slot primitives.Slot) {
	for s := range c.entries {
		if s < slot {
			delete(c.entries, s)
		}
	}
}
