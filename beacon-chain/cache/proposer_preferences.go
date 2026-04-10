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

// ProposerPreferencesCache stores proposer preferences by slot and validator index,
// making it safe under chain reorgs where the proposer for a slot may change.
type ProposerPreferencesCache struct {
	preferences map[primitives.Slot]map[primitives.ValidatorIndex]ProposerPreference
	lock        sync.RWMutex
}

// NewProposerPreferencesCache initializes a proposer preferences cache.
func NewProposerPreferencesCache() *ProposerPreferencesCache {
	return &ProposerPreferencesCache{
		preferences: make(map[primitives.Slot]map[primitives.ValidatorIndex]ProposerPreference),
	}
}

// Add stores proposer preferences for a slot and validator index. If the
// (slot, validatorIndex) pair already exists, the existing value is kept and
// false is returned.
func (c *ProposerPreferencesCache) Add(slot primitives.Slot, validatorIndex primitives.ValidatorIndex, feeRecipient []byte, gasLimit uint64) bool {
	c.lock.Lock()
	defer c.lock.Unlock()

	validators, ok := c.preferences[slot]
	if ok {
		if _, exists := validators[validatorIndex]; exists {
			return false
		}
		existing := make([]primitives.ValidatorIndex, 0, len(validators))
		for idx := range validators {
			existing = append(existing, idx)
		}
		log.WithField("slot", slot).WithField("newValidator", validatorIndex).WithField("existingValidators", existing).
			Debug("New proposer preference for slot that already has a different validator (possible reorg)")
	} else {
		validators = make(map[primitives.ValidatorIndex]ProposerPreference)
		c.preferences[slot] = validators
	}

	// FeeRecipient comes from validated SSZ-decoded proposer preferences, so
	// retaining the slice reference here is intentional.
	validators[validatorIndex] = ProposerPreference{
		FeeRecipient: feeRecipient,
		GasLimit:     gasLimit,
	}
	return true
}

// Get returns proposer preferences for a slot and validator index.
func (c *ProposerPreferencesCache) Get(slot primitives.Slot, validatorIndex primitives.ValidatorIndex) (ProposerPreference, bool) {
	c.lock.RLock()
	defer c.lock.RUnlock()

	validators, ok := c.preferences[slot]
	if !ok {
		return ProposerPreference{}, false
	}
	pref, ok := validators[validatorIndex]
	if !ok {
		return ProposerPreference{}, false
	}
	return pref, true
}

// Has returns true if proposer preferences for the slot and validator index already exist.
func (c *ProposerPreferencesCache) Has(slot primitives.Slot, validatorIndex primitives.ValidatorIndex) bool {
	c.lock.RLock()
	defer c.lock.RUnlock()

	validators, ok := c.preferences[slot]
	if !ok {
		return false
	}
	_, exists := validators[validatorIndex]
	return exists
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

	c.preferences = make(map[primitives.Slot]map[primitives.ValidatorIndex]ProposerPreference)
}
