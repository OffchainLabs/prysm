package cache

import (
	"sync"

	consensusblocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// ExecutionPayloadContents bundles the latest self-built execution payload
// envelope with its precomputed data column sidecars and the raw blob bundle
// the producer used. The fields are always written and read together so
// callers never see an envelope from one block alongside blobs from another.
type ExecutionPayloadContents struct {
	Envelope    *ethpb.ExecutionPayloadEnvelope
	DataColumns []consensusblocks.RODataColumn
	Blobs       [][]byte
	KzgProofs   [][]byte
}

// ExecutionPayloadEnvelopeCache holds the most recent ExecutionPayloadContents
// produced by the proposer. It backs:
//   - The Gloas validator gRPC GetExecutionPayloadEnvelope endpoint.
//   - The v4 ProduceBlock include_payload=true response (raw blobs/proofs).
//   - The publish-time data column broadcast that runs before
//     ReceiveExecutionPayloadEnvelope checks data availability.
//
// The cache holds at most one entry; Set replaces the current entry.
type ExecutionPayloadEnvelopeCache struct {
	mu       sync.RWMutex
	contents *ExecutionPayloadContents
}

// NewExecutionPayloadEnvelopeCache returns an empty cache.
func NewExecutionPayloadEnvelopeCache() *ExecutionPayloadEnvelopeCache {
	return &ExecutionPayloadEnvelopeCache{}
}

// Set replaces the cached contents atomically. No-op on a nil receiver, nil
// contents, or contents without a fully-populated envelope. Enforcing
// Envelope.Payload != nil here lets readers treat that field as a guaranteed
// invariant of any cache hit.
func (c *ExecutionPayloadEnvelopeCache) Set(contents *ExecutionPayloadContents) {
	if c == nil || contents == nil || contents.Envelope == nil || contents.Envelope.Payload == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.contents = contents
}

// Contents returns the current cached bundle as a snapshot and a boolean
// indicating whether the cache held a valid entry. The returned struct is a
// fresh value; the slices inside are shared with the cache but the cache only
// ever re-assigns them whole, so a caller's reference remains stable for the
// lifetime of its snapshot. When ok is true, Envelope and Envelope.Payload are
// guaranteed non-nil.
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
