package cache

import (
	"sync"

	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
)

// RootToPayloadIDMap is a map with keys the head root and values the
// corresponding PayloadID
type RootToPayloadIDMap map[[32]byte]primitives.PayloadID

// PayloadIDCache is a cache that keeps track of the prepared payload ID for the
// given slot and with the given head root.
type PayloadIDCache struct {
	slotToPayloadID map[primitives.Slot]RootToPayloadIDMap
	sync.Mutex
}

// NewPayloadIDCache returns a new payload ID cache
func NewPayloadIDCache() *PayloadIDCache {
	return &PayloadIDCache{slotToPayloadID: make(map[primitives.Slot]RootToPayloadIDMap)}
}

// PayloadID returns the payload ID for the given slot and parent block root
func (p *PayloadIDCache) PayloadID(slot primitives.Slot, root [32]byte) (primitives.PayloadID, bool) {
	p.Lock()
	defer p.Unlock()
	inner, ok := p.slotToPayloadID[slot]
	if !ok {
		return primitives.PayloadID{}, false
	}
	pid, ok := inner[root]
	if !ok {
		return primitives.PayloadID{}, false
	}
	return pid, true
}

// SetPayloadID updates the payload ID for the given slot and head root
// it also prunes older entries in the cache
func (p *PayloadIDCache) Set(slot primitives.Slot, root [32]byte, pid primitives.PayloadID) {
	p.Lock()
	defer p.Unlock()
	if slot > 1 {
		p.prune(slot - 2)
	}
	inner, ok := p.slotToPayloadID[slot]
	if !ok {
		inner = make(RootToPayloadIDMap)
		p.slotToPayloadID[slot] = inner
	}
	inner[root] = pid
}

// Prune prunes old payload IDs. Requires a Lock in the cache
func (p *PayloadIDCache) prune(slot primitives.Slot) {
	for key := range p.slotToPayloadID {
		if key < slot {
			delete(p.slotToPayloadID, key)
		}
	}
}
