package cache

import (
	"strconv"
	"time"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	gocache "github.com/patrickmn/go-cache"
)

const (
	subscribedValidatorsTTL             = time.Hour
	subscribedValidatorsCleanupInterval = 15 * time.Minute
)

// SubscribedValidatorsCache tracks validator indices the validator client has
// posted committee subscriptions for.
type SubscribedValidatorsCache struct {
	entries *gocache.Cache
}

// NewSubscribedValidatorsCache returns a cache keyed by validator index. The VC
// re-posts subscriptions each epoch, so a TTL of a few epochs is enough to ride
// out a missed post without dropping the validator from custody calculations.
func NewSubscribedValidatorsCache() *SubscribedValidatorsCache {
	return &SubscribedValidatorsCache{
		entries: gocache.New(subscribedValidatorsTTL, subscribedValidatorsCleanupInterval),
	}
}

// Add records the validator as attached. Re-adding extends the TTL.
func (c *SubscribedValidatorsCache) Add(index primitives.ValidatorIndex) {
	c.entries.Set(subscribedKey(index), struct{}{}, gocache.DefaultExpiration)
}

// Has returns true if the validator is currently attached.
func (c *SubscribedValidatorsCache) Has(index primitives.ValidatorIndex) bool {
	_, ok := c.entries.Get(subscribedKey(index))
	return ok
}

// Validating returns true if at least one validator is attached.
func (c *SubscribedValidatorsCache) Validating() bool {
	return c.entries.ItemCount() > 0
}

// Clear removes all attached validators.
func (c *SubscribedValidatorsCache) Clear() {
	c.entries.Flush()
}

// Indices returns the set of currently-attached validator indices.
func (c *SubscribedValidatorsCache) Indices() map[primitives.ValidatorIndex]bool {
	items := c.entries.Items()
	out := make(map[primitives.ValidatorIndex]bool, len(items))
	for key := range items {
		idx, err := strconv.ParseUint(key, 10, 64)
		if err != nil {
			log.WithError(err).Errorf("Failed to parse subscribed validator key: %s", key)
			continue
		}
		out[primitives.ValidatorIndex(idx)] = true
	}
	return out
}

func subscribedKey(index primitives.ValidatorIndex) string {
	return strconv.FormatUint(uint64(index), 10)
}
