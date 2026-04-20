package beacon_api

import (
	"sync"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// executionPayloadEnvelopeCache is a small slot-keyed cache used by the
// stateless block production path to carry the execution payload envelope from
// the /eth/v4/validator/blocks response to the self-build envelope publisher,
// avoiding a redundant /eth/v1/validator/execution_payload_envelope fetch.
type executionPayloadEnvelopeCache struct {
	mu        sync.Mutex
	envelopes map[primitives.Slot]*ethpb.ExecutionPayloadEnvelope
}

func newExecutionPayloadEnvelopeCache() *executionPayloadEnvelopeCache {
	return &executionPayloadEnvelopeCache{
		envelopes: make(map[primitives.Slot]*ethpb.ExecutionPayloadEnvelope),
	}
}

// Add stores an envelope for the given slot and drops any entry for an older
// slot — once we're writing for a newer slot, any lingering entry belongs to
// an aborted proposal and will never be consumed. No-op on a nil receiver so
// callers that construct the client without initializing the cache (e.g. tests
// exercising unrelated paths) do not panic.
func (c *executionPayloadEnvelopeCache) Add(slot primitives.Slot, envelope *ethpb.ExecutionPayloadEnvelope) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for s := range c.envelopes {
		if s < slot {
			delete(c.envelopes, s)
		}
	}
	c.envelopes[slot] = envelope
}

// Take returns the cached envelope for the given slot and removes it from the
// cache. Returns nil if no entry is present or the receiver is nil.
func (c *executionPayloadEnvelopeCache) Take(slot primitives.Slot) *ethpb.ExecutionPayloadEnvelope {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	envelope, ok := c.envelopes[slot]
	if !ok {
		return nil
	}
	delete(c.envelopes, slot)
	return envelope
}
