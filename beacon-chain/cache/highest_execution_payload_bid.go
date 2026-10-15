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
// the incoming value is strictly greater. It returns false for a nil or malformed bid.
func (c *HighestExecutionPayloadBidCache) SetIfHigher(bid interfaces.ROSignedExecutionPayloadBid) bool {
	if bid == nil {
		return false
	}
	msg, err := bid.Bid()
	if err != nil {
		return false
	}

	c.lock.Lock()
	defer c.lock.Unlock()

	key := executionPayloadBidKey{
		slot:       msg.Slot(),
		parentHash: msg.ParentBlockHash(),
		parentRoot: msg.ParentBlockRoot(),
	}
	if cached, ok := c.bids[key]; ok {
		if cachedMsg, err := cached.Bid(); err == nil && msg.Value() <= cachedMsg.Value() {
			return false
		}
	}
	c.bids[key] = bid
	return true
}

// HighestValue returns the value of the highest cached bid for the given tuple.
func (c *HighestExecutionPayloadBidCache) HighestValue(
	slot primitives.Slot,
	parentHash [32]byte,
	parentRoot [32]byte,
) (primitives.Gwei, bool) {
	bid, ok := c.Get(slot, parentHash, parentRoot)
	if !ok {
		return 0, false
	}
	msg, err := bid.Bid()
	if err != nil {
		return 0, false
	}
	return msg.Value(), true
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
