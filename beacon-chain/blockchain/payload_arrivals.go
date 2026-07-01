package blockchain

import (
	"sync"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
)

type payloadArrival struct {
	slot  primitives.Slot
	early bool
}

type payloadArrivals struct {
	sync.Mutex
	entries map[[32]byte]payloadArrival
}

func newPayloadArrivals() *payloadArrivals {
	return &payloadArrivals{entries: make(map[[32]byte]payloadArrival)}
}

func (p *payloadArrivals) record(root [32]byte, slot primitives.Slot, early bool) {
	p.Lock()
	defer p.Unlock()
	p.entries[root] = payloadArrival{slot: slot, early: early}
	if slot > 1 {
		cutoff := slot - 1
		for r, pa := range p.entries {
			if pa.slot < cutoff {
				delete(p.entries, r)
			}
		}
	}
}

func (p *payloadArrivals) isEarly(root [32]byte) (bool, bool) {
	p.Lock()
	defer p.Unlock()
	pa, ok := p.entries[root]
	if !ok {
		return false, false
	}
	return pa.early, true
}
