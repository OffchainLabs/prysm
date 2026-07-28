package client

import (
	"sync"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
)

// payloadAvailability releases per-slot waiters when an execution_payload_available
// event is received, so PTC members can attest as soon as the payload and blobs are
// available rather than waiting for the attestation deadline.
type payloadAvailability struct {
	mu    sync.Mutex
	chans map[primitives.Slot]chan struct{}
}

func newPayloadAvailability() *payloadAvailability {
	return &payloadAvailability{chans: make(map[primitives.Slot]chan struct{})}
}

// waiter returns a channel closed once the payload for slot is available.
func (p *payloadAvailability) waiter(slot primitives.Slot) <-chan struct{} {
	p.mu.Lock()
	defer p.mu.Unlock()
	ch, ok := p.chans[slot]
	if !ok {
		ch = make(chan struct{})
		p.chans[slot] = ch
	}
	return ch
}

// notify releases waiters for slot and prunes older slots. A closed channel is
// stored so a waiter that registers after the event still observes availability.
func (p *payloadAvailability) notify(slot primitives.Slot) {
	p.mu.Lock()
	defer p.mu.Unlock()
	ch, ok := p.chans[slot]
	if !ok {
		ch = make(chan struct{})
		p.chans[slot] = ch
	}
	select {
	case <-ch:
	default:
		close(ch)
	}
	for s := range p.chans {
		if s < slot {
			delete(p.chans, s)
		}
	}
}
