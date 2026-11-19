package cache

import (
	"sync"

	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
)

type executionPayloadBidKey struct {
	slot       primitives.Slot
	parentHash [32]byte
	parentRoot [32]byte
}

// HighestExecutionPayloadBidCache stores the highest bid for each
// (slot, parent_block_hash, parent_block_root) tuple.
type HighestExecutionPayloadBidCache struct {
	bids map[executionPayloadBidKey]interfaces.ROSignedExecutionPayloadBid
	lock sync.RWMutex
}

// NewHighestExecutionPayloadBidCache initializes a highest-bid cache.
func NewHighestExecutionPayloadBidCache() *HighestExecutionPayloadBidCache {
	return &HighestExecutionPayloadBidCache{
		bids: make(map[executionPayloadBidKey]interfaces.ROSignedExecutionPayloadBid),
	}
}

// Get returns the highest cached bid for the given tuple.
func (c *HighestExecutionPayloadBidCache) Get(
	slot primitives.Slot,
	parentHash [32]byte,
	parentRoot [32]byte,
) (interfaces.ROSignedExecutionPayloadBid, bool) {
	c.lock.RLock()
	defer c.lock.RUnlock()

	bid, ok := c.bids[executionPayloadBidKey{
		slot:       slot,
		parentHash: parentHash,
		parentRoot: parentRoot,
	}]
	return bid, ok
}

// SetIfHigher inserts the bid if absent, or replaces the cached bid only if
// the incoming value is strictly greater. It returns false for a nil/invalid bid.
func (c *HighestExecutionPayloadBidCache) SetIfHigher(signedBid interfaces.ROSignedExecutionPayloadBid) bool {
	if signedBid == nil || signedBid.IsNil() {
		return false
	}
	msg := signedBid.Bid()

	c.lock.Lock()
	defer c.lock.Unlock()

	key := executionPayloadBidKey{
		slot:       msg.Slot(),
		parentHash: msg.ParentBlockHash(),
		parentRoot: msg.ParentBlockRoot(),
	}
	if cached, ok := c.bids[key]; ok && msg.Value() <= cached.Bid().Value() {
		return false
	}
	c.bids[key] = signedBid
	return true
}

// PruneBefore removes all cached bids for slots before the provided slot.
func (c *HighestExecutionPayloadBidCache) PruneBefore(slot primitives.Slot) {
	c.lock.Lock()
	defer c.lock.Unlock()

	for key := range c.bids {
		if key.slot < slot {
			delete(c.bids, key)
		}
	}
}
