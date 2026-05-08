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

// ProposerPreference is the unified preference shape stored in both stores
// of ProposerPreferencesCache. For foreign entries it carries the gossiped
// branch anchor (DependentRoot). For owned entries written via
// SubmitSignedProposerPreferences, DependentRoot reflects the latest
// local Submit; for entries written via the pre-Gloas
// prepare_beacon_proposer endpoint, DependentRoot is zero (the concept
// does not apply pre-Gloas).
type ProposerPreference struct {
	DependentRoot  [32]byte
	ValidatorIndex primitives.ValidatorIndex
	FeeRecipient   []byte
	GasLimit       uint64
}

// ProposerPreferencesCache holds two semantically distinct stores with no
// data overlap:
//
//  1. external: foreign proposer preferences received via gossip, keyed by
//     (slot, dependent_root). Branch-specific per spec. Used by bid
//     validation to verify that a foreign builder bid matches what the
//     foreign proposer announced. The gossip validator filters
//     self-broadcasts, so our own preferences never land here.
//
//  2. owned: validators this BN's VC manages, keyed by validator_index.
//     Branch-independent — fee recipient / gas limit are properties of the
//     validator, not of any specific (slot, dependent_root). The reason
//     external is keyed by branch is that on different branches the
//     proposing validator may differ; for our own validators that question
//     does not apply because we propose with our own preferences regardless
//     of branch. Populated by prepare_beacon_proposer (pre-Gloas) and
//     SubmitSignedProposerPreferences (post-Gloas). Entries TTL out.
//
// Validating()/Indices()/the trackedProposer ownership check read from
// owned; bid validation reads from external.
type ProposerPreferencesCache struct {
	external map[primitives.Slot][]ProposerPreference
	owned    *gocache.Cache
	lock     sync.RWMutex
}

// NewProposerPreferencesCache initializes a proposer preferences cache.
func NewProposerPreferencesCache() *ProposerPreferencesCache {
	return &ProposerPreferencesCache{
		external: make(map[primitives.Slot][]ProposerPreference),
		owned:    gocache.New(defaultExpiration, cleanupInterval),
	}
}

// Add stores a foreign (gossip-ingested) proposer preference. If an entry
// with the same (slot, dependentRoot) already exists, the existing value
// is kept and false is returned. If the validator is locally owned, the
// preference is rejected — our own vidxs must never appear in external.
// Use Set for our own validators — SubmitSignedProposerPreferences
// populates owned, not external.
func (c *ProposerPreferencesCache) Add(
	dependentRoot [32]byte,
	slot primitives.Slot,
	validatorIndex primitives.ValidatorIndex,
	feeRecipient []byte,
	gasLimit uint64,
) bool {
	if _, owned := c.owned.Get(ownedKey(validatorIndex)); owned {
		return false
	}
	c.lock.Lock()
	defer c.lock.Unlock()

	for _, p := range c.external[slot] {
		if p.DependentRoot == dependentRoot {
			return false
		}
	}
	c.external[slot] = append(c.external[slot], ProposerPreference{
		DependentRoot:  dependentRoot,
		ValidatorIndex: validatorIndex,
		FeeRecipient:   feeRecipient,
		GasLimit:       gasLimit,
	})
	return true
}

// Get returns the foreign proposer preference stored for (slot, dependentRoot).
func (c *ProposerPreferencesCache) Get(dependentRoot [32]byte, slot primitives.Slot) (ProposerPreference, bool) {
	c.lock.RLock()
	defer c.lock.RUnlock()

	for _, p := range c.external[slot] {
		if p.DependentRoot == dependentRoot {
			return p, true
		}
	}
	return ProposerPreference{}, false
}

// Has returns true if a foreign proposer preference exists for (slot, dependentRoot).
func (c *ProposerPreferencesCache) Has(dependentRoot [32]byte, slot primitives.Slot) bool {
	c.lock.RLock()
	defer c.lock.RUnlock()

	for _, p := range c.external[slot] {
		if p.DependentRoot == dependentRoot {
			return true
		}
	}
	return false
}

// PruneBefore removes all foreign preferences for slots before the provided slot.
func (c *ProposerPreferencesCache) PruneBefore(slot primitives.Slot) {
	c.lock.Lock()
	defer c.lock.Unlock()

	for cachedSlot := range c.external {
		if cachedSlot < slot {
			delete(c.external, cachedSlot)
		}
	}
}

// Clear removes all cached foreign preferences.
func (c *ProposerPreferencesCache) Clear() {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.external = make(map[primitives.Slot][]ProposerPreference)
}

// Set records a validator that this BN's VC owns, keyed by ValidatorIndex.
// Pre-Gloas callers (prepare_beacon_proposer) leave DependentRoot zero;
// post-Gloas callers (SubmitSignedProposerPreferences) populate it from
// the local Submit. The fee recipient / gas limit are treated as
// branch-independent for our own validators. Foreign gossip ingestion
// must not call this.
func (c *ProposerPreferencesCache) Set(pref ProposerPreference) {
	c.owned.Set(ownedKey(pref.ValidatorIndex), pref, gocache.DefaultExpiration)
}

// Validator retrieves an owned validator entry by index, if present. A hit
// indicates the BN's VC owns the validator and carries the branch-independent
// fee recipient / gas limit.
func (c *ProposerPreferencesCache) Validator(index primitives.ValidatorIndex) (ProposerPreference, bool) {
	item, ok := c.owned.Get(ownedKey(index))
	if !ok {
		return ProposerPreference{}, false
	}
	pref, ok := item.(ProposerPreference)
	if !ok {
		log.Errorf("Failed to cast owned validator from cache, got unexpected item type %T", item)
		return ProposerPreference{}, false
	}
	return pref, true
}

// Validating returns true if this BN's VC owns at least one validator.
// Foreign gossiped preferences do not count.
func (c *ProposerPreferencesCache) Validating() bool {
	return c.owned.ItemCount() > 0
}

// Indices returns the set of validator indices owned by this BN's VC.
// Foreign gossiped preferences are excluded.
func (c *ProposerPreferencesCache) Indices() map[primitives.ValidatorIndex]bool {
	items := c.owned.Items()
	indices := make(map[primitives.ValidatorIndex]bool, len(items))
	for key := range items {
		idx, err := strconv.ParseUint(key, 10, 64)
		if err != nil {
			log.WithError(err).Errorf("Failed to parse owned validator key: %s", key)
			continue
		}
		indices[primitives.ValidatorIndex(idx)] = true
	}
	return indices
}

func ownedKey(index primitives.ValidatorIndex) string {
	return strconv.FormatUint(uint64(index), 10)
}
