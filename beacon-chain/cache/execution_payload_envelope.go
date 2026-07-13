package cache

import (
	"sync"

	consensusblocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// ExecutionPayloadContents holds the producer's envelope with precomputed
// data column sidecars; raw blobs/proofs are derived from the columns at read
// time so the publish hot path skips the KZG cell extension.
type ExecutionPayloadContents struct {
	Envelope    *ethpb.ExecutionPayloadEnvelope
	DataColumns []consensusblocks.RODataColumn
}

// ExecutionPayloadEnvelopeCache holds the most recent ExecutionPayloadContents
// produced by the proposer. Single-entry; Set replaces.
type ExecutionPayloadEnvelopeCache struct {
	mu       sync.RWMutex
	contents *ExecutionPayloadContents
}

func NewExecutionPayloadEnvelopeCache() *ExecutionPayloadEnvelopeCache {
	return &ExecutionPayloadEnvelopeCache{}
}

// Set replaces the cached contents. No-op on nil receiver/contents/envelope so
// readers can treat Envelope and Envelope.Payload as non-nil on a hit.
func (c *ExecutionPayloadEnvelopeCache) Set(contents *ExecutionPayloadContents) {
	if c == nil || contents == nil || contents.Envelope == nil || contents.Envelope.Payload == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.contents = contents
}

// Contents returns a snapshot of the cached bundle. The struct is freshly
// allocated; inner slices alias the cache (safe — Set re-assigns whole).
func (c *ExecutionPayloadEnvelopeCache) Contents() (*ExecutionPayloadContents, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.contents == nil {
		return nil, false
	}
	snapshot := *c.contents
	return &snapshot, true
}
