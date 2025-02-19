package cache

import (
	"strconv"
	"time"

	"github.com/patrickmn/go-cache"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
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

var (
	// Metrics.
	trackedValidatorsCacheMiss = promauto.NewCounter(prometheus.CounterOpts{
		Name: "tracked_validators_cache_miss",
		Help: "The number of tracked validators requests that are not present in the cache.",
	})

	trackedValidatorsCacheTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "tracked_validators_cache_total",
		Help: "The total number of tracked validators requests in the cache.",
	})

	trackedValidatorsCacheCount = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "tracked_validators_cache_count",
		Help: "The number of tracked validators in the cache.",
	})
)

// NewTrackedValidatorsCache creates a new cache for tracking validators.
func NewTrackedValidatorsCache() *TrackedValidatorsCache {
	return &TrackedValidatorsCache{
		trackedValidators: *cache.New(defaultExpiration, cleanupInterval),
	}
}

// Validator retrieves a tracked validator from the cache (if present).
func (t *TrackedValidatorsCache) Validator(index primitives.ValidatorIndex) (TrackedValidator, bool) {
	trackedValidatorsCacheTotal.Inc()

	key := toCacheKey(index)
	item, ok := t.trackedValidators.Get(key)
	if !ok {
		trackedValidatorsCacheMiss.Inc()
		return TrackedValidator{}, false
	}

	val, ok := item.(TrackedValidator)
	if !ok {
		logrus.Errorf("Failed to cast tracked validator from cache, got unexpected item type %T", item)
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
	trackedValidatorsCacheCount.Set(0)
}

// Validating returns true if there are at least one tracked validators in the cache.
func (t *TrackedValidatorsCache) Validating() bool {
	count := t.trackedValidators.ItemCount()
	trackedValidatorsCacheCount.Set(float64(count))

	return count > 0
}

// ItemCount returns the number of tracked validators in the cache.
func (t *TrackedValidatorsCache) ItemCount() int {
	count := t.trackedValidators.ItemCount()
	trackedValidatorsCacheCount.Set(float64(count))

	return count
}

// Indices returns a map of validator indices that are being tracked.
func (t *TrackedValidatorsCache) Indices() map[primitives.ValidatorIndex]bool {
	items := t.trackedValidators.Items()
	count := len(items)
	trackedValidatorsCacheCount.Set(float64(count))

	indices := make(map[primitives.ValidatorIndex]bool, count)

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
