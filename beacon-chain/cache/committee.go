//go:build !fuzz

package cache

import (
	"context"
	"errors"
	"math"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	lruwrpr "github.com/prysmaticlabs/prysm/v5/cache/lru"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/container/slice"
	mathutil "github.com/prysmaticlabs/prysm/v5/math"
	log "github.com/sirupsen/logrus"
)

const (
	// maxCommitteesCacheSize defines the max number of shuffled committees on per randao basis can cache.
	// Due to reorgs and long finality, it's good to keep the old cache around for quickly switch over.
	maxCommitteesCacheSize = int(4)
	// expandedCommitteeCacheSize defines the expanded size of the committee cache in the event we
	// do not have finality to deal with long forks better.
	expandedCommitteeCacheSize = int(32)
)

var (
	// CommitteeCacheMiss tracks the number of committee requests that aren't present in the cache.
	CommitteeCacheMiss = promauto.NewCounter(prometheus.CounterOpts{
		Name: "committee_cache_miss",
		Help: "The number of committee requests that aren't present in the cache.",
	})
	// CommitteeCacheHit tracks the number of committee requests that are in the cache.
	CommitteeCacheHit = promauto.NewCounter(prometheus.CounterOpts{
		Name: "committee_cache_hit",
		Help: "The number of committee requests that are present in the cache.",
	})
)

// CommitteeCache is a struct with 1 queue for looking up shuffled indices list by seed.
type CommitteeCache struct {
	CommitteeCache *lru.Cache
	lock           sync.RWMutex
	inProgress     map[string]bool
	size           int
}

// committeeKeyFn takes the seed as the key to retrieve shuffled indices of a committee in a given epoch.
func committeeKeyFn(obj interface{}) (string, error) {
	info, ok := obj.(*Committees)
	if !ok {
		return "", ErrNotCommittee
	}
	return key(info.Seed), nil
}

// NewCommitteesCache creates a new committee cache for storing/accessing shuffled indices of a committee.
func NewCommitteesCache() *CommitteeCache {
	cc := &CommitteeCache{}
	cc.Clear()
	return cc
}

// Clear resets the CommitteeCache to its initial state
func (c *CommitteeCache) Clear() {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.CommitteeCache = lruwrpr.New(maxCommitteesCacheSize)
	c.inProgress = make(map[string]bool)
	c.size = maxCommitteesCacheSize
}

// ExpandCommitteeCache expands the size of the committee cache.
func (c *CommitteeCache) ExpandCommitteeCache() {
	c.lock.Lock()
	defer c.lock.Unlock()

	if c.size == expandedCommitteeCacheSize {
		return
	}
	c.CommitteeCache.Resize(expandedCommitteeCacheSize)
	c.size = expandedCommitteeCacheSize
	log.Warnf("Expanding committee cache size from %d to %d", maxCommitteesCacheSize, expandedCommitteeCacheSize)
}

// CompressCommitteeCache compresses the size of the committee cache.
func (c *CommitteeCache) CompressCommitteeCache() {
	c.lock.Lock()
	defer c.lock.Unlock()

	if c.size == maxCommitteesCacheSize {
		return
	}
	c.CommitteeCache.Resize(maxCommitteesCacheSize)
	c.size = maxCommitteesCacheSize
	log.Warnf("Reducing committee cache size from %d to %d", expandedCommitteeCacheSize, maxCommitteesCacheSize)
}

// Committee fetches the shuffled indices by slot and committee index. Every list of indices
// represent one committee. Returns true if the list exists with slot and committee index. Otherwise returns false, nil.
func (c *CommitteeCache) Committee(ctx context.Context, slot primitives.Slot, seed [32]byte, index primitives.CommitteeIndex) ([]primitives.ValidatorIndex, error) {
	if err := c.checkInProgress(ctx, seed); err != nil {
		return nil, err
	}

	obj, exists := c.CommitteeCache.Get(key(seed))
	if exists {
		CommitteeCacheHit.Inc()
	} else {
		CommitteeCacheMiss.Inc()
		return nil, nil
	}

	item, ok := obj.(*Committees)
	if !ok {
		return nil, ErrNotCommittee
	}

	committeeCountPerSlot := uint64(1)
	if item.CommitteeCount/uint64(params.BeaconConfig().SlotsPerEpoch) > 1 {
		committeeCountPerSlot = item.CommitteeCount / uint64(params.BeaconConfig().SlotsPerEpoch)
	}

	indexOffSet, err := mathutil.Add64(uint64(index), uint64(slot.ModSlot(params.BeaconConfig().SlotsPerEpoch).Mul(committeeCountPerSlot)))
	if err != nil {
		return nil, err
	}
	start, end := startEndIndices(item, indexOffSet)

	if end > uint64(len(item.ShuffledIndices)) || end < start {
		return nil, errors.New("requested index out of bound")
	}

	return item.ShuffledIndices[start:end], nil
}

