package cache

import (
	"sync"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
)

// ProposerPreference is a broadcast preference anchored to a specific branch
// via DependentRoot (Gloas spec).
type ProposerPreference struct {
	DependentRoot  [32]byte
	ProposalSlot   primitives.Slot
	ValidatorIndex primitives.ValidatorIndex
	FeeRecipient   primitives.ExecutionAddress
	TargetGasLimit uint64
	Signature      [fieldparams.BLSSignatureLength]byte
}

// ProposerPreferencesCache stores broadcast proposer preferences indexed by
// proposal slot, looked up within a slot by dependent_root.
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
// (slot, dependentRoot) already exists, the existing value is kept and false
// is returned.
func (c *ProposerPreferencesCache) Add(p ProposerPreference, slot primitives.Slot) bool {
	c.lock.Lock()
	defer c.lock.Unlock()

	for _, existing := range c.preferences[slot] {
		if existing.DependentRoot == p.DependentRoot {
			return false
		}
	}
	c.preferences[slot] = append(c.preferences[slot], p)
	return true
}

// Get returns the proposer preference stored for (slot, dependentRoot).
func (c *ProposerPreferencesCache) Get(dependentRoot [32]byte, slot primitives.Slot) (ProposerPreference, bool) {
	c.lock.RLock()
	defer c.lock.RUnlock()

	for _, p := range c.preferences[slot] {
		if p.DependentRoot == dependentRoot {
			return p, true
		}
	}
	return ProposerPreference{}, false
}

// Has returns true if a proposer preference exists for (slot, dependentRoot).
func (c *ProposerPreferencesCache) Has(dependentRoot [32]byte, slot primitives.Slot) bool {
	c.lock.RLock()
	defer c.lock.RUnlock()

	for _, p := range c.preferences[slot] {
		if p.DependentRoot == dependentRoot {
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

// List returns a flat slice of every cached proposer preference.
func (c *ProposerPreferencesCache) List() []ProposerPreference {
	c.lock.RLock()
	defer c.lock.RUnlock()

	var out []ProposerPreference
	for _, prefs := range c.preferences {
		out = append(out, prefs...)
	}
	return out
}

// Clear removes all cached proposer preferences.
func (c *ProposerPreferencesCache) Clear() {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.preferences = make(map[primitives.Slot][]ProposerPreference)
}
