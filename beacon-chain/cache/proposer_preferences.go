package cache

import (
	"strconv"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	gocache "github.com/patrickmn/go-cache"
)

const (
	defaultExpiration = 1 * time.Hour
	cleanupInterval   = 15 * time.Minute
)

// ProposerPreference is a proposer fee-recipient / gas-limit preference. When
// stored in the (slot, dep_root) preferences map it represents a signed
// SignedProposerPreferences; when stored in the per-validator defaults map it
// represents a pre-Gloas PrepareBeaconProposer write (no signature, no
// dependent_root).
type ProposerPreference struct {
	DependentRoot  [32]byte
	ValidatorIndex primitives.ValidatorIndex
	FeeRecipient   primitives.ExecutionAddress
	GasLimit       uint64
}

// ProposerPreferencesCache holds two stores with different lookup keys:
//
//  1. preferences: signed proposer preferences from gossip / our local
//     SubmitSignedProposerPreferences, keyed by (slot, dependent_root).
//     Spec-aligned lookup for bid validation and post-Gloas proposing.
//
//  2. defaults: per-validator fee-recipient defaults written via the
//     pre-Gloas PrepareBeaconProposer endpoint, keyed by validator_index.
//     Branch-independent fallback for proposing when no (slot, dep_root)
//     entry exists.
//
// Lookup order at proposal time: preferences → defaults → DefaultFeeRecipient
// (the --suggested-fee-recipient flag). "Which validators are attached to this
// BN" lives in SubscribedValidatorsCache, not here.
type ProposerPreferencesCache struct {
	preferences map[primitives.Slot][]ProposerPreference
	defaults    *gocache.Cache
	lock        sync.RWMutex
}

// NewProposerPreferencesCache initializes a proposer preferences cache.
func NewProposerPreferencesCache() *ProposerPreferencesCache {
	return &ProposerPreferencesCache{
		preferences: make(map[primitives.Slot][]ProposerPreference),
		defaults:    gocache.New(defaultExpiration, cleanupInterval),
	}
}

// Add stores a signed proposer preference at the given slot. If an entry
// with the same (slot, pref.DependentRoot) already exists, the existing value
// is kept and false is returned.
func (c *ProposerPreferencesCache) Add(pref ProposerPreference, slot primitives.Slot) bool {
	c.lock.Lock()
	defer c.lock.Unlock()

	for _, p := range c.preferences[slot] {
		if p.DependentRoot == pref.DependentRoot {
			return false
		}
	}
	c.preferences[slot] = append(c.preferences[slot], pref)
	return true
}

// BestFor returns the best-available preference for proposer `idx` at
// (slot, dependentRoot): the signed branch-specific entry if present, else
// the per-validator default, else (zero, false).
func (c *ProposerPreferencesCache) BestFor(dependentRoot [32]byte, slot primitives.Slot, idx primitives.ValidatorIndex) (ProposerPreference, bool) {
	if pref, ok := c.Get(dependentRoot, slot); ok && pref.ValidatorIndex == idx {
		return pref, true
	}
	if def, ok := c.Default(idx); ok {
		return def, true
	}
	return ProposerPreference{}, false
}

// Get returns the signed proposer preference stored for (slot, dependentRoot).
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

// Has returns true if a signed preference exists for (slot, dependentRoot).
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

// PruneBefore removes all signed preferences for slots before the provided slot.
func (c *ProposerPreferencesCache) PruneBefore(slot primitives.Slot) {
	c.lock.Lock()
	defer c.lock.Unlock()

	for cachedSlot := range c.preferences {
		if cachedSlot < slot {
			delete(c.preferences, cachedSlot)
		}
	}
}

// Clear removes all cached signed preferences. Does not touch defaults.
func (c *ProposerPreferencesCache) Clear() {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.preferences = make(map[primitives.Slot][]ProposerPreference)
}

// Set records a per-validator fee-recipient default, keyed by ValidatorIndex.
// Populated by the pre-Gloas PrepareBeaconProposer endpoint. DependentRoot on
// the supplied preference is ignored.
func (c *ProposerPreferencesCache) Set(pref ProposerPreference) {
	c.defaults.Set(defaultKey(pref.ValidatorIndex), pref, gocache.DefaultExpiration)
}

// Default returns the per-validator fee-recipient default for the given
// validator index, if one was set via PrepareBeaconProposer.
func (c *ProposerPreferencesCache) Default(index primitives.ValidatorIndex) (ProposerPreference, bool) {
	item, ok := c.defaults.Get(defaultKey(index))
	if !ok {
		return ProposerPreference{}, false
	}
	pref, ok := item.(ProposerPreference)
	if !ok {
		log.Errorf("Failed to cast default fee recipient from cache, got unexpected item type %T", item)
		return ProposerPreference{}, false
	}
	return pref, true
}

func defaultKey(index primitives.ValidatorIndex) string {
	return strconv.FormatUint(uint64(index), 10)
}
