package client

import (
	"sync"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
)

// slotReservations dedups submissions keyed by proposal slot so each slot is
// pushed at most once. The zero value is ready to use.
type slotReservations struct {
	sync.Mutex
	slots map[primitives.Slot]bool
}

// prune resets the cache when force is set, otherwise drops reservations
// from before epochStart.
func (r *slotReservations) prune(force bool, epochStart primitives.Slot) {
	r.Lock()
	defer r.Unlock()
	if force || r.slots == nil {
		r.slots = make(map[primitives.Slot]bool)
		return
	}
	for s := range r.slots {
		if s < epochStart {
			delete(r.slots, s)
		}
	}
}

// reserve marks slot as submitted, returning false if another pass already claimed it.
func (r *slotReservations) reserve(slot primitives.Slot) bool {
	r.Lock()
	defer r.Unlock()
	if r.slots[slot] {
		return false
	}
	if r.slots == nil {
		r.slots = make(map[primitives.Slot]bool)
	}
	r.slots[slot] = true
	return true
}

// release un-reserves slots whose submission failed so a later pass retries them.
func (r *slotReservations) release(slots ...primitives.Slot) {
	r.Lock()
	defer r.Unlock()
	for _, s := range slots {
		delete(r.slots, s)
	}
}

func (r *slotReservations) count() int {
	r.Lock()
	defer r.Unlock()
	return len(r.slots)
}
