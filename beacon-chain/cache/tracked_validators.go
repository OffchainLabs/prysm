package cache

import (
	"strconv"
	"time"

	"github.com/patrickmn/go-cache"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/sirupsen/logrus"
)

const (
	defaultExpiration = 1 * time.Hour
	cleanupInterval   = 15 * time.Minute
)

type (
	TrackedValidator struct {
		Active       bool
		FeeRecipient primitives.ExecutionAddress
		Index        primitives.ValidatorIndex
	}

	TrackedValidatorsCache struct {
		trackedValidators cache.Cache
	}
)

// NewTrackedValidatorsCache creates a new cache for tracking validators.
func NewTrackedValidatorsCache() *TrackedValidatorsCache {
	return &TrackedValidatorsCache{
		trackedValidators: *cache.New(defaultExpiration, cleanupInterval),
	}
}

// Validator retrieves a tracked validator from the cache (if present).
func (t *TrackedValidatorsCache) Validator(index primitives.ValidatorIndex) (TrackedValidator, bool) {
	key := toCacheKey(index)
	item, ok := t.trackedValidators.Get(key)
	if !ok {
		return TrackedValidator{}, false
	}

	val, ok := item.(TrackedValidator)
	if !ok {
		logrus.Error("Failed to cast tracked validator from cache")
		return TrackedValidator{}, false
	}

	return val, true
}

// Set adds a tracked validator to the cache.
func (t *TrackedValidatorsCache) Set(val TrackedValidator) {
	key := toCacheKey(val.Index)
	t.trackedValidators.Set(key, val, cache.DefaultExpiration)
}

// Delete removes a tracked validator from the cache.
func (t *TrackedValidatorsCache) Prune() {
	t.trackedValidators.Flush()
}

// Validating returns true if there are at least one tracked validators in the cache.
func (t *TrackedValidatorsCache) Validating() bool {
	return t.trackedValidators.ItemCount() > 0
}

// ItemCount returns the number of tracked validators in the cache.
func (t *TrackedValidatorsCache) ItemCount() int {
	return t.trackedValidators.ItemCount()
}

// Indices returns a map of validator indices that are being tracked.
func (t *TrackedValidatorsCache) Indices() map[primitives.ValidatorIndex]bool {
	items := t.trackedValidators.Items()

	indices := make(map[primitives.ValidatorIndex]bool, len(items))

	for cacheKey := range items {
		index, err := fromCacheKey(cacheKey)
		if err != nil {
			logrus.WithError(err).Error("Failed to get validator index from cache key")
			continue
		}

		indices[index] = true
	}

	return indices
}

// toCacheKey creates a cache key from the validator index.
func toCacheKey(validatorIndex primitives.ValidatorIndex) string {
	return strconv.FormatUint(uint64(validatorIndex), 10)
}

// fromCacheKey gets the validator index from the cache key.
func fromCacheKey(key string) (primitives.ValidatorIndex, error) {
	validatorIndex, err := strconv.ParseUint(key, 10, 64)
	if err != nil {
		return 0, errors.Wrapf(err, "parse Uint: %s", key)
	}

	return primitives.ValidatorIndex(validatorIndex), nil
}
