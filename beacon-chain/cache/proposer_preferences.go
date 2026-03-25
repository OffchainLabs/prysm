package cache

import (
	"sync"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// ProposerPreference stores the proposer fee recipient and gas limit for a slot.
type ProposerPreference struct {
	FeeRecipient []byte
	GasLimit     uint64
	Signed       *ethpb.SignedProposerPreferences
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
func (c *ProposerPreferencesCache) Add(slot primitives.Slot, signed *ethpb.SignedProposerPreferences) bool {
	c.lock.Lock()
	defer c.lock.Unlock()

	if _, ok := c.slotToPreferences[slot]; ok {
		return false
	}

	c.slotToPreferences[slot] = ProposerPreference{
		FeeRecipient: signed.Message.FeeRecipient,
		GasLimit:     signed.Message.GasLimit,
		Signed:       signed,
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

// Pending returns cached signed proposer preferences not yet included in a
// block. If slot is non-zero, only the entry for that slot is returned.
func (c *ProposerPreferencesCache) Pending(slot primitives.Slot) []*ethpb.SignedProposerPreferences {
	c.lock.RLock()
	defer c.lock.RUnlock()

	if slot != 0 {
		if p, ok := c.slotToPreferences[slot]; ok && p.Signed != nil {
			return []*ethpb.SignedProposerPreferences{p.Signed}
		}
		return nil
	}
	result := make([]*ethpb.SignedProposerPreferences, 0, len(c.slotToPreferences))
	for _, p := range c.slotToPreferences {
		if p.Signed != nil {
			result = append(result, p.Signed)
		}
	}
	return result
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
