package cache

import (
	"sync"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
)

// PayloadAttestationCache tracks seen payload attestation messages for a single slot.
type PayloadAttestationCache struct {
	slot primitives.Slot
	seen map[primitives.ValidatorIndex]struct{}
	mu   sync.RWMutex
}

// Seen returns true if a vote for the given slot has already been
// processed for this validator index.
func (p *PayloadAttestationCache) Seen(slot primitives.Slot, idx primitives.ValidatorIndex) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.slot != slot {
		return false
	}
	if p.seen == nil {
		return false
	}
	_, ok := p.seen[idx]
	return ok
}

// Add marks the given slot and validator index as seen.
// This function assumes that the message has already been validated.
func (p *PayloadAttestationCache) Add(slot primitives.Slot, idx primitives.ValidatorIndex) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.slot != slot {
		p.slot = slot
		p.seen = make(map[primitives.ValidatorIndex]struct{})
	}
	if p.seen == nil {
		p.seen = make(map[primitives.ValidatorIndex]struct{})
	}
	p.seen[idx] = struct{}{}
	return nil
}

// Clear clears the internal cache.
func (p *PayloadAttestationCache) Clear() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.slot = 0
	p.seen = nil
}
