package cache

import (
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	lruwrpr "github.com/OffchainLabs/prysm/v7/cache/lru"
	"github.com/OffchainLabs/prysm/v7/crypto/hash"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	lru "github.com/hashicorp/golang-lru"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	// defaultCheckpointStateSize defines the default number of checkpoint states to cache
	// during normal finality.
	defaultCheckpointStateSize = 3
	// expandedCheckpointStateSize defines the expanded number of checkpoint states to cache
	// during periods of non-finality (4+ epochs since finality).
	expandedCheckpointStateSize = 10
	// checkpointStateTTL is the time-to-live for checkpoint state cache entries.
	// Attestations reference checkpoints from the current or previous epoch (~13 min),
	// so we use a TTL of 3 epochs to provide a comfortable margin.
	checkpointStateTTL = 3 * 32 * 12 * time.Second // 3 epochs
)

var (
	// Metrics.
	checkpointStateMiss = promauto.NewCounter(prometheus.CounterOpts{
		Name: "check_point_state_cache_miss",
		Help: "The number of check point state requests that aren't present in the cache.",
	})
	checkpointStateHit = promauto.NewCounter(prometheus.CounterOpts{
		Name: "check_point_state_cache_hit",
		Help: "The number of check point state requests that are present in the cache.",
	})
)

// checkpointStateEntry wraps a cached state with the time it was added.
type checkpointStateEntry struct {
	state   state.ReadOnlyBeaconState
	addedAt time.Time
}

// CheckpointStateCache is a struct with 1 queue for looking up state by checkpoint.
type CheckpointStateCache struct {
	cache *lru.Cache
	size  int
}

// NewCheckpointStateCache creates a new checkpoint state cache for storing/accessing processed state.
func NewCheckpointStateCache() *CheckpointStateCache {
	return &CheckpointStateCache{
		cache: lruwrpr.New(defaultCheckpointStateSize),
		size:  defaultCheckpointStateSize,
	}
}

// StateByCheckpoint fetches state by checkpoint. Returns true with a
// reference to the CheckpointState info, if exists. Otherwise returns false, nil.
func (c *CheckpointStateCache) StateByCheckpoint(cp *ethpb.Checkpoint) (state.BeaconState, error) {
	h, err := hash.Proto(cp)
	if err != nil {
		return nil, err
	}

	item, exists := c.cache.Get(h)

	if exists && item != nil {
		checkpointStateHit.Inc()
		// Copy here is unnecessary since the return will only be used to verify attestation signature.
		return item.(*checkpointStateEntry).state.(state.BeaconState), nil
	}

	checkpointStateMiss.Inc()
	return nil, nil
}

// AddCheckpointState adds CheckpointState object to the cache. This method also trims the least
// recently added CheckpointState object if the cache size has ready the max cache size limit.
func (c *CheckpointStateCache) AddCheckpointState(cp *ethpb.Checkpoint, s state.ReadOnlyBeaconState) error {
	c.evictExpired()
	h, err := hash.Proto(cp)
	if err != nil {
		return err
	}
	c.cache.Add(h, &checkpointStateEntry{
		state:   s,
		addedAt: time.Now(),
	})
	return nil
}

// evictExpired removes all cache entries that have exceeded the TTL.
// Keys() returns oldest-to-newest, so we stop at the first non-expired entry.
func (c *CheckpointStateCache) evictExpired() {
	now := time.Now()
	for _, key := range c.cache.Keys() {
		item, ok := c.cache.Peek(key)
		if !ok {
			continue
		}
		if now.Sub(item.(*checkpointStateEntry).addedAt) > checkpointStateTTL {
			c.cache.Remove(key)
		} else {
			break
		}
	}
}

// ExpandCheckpointStateCache expands the checkpoint state cache to the expanded
// size for non-finality periods.
func (c *CheckpointStateCache) ExpandCheckpointStateCache() {
	if c.size == expandedCheckpointStateSize {
		return
	}
	c.cache.Resize(expandedCheckpointStateSize)
	c.size = expandedCheckpointStateSize
	log.Warnf("Expanding checkpoint state cache size from %d to %d", defaultCheckpointStateSize, expandedCheckpointStateSize)
}

// CompressCheckpointStateCache compresses the checkpoint state cache back to
// the default size for normal finality periods.
func (c *CheckpointStateCache) CompressCheckpointStateCache() {
	if c.size == defaultCheckpointStateSize {
		return
	}
	c.cache.Resize(defaultCheckpointStateSize)
	c.size = defaultCheckpointStateSize
	log.Warnf("Compressing checkpoint state cache size from %d to %d", expandedCheckpointStateSize, defaultCheckpointStateSize)
}
