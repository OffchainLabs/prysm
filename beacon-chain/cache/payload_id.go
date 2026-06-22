package cache

import (
	"sync"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
)

type payloadIDKey struct {
	root [32]byte
	full bool
}

// PayloadIDCache is a cache that keeps track of the prepared payload ID for the
// given slot, head root and parent payload status.
type PayloadIDCache struct {
	slotToPayloadID map[primitives.Slot]map[payloadIDKey]primitives.PayloadID
	sync.Mutex
}

// NewPayloadIDCache returns a new payload ID cache
func NewPayloadIDCache() *PayloadIDCache {
	return &PayloadIDCache{slotToPayloadID: make(map[primitives.Slot]map[payloadIDKey]primitives.PayloadID)}
}

// PayloadID returns the payload ID for the given slot, parent block root and parent payload status
func (p *PayloadIDCache) PayloadID(slot primitives.Slot, root [32]byte, full bool) (primitives.PayloadID, bool) {
	p.Lock()
	defer p.Unlock()
	inner, ok := p.slotToPayloadID[slot]
	if !ok {
		return primitives.PayloadID{}, false
	}
	pid, ok := inner[payloadIDKey{root: root, full: full}]
	if !ok {
		return primitives.PayloadID{}, false
	}
	return pid, true
}

// SetPayloadID updates the payload ID for the given slot, head root and parent payload status
// it also prunes older entries in the cache
func (p *PayloadIDCache) Set(slot primitives.Slot, root [32]byte, full bool, pid primitives.PayloadID) {
	p.Lock()
	defer p.Unlock()
	if slot > 1 {
		p.prune(slot - 2)
	}
	inner, ok := p.slotToPayloadID[slot]
	if !ok {
		inner = make(map[payloadIDKey]primitives.PayloadID)
		p.slotToPayloadID[slot] = inner
	}
	inner[payloadIDKey{root: root, full: full}] = pid
}

// Prune prunes old payload IDs. Requires a Lock in the cache
func (p *PayloadIDCache) prune(slot primitives.Slot) {
	for key := range p.slotToPayloadID {
		if key < slot {
			delete(p.slotToPayloadID, key)
		}
	}
}