// AddCommitteeShuffledList adds Committee shuffled list object to the cache. T
// his method also trims the least recently list if the cache size has ready the max cache size limit.
func (c *CommitteeCache) AddCommitteeShuffledList(ctx context.Context, committees *Committees) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	key, err := committeeKeyFn(committees)
	if err != nil {
		return err
	}
	_ = c.CommitteeCache.Add(key, committees)
	return nil
}

// ActiveIndices returns the active indices of a given seed stored in cache.
func (c *CommitteeCache) ActiveIndices(ctx context.Context, seed [32]byte) ([]primitives.ValidatorIndex, error) {
	if err := c.checkInProgress(ctx, seed); err != nil {
		return nil, err
	}
	obj, exists := c.CommitteeCache.Get(key(seed))

	if exists {
		CommitteeCacheHit.Inc()
	} else {
		CommitteeCacheMiss.Inc()
		return nil, nil
	}

	item, ok := obj.(*Committees)
	if !ok {
		return nil, ErrNotCommittee
	}

	return item.SortedIndices, nil
}

// ActiveIndicesCount returns the active indices count of a given seed stored in cache.
func (c *CommitteeCache) ActiveIndicesCount(ctx context.Context, seed [32]byte) (int, error) {
	if err := c.checkInProgress(ctx, seed); err != nil {
		return 0, err
	}

	obj, exists := c.CommitteeCache.Get(key(seed))
	if exists {
		CommitteeCacheHit.Inc()
	} else {
		CommitteeCacheMiss.Inc()
		return 0, nil
	}

	item, ok := obj.(*Committees)
	if !ok {
		return 0, ErrNotCommittee
	}

	return len(item.SortedIndices), nil
}

// HasEntry returns true if the committee cache has a value.
func (c *CommitteeCache) HasEntry(seed string) bool {
	_, ok := c.CommitteeCache.Get(seed)
	return ok
}

// MarkInProgress a request so that any other similar requests will block on
// Get until MarkNotInProgress is called.
func (c *CommitteeCache) MarkInProgress(seed [32]byte) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	s := key(seed)
	if c.inProgress[s] {
		return ErrAlreadyInProgress
	}
	c.inProgress[s] = true
	return nil
}

// MarkNotInProgress will release the lock on a given request. This should be
// called after put.
func (c *CommitteeCache) MarkNotInProgress(seed [32]byte) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	s := key(seed)
	delete(c.inProgress, s)
	return nil
}

func startEndIndices(c *Committees, index uint64) (uint64, uint64) {
	validatorCount := uint64(len(c.ShuffledIndices))
	start := slice.SplitOffset(validatorCount, c.CommitteeCount, index)
	end := slice.SplitOffset(validatorCount, c.CommitteeCount, index+1)
	return start, end
}

// Using seed as source for key to handle reorgs in the same epoch.
// The seed is derived from state's array of randao mixes and epoch value
// hashed together. This avoids collisions on different validator set. Spec definition:
// https://github.com/ethereum/consensus-specs/blob/v0.9.3/specs/core/0_beacon-chain.md#get_seed
func key(seed [32]byte) string {
	return string(seed[:])
}

func (c *CommitteeCache) checkInProgress(ctx context.Context, seed [32]byte) error {
	delay := minDelay
	// Another identical request may be in progress already. Let's wait until
	// any in progress request resolves or our timeout is exceeded.
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

		// This increasing backoff is to decrease the CPU cycles while waiting
		// for the in progress boolean to flip to false.
		time.Sleep(time.Duration(delay) * time.Nanosecond)
		delay *= delayFactor
		delay = math.Min(delay, maxDelay)
	}
	return nil
}
