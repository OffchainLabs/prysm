//go:build !fuzz

package cache

import (
	"context"
	"sync"
	"time"

	lruwrpr "github.com/OffchainLabs/prysm/v7/cache/lru"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	lru "github.com/hashicorp/golang-lru"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	// maxPayloadCommitteeCacheSize is the max number of payload committee entries to cache.
	// 64 covers two full epochs of slots.
	maxPayloadCommitteeCacheSize = 64
)

var (
	// PayloadCommitteeCacheMiss tracks the number of payload committee requests that aren't present in the cache.
	PayloadCommitteeCacheMiss = promauto.NewCounter(prometheus.CounterOpts{
		Name: "payload_committee_cache_miss",
		Help: "The number of payload committee requests that aren't present in the cache.",
	})
	// PayloadCommitteeCacheHit tracks the number of payload committee requests that are in the cache.
	PayloadCommitteeCacheHit = promauto.NewCounter(prometheus.CounterOpts{
		Name: "payload_committee_cache_hit",
		Help: "The number of payload committee requests that are present in the cache.",
	})
)

// PayloadCommitteeCache is an LRU cache for payload timeliness committee results keyed by ptcSeed.
type PayloadCommitteeCache struct {
	cache      *lru.Cache
	lock       sync.RWMutex
	inProgress map[string]bool
}

// NewPayloadCommitteeCache creates a new cache for storing payload committee results.
func NewPayloadCommitteeCache() *PayloadCommitteeCache {
	c := &PayloadCommitteeCache{}
	c.Clear()
	return c
}

// Get returns the cached payload committee for the given seed. Returns nil on cache miss.
// Blocks if another goroutine is computing the same seed.
func (c *PayloadCommitteeCache) Get(ctx context.Context, seed [32]byte) ([]primitives.ValidatorIndex, error) {
	if err := c.checkInProgress(ctx, seed); err != nil {
		return nil, err
	}

	obj, exists := c.cache.Get(key(seed))
	if exists {
		PayloadCommitteeCacheHit.Inc()
	} else {
		PayloadCommitteeCacheMiss.Inc()
		return nil, nil
	}

	indices, ok := obj.([]primitives.ValidatorIndex)
	if !ok {
		return nil, ErrIncorrectType
	}

	return indices, nil
}

// Add stores a payload committee result in the cache.
func (c *PayloadCommitteeCache) Add(seed [32]byte, indices []primitives.ValidatorIndex) {
	c.cache.Add(key(seed), indices)
}

// MarkInProgress marks a seed as being computed. Returns ErrAlreadyInProgress if another
// goroutine is already computing it.
func (c *PayloadCommitteeCache) MarkInProgress(seed [32]byte) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	s := key(seed)
	if c.inProgress[s] {
		return ErrAlreadyInProgress
	}
	c.inProgress[s] = true
	return nil
}

// MarkNotInProgress releases the in-progress lock on a given seed.
func (c *PayloadCommitteeCache) MarkNotInProgress(seed [32]byte) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	s := key(seed)
	delete(c.inProgress, s)
	return nil
}

// Clear resets the cache to its initial state.
func (c *PayloadCommitteeCache) Clear() {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.cache = lruwrpr.New(maxPayloadCommitteeCacheSize)
	c.inProgress = make(map[string]bool)
}

func (c *PayloadCommitteeCache) checkInProgress(ctx context.Context, seed [32]byte) error {
	delay := minDelay
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		c.lock.RLock()
		if !c.inProgress[key(seed)] {
			c.lock.RUnlock()
			break
		}
		c.lock.RUnlock()

		time.Sleep(time.Duration(delay) * time.Nanosecond)
		delay *= delayFactor
		delay = min(delay, maxDelay)
	}
	return nil
}
