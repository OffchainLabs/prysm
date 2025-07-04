package cache

import (
	"sync"

	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
)

// SlotToPredictionIDMap is a map with keys the head root and values the
// corresponding PredictionID
type SlotToPredictionIDMap map[[32]byte]primitives.PredictionID

// PredictionIDCache is a cache that keeps track of the prepared payload ID for the
// given slot and with the given head root.
type PredictionIDCache struct {
	slotToPredictionID map[primitives.Slot]SlotToPredictionIDMap
	sync.Mutex
}

// NewPredictionIDCache returns a new payload ID cache
func NewPredictionIDCache() *PredictionIDCache {
	return &PredictionIDCache{slotToPredictionID: make(map[primitives.Slot]SlotToPredictionIDMap)}
}

// PredictionID returns the payload ID for the given slot and parent block root
func (p *PredictionIDCache) PredictionID(slot primitives.Slot, root [32]byte) (primitives.PredictionID, bool) {
	p.Lock()
	defer p.Unlock()
	inner, ok := p.slotToPredictionID[slot]
	if !ok {
		return primitives.PredictionID{}, false
	}
	pid, ok := inner[root]
	if !ok {
		return primitives.PredictionID{}, false
	}
	return pid, true
}

// SetPredictionID updates the payload ID for the given slot and head root
// it also prunes older entries in the cache
func (p *PredictionIDCache) Set(slot primitives.Slot, root [32]byte, pid primitives.PredictionID) {
	p.Lock()
	defer p.Unlock()
	if slot > 1 {
		p.prune(slot - 2)
	}
	inner, ok := p.slotToPredictionID[slot]
	if !ok {
		inner = make(SlotToPredictionIDMap)
		p.slotToPredictionID[slot] = inner
	}
	inner[root] = pid
}

// Prune prunes old payload IDs. Requires a Lock in the cache
func (p *PredictionIDCache) prune(slot primitives.Slot) {
	for key := range p.slotToPredictionID {
		if key < slot {
			delete(p.slotToPredictionID, key)
		}
	}
}
