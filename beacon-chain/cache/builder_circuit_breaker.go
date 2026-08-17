package cache

import (
	"sync"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
)

// builderFailure tracks consecutive payload delivery failures for a single builder index.
type builderFailure struct {
	failed              uint64
	blacklistUntilEpoch primitives.Epoch
	backOffEpoch        primitives.Epoch // epoch at which `failed` resets to zero
}

// BuilderCircuitBreaker tracks builders that won an auction but failed to reveal the payload,
// and blacklists them so their bids are neither propagated nor used for block production.
//
// It is written by the blockchain service on block import and read by the sync and validator
// services, so all methods take the epoch to evaluate against rather than depending on a clock.
type BuilderCircuitBreaker struct {
	lock     sync.RWMutex
	failures map[primitives.BuilderIndex]*builderFailure
	recorded map[[32]byte]primitives.Epoch // parent roots whose failure was already recorded, and when
}

func NewBuilderCircuitBreaker() *BuilderCircuitBreaker {
	return &BuilderCircuitBreaker{
		failures: make(map[primitives.BuilderIndex]*builderFailure),
		recorded: make(map[[32]byte]primitives.Epoch),
	}
}

// RecordFailure records a missing payload for parentRoot against the given builder and reports
// whether the builder is blacklisted as a result. Repeated calls for the same parentRoot are ignored, since
// several children can build on the same empty parent.
func (c *BuilderCircuitBreaker) RecordFailure(
	idx primitives.BuilderIndex,
	parentRoot [32]byte,
	epoch primitives.Epoch,
) bool {
	if c == nil {
		return false
	}
	c.lock.Lock()
	defer c.lock.Unlock()

	if _, ok := c.recorded[parentRoot]; ok {
		return false
	}
	c.recorded[parentRoot] = epoch

	cfg := params.BeaconConfig()
	f, ok := c.failures[idx]
	if !ok {
		f = &builderFailure{}
		c.failures[idx] = f
	} else if epoch >= f.backOffEpoch {
		f.failed = 0
	}
	f.failed++
	f.backOffEpoch = epoch + cfg.BuilderFailureBackOffPeriod

	if f.failed <= cfg.BuilderAllowedFailures {
		return c.blacklistedAtEpoch(idx, epoch)
	}
	period := cfg.BuilderBlacklistPeriod
	if f.failed >= cfg.BuilderCriticalFailures {
		period = cfg.BuilderCriticalBlacklistPeriod
	}
	if until := epoch + period; until > f.blacklistUntilEpoch {
		f.blacklistUntilEpoch = until
	}
	return true
}

// RecordSuccess clears a builder's failure record after it reveals a valid payload.
func (c *BuilderCircuitBreaker) RecordSuccess(idx primitives.BuilderIndex) {
	if c == nil {
		return
	}
	c.lock.Lock()
	defer c.lock.Unlock()
	delete(c.failures, idx)
}

// Blacklisted reports whether the builder's bids must be ignored at the given epoch.
func (c *BuilderCircuitBreaker) Blacklisted(idx primitives.BuilderIndex, epoch primitives.Epoch) bool {
	if c == nil {
		return false
	}
	c.lock.RLock()
	defer c.lock.RUnlock()
	return c.blacklistedAtEpoch(idx, epoch)
}

// SelfBuildOnly reports whether enough builders are concurrently blacklisted that the node should
// stop taking foreign bids altogether.
func (c *BuilderCircuitBreaker) SelfBuildOnly(epoch primitives.Epoch) bool {
	if c == nil {
		return false
	}
	c.lock.RLock()
	defer c.lock.RUnlock()
	return c.blacklistedCount(epoch) >= params.BeaconConfig().BuilderCriticalFailedBuilders
}

// BlacklistedCount returns the number of builders currently blacklisted by failure tracking.
func (c *BuilderCircuitBreaker) BlacklistedCount(epoch primitives.Epoch) uint64 {
	if c == nil {
		return 0
	}
	c.lock.RLock()
	defer c.lock.RUnlock()
	return c.blacklistedCount(epoch)
}

// DropInactiveBuilders clears the records of builders that can no longer bid, so that a recycled
// index does not punish its newcomer. Unresolvable indices keep their record.
func (c *BuilderCircuitBreaker) DropInactiveBuilders(isActive func(primitives.BuilderIndex) (bool, error)) {
	if c == nil {
		return
	}
	c.lock.Lock()
	defer c.lock.Unlock()

	for idx := range c.failures {
		active, err := isActive(idx)
		if err != nil {
			continue
		}
		if !active {
			delete(c.failures, idx)
		}
	}
}

// Prune drops records whose blacklist and back off periods have both elapsed, along with recorded
// roots that can no longer be revisited.
func (c *BuilderCircuitBreaker) Prune(epoch primitives.Epoch) {
	if c == nil {
		return
	}
	c.lock.Lock()
	defer c.lock.Unlock()

	for idx, f := range c.failures {
		if f.blacklistUntilEpoch <= epoch && epoch >= f.backOffEpoch {
			delete(c.failures, idx)
		}
	}
	for root, at := range c.recorded {
		if epoch > at+1 {
			delete(c.recorded, root)
		}
	}
}

// blacklistedCount requires the caller to hold the lock.
func (c *BuilderCircuitBreaker) blacklistedCount(epoch primitives.Epoch) uint64 {
	var count uint64
	for _, f := range c.failures {
		if f.blacklistUntilEpoch > epoch {
			count++
		}
	}
	return count
}

// blacklistedAtEpoch requires the caller to hold the lock.
func (c *BuilderCircuitBreaker) blacklistedAtEpoch(idx primitives.BuilderIndex, epoch primitives.Epoch) bool {
	f, ok := c.failures[idx]
	return ok && f.blacklistUntilEpoch > epoch
}
