package cache

import (
	"sync"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
)

// ProposerPreference stores the proposer fee recipient and gas limit for a slot.
type ProposerPreference struct {
	FeeRecipient []byte
	GasLimit     uint64
}

// ProposerPreferencesCache stores proposer preferences by slot.
type ProposerPreferencesCache struct {
	slotToPreferences map[primitives.Slot]ProposerPreference
	lock              sync.RWMutex
}

// NewProposerPreferencesCache initializes a proposer preferences cache.
func NewProposerPreferencesCache() *ProposerPreferencesCache {
	return &ProposerPreferencesCache{
		slotToPreferences: make(map[primitives.Slot]ProposerPreference),
	}
}

// Add stores proposer preferences for a slot. If the slot already exists, the
// existing value is kept and false is returned.
func (c *ProposerPreferencesCache) Add(slot primitives.Slot, feeRecipient []byte, gasLimit uint64) bool {
	c.lock.Lock()
	defer c.lock.Unlock()

	if _, ok := c.slotToPreferences[slot]; ok {
		return false
	}

	// FeeRecipient comes from validated SSZ-decoded proposer preferences, so
	// retaining the slice reference here is intentional.
	c.slotToPreferences[slot] = ProposerPreference{
		FeeRecipient: feeRecipient,
		GasLimit:     gasLimit,
	}
	return true
}

// Get returns proposer preferences for a slot.
func (c *ProposerPreferencesCache) Get(slot primitives.Slot) (ProposerPreference, bool) {
	c.lock.RLock()
	defer c.lock.RUnlock()

	pref, ok := c.slotToPreferences[slot]
	if !ok {
		return ProposerPreference{}, false
	}

	return pref, true
}

// Has returns true if proposer preferences for the slot already exist.
func (c *ProposerPreferencesCache) Has(slot primitives.Slot) bool {
	c.lock.RLock()
	defer c.lock.RUnlock()

	_, ok := c.slotToPreferences[slot]
	return ok
}

// PruneBefore removes all proposer preferences for slots before the provided slot.
func (c *ProposerPreferencesCache) PruneBefore(slot primitives.Slot) {
	c.lock.Lock()
	defer c.lock.Unlock()

	for cachedSlot := range c.slotToPreferences {
		if cachedSlot < slot {
			delete(c.slotToPreferences, cachedSlot)
		}
	}
}

// Clear removes all cached proposer preferences.
func (c *ProposerPreferencesCache) Clear() {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.slotToPreferences = make(map[primitives.Slot]ProposerPreference)
}
