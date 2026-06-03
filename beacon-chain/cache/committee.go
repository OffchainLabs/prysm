//go:build !fuzz

package cache

import (
	"context"
	"errors"
	"sync"

	lruwrpr "github.com/OffchainLabs/prysm/v7/cache/lru"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/container/slice"
	mathutil "github.com/OffchainLabs/prysm/v7/math"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	lru "github.com/hashicorp/golang-lru"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"golang.org/x/sync/singleflight"
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
	Wg             sync.WaitGroup
	Sf             singleflight.Group
	CommitteeCache *lru.Cache
	lock           sync.RWMutex
	size           int
}

// committeeKeyFn takes the seed as the key to retrieve shuffled indices of a committee in a given epoch.
func committeeKeyFn(obj any) (string, error) {
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
	c.Wg.Wait()
	c.lock.Lock()
	defer c.lock.Unlock()
	c.CommitteeCache = lruwrpr.New(maxCommitteesCacheSize)
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
	_, span := trace.StartSpan(ctx, "committeeCache.Committee")
	defer span.End()
	span.SetAttributes(trace.Int64Attribute("slot", int64(slot)), trace.Int64Attribute("index", int64(index))) // lint:ignore uintcast -- OK for tracing.

	obj, exists := c.CommitteeCache.Get(key(seed))
	span.SetAttributes(trace.BoolAttribute("cache_hit", exists))
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
	_, span := trace.StartSpan(ctx, "committeeCache.ActiveIndices")
	defer span.End()

	obj, exists := c.CommitteeCache.Get(key(seed))
	span.SetAttributes(trace.BoolAttribute("cache_hit", exists))
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
func (c *CommitteeCache) ActiveIndicesCount(seed [32]byte) (int, error) {
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
