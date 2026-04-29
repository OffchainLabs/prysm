package cache

import (
	"sync"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
)

// ProposerPreference is a broadcast preference anchored to a specific branch
// via CheckpointRoot (Gloas spec).
type ProposerPreference struct {
	CheckpointRoot [32]byte
	ValidatorIndex primitives.ValidatorIndex
	FeeRecipient   []byte
	GasLimit       uint64
}

// ProposerPreferencesCache stores broadcast proposer preferences indexed by
// proposal slot, looked up within a slot by checkpoint_root.
type ProposerPreferencesCache struct {
	preferences map[primitives.Slot][]ProposerPreference
	lock        sync.RWMutex
}

// NewProposerPreferencesCache initializes a proposer preferences cache.
func NewProposerPreferencesCache() *ProposerPreferencesCache {
	return &ProposerPreferencesCache{
		preferences: make(map[primitives.Slot][]ProposerPreference),
	}
}

// Add stores a proposer preference. If an entry with the same
// (slot, checkpointRoot) already exists, the existing value is kept and false
// is returned.
func (c *ProposerPreferencesCache) Add(
	checkpointRoot [32]byte,
	slot primitives.Slot,
	validatorIndex primitives.ValidatorIndex,
	feeRecipient []byte,
	gasLimit uint64,
) bool {
	c.lock.Lock()
	defer c.lock.Unlock()

	for _, p := range c.preferences[slot] {
		if p.CheckpointRoot == checkpointRoot {
			return false
		}
	}
	c.preferences[slot] = append(c.preferences[slot], ProposerPreference{
		CheckpointRoot: checkpointRoot,
		ValidatorIndex: validatorIndex,
		FeeRecipient:   feeRecipient,
		GasLimit:       gasLimit,
	})
	return true
}

// Get returns the proposer preference stored for (slot, checkpointRoot).
func (c *ProposerPreferencesCache) Get(checkpointRoot [32]byte, slot primitives.Slot) (ProposerPreference, bool) {
	c.lock.RLock()
	defer c.lock.RUnlock()

	for _, p := range c.preferences[slot] {
		if p.CheckpointRoot == checkpointRoot {
			return p, true
		}
	}
	return ProposerPreference{}, false
}

// Has returns true if a proposer preference exists for (slot, checkpointRoot).
func (c *ProposerPreferencesCache) Has(checkpointRoot [32]byte, slot primitives.Slot) bool {
	c.lock.RLock()
	defer c.lock.RUnlock()

	for _, p := range c.preferences[slot] {
		if p.CheckpointRoot == checkpointRoot {
			return true
		}
	}
	return false
}

// PruneBefore removes all proposer preferences for slots before the provided slot.
func (c *ProposerPreferencesCache) PruneBefore(slot primitives.Slot) {
	c.lock.Lock()
	defer c.lock.Unlock()

	for cachedSlot := range c.preferences {
		if cachedSlot < slot {
			delete(c.preferences, cachedSlot)
		}
	}
}

// Clear removes all cached proposer preferences.
func (c *ProposerPreferencesCache) Clear() {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.preferences = make(map[primitives.Slot][]ProposerPreference)
}
