package cache

import (
	"fmt"
	"net"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
)

const maxRelayKeyLen = 256

// builderFailure tracks consecutive payload delivery failures for a single builder index.
//
// A record exists only for a builder charged directly. Collateral relay bans never create one,
// which keeps blacklistedCount, and therefore SelfBuildOnly, counting direct offenses only.
type builderFailure struct {
	failed              uint64
	blacklistUntilEpoch primitives.Epoch
	backOffEpoch        primitives.Epoch // epoch at which `failed` resets to zero
}

// relayRecord is what this node learned about one direct connection endpoint.
type relayRecord struct {
	members       map[primitives.BuilderIndex]primitives.Epoch
	bannedUntil   primitives.Epoch
	bannedBy      primitives.BuilderIndex
	lastSeenEpoch primitives.Epoch
}

// FailureOutcome reports what a charged failure did, so the caller can log the relay fallout
// without a second, racy lock acquisition.
type FailureOutcome struct {
	Blacklisted  bool
	BannedRelays []string
	Collateral   int
}

// BuilderCircuitBreaker tracks builders that won an auction but failed to reveal the payload,
// and blacklists them so their bids are neither propagated nor used for block production.
//
// It also tracks which builder indices each direct connection endpoint has served, so that a
// failing builder bans every endpoint serving it and, through them, that endpoint's other builders.
//
// It is written by the blockchain service on block import and read by the sync and validator
// services, so all methods take the epoch to evaluate against rather than depending on a clock.
type BuilderCircuitBreaker struct {
	lock     sync.RWMutex
	failures map[primitives.BuilderIndex]*builderFailure
	recorded map[[32]byte]primitives.Epoch // parent roots whose failure was already recorded, and when

	relays        map[string]*relayRecord
	relaysByIndex map[primitives.BuilderIndex][]string

	lastRelaySweepEpoch primitives.Epoch
	relaySwept          bool
}

func NewBuilderCircuitBreaker() *BuilderCircuitBreaker {
	return &BuilderCircuitBreaker{
		failures:      make(map[primitives.BuilderIndex]*builderFailure),
		recorded:      make(map[[32]byte]primitives.Epoch),
		relays:        make(map[string]*relayRecord),
		relaysByIndex: make(map[primitives.BuilderIndex][]string),
	}
}

// RecordFailure records a missing payload for parentRoot against the given builder and reports
// whether the builder is blacklisted as a result, along with the endpoints banned for serving it.
// Repeated calls for the same parentRoot are ignored, since several children can build on the same
// empty parent.
func (c *BuilderCircuitBreaker) RecordFailure(
	idx primitives.BuilderIndex,
	parentRoot [32]byte,
	epoch primitives.Epoch,
) FailureOutcome {
	if c == nil {
		return FailureOutcome{}
	}
	c.lock.Lock()
	defer c.lock.Unlock()

	if _, ok := c.recorded[parentRoot]; ok {
		return FailureOutcome{}
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
		return FailureOutcome{Blacklisted: c.directlyBlacklisted(idx, epoch)}
	}
	period := cfg.BuilderBlacklistPeriod
	if f.failed >= cfg.BuilderCriticalFailures {
		period = cfg.BuilderCriticalBlacklistPeriod
	}
	if until := epoch + period; until > f.blacklistUntilEpoch {
		f.blacklistUntilEpoch = until
	}

	// Capping the relay ban at the offender's own expiry keeps its record alive for at least as
	// long as the ban, so a release is always still possible.
	until := f.blacklistUntilEpoch
	if capped := epoch + cfg.BuilderRelayBlacklistPeriod; capped < until {
		until = capped
	}
	banned, collateral := c.banRelaysServing(idx, until)
	return FailureOutcome{Blacklisted: true, BannedRelays: banned, Collateral: collateral}
}

// Propagation stops here by construction: only a directly charged index reaches this function, so
// a collaterally banned member never bans the endpoints serving it.
func (c *BuilderCircuitBreaker) banRelaysServing(idx primitives.BuilderIndex, until primitives.Epoch) ([]string, int) {
	endpoints := c.relaysByIndex[idx]
	if len(endpoints) == 0 {
		return nil, 0
	}
	banned := make([]string, 0, len(endpoints))
	collateral := make(map[primitives.BuilderIndex]struct{})
	for _, ep := range endpoints {
		r, ok := c.relays[ep]
		if !ok {
			continue
		}
		if until > r.bannedUntil {
			r.bannedUntil = until
		}
		r.bannedBy = idx
		banned = append(banned, ep)
		for m := range r.members {
			if m != idx {
				collateral[m] = struct{}{}
			}
		}
	}
	return banned, len(collateral)
}

// RecordSuccess clears a builder's failure record after it reveals a valid payload. A builder that
// belongs to a tracked endpoint keeps a zeroed record instead, since that record is the evidence a
// relay release requires, and lifts the ban of every endpoint it serves that has no failing member
// left.
func (c *BuilderCircuitBreaker) RecordSuccess(idx primitives.BuilderIndex) {
	if c == nil {
		return
	}
	c.lock.Lock()
	defer c.lock.Unlock()

	f, ok := c.failures[idx]
	if !ok {
		return
	}
	if len(c.relaysByIndex[idx]) == 0 {
		delete(c.failures, idx)
		return
	}
	f.failed = 0
	f.blacklistUntilEpoch = 0
	for _, ep := range c.relaysByIndex[idx] {
		r, ok := c.relays[ep]
		if !ok || r.bannedUntil == 0 || c.relayHasFailingMember(r) {
			continue
		}
		r.bannedUntil, r.bannedBy = 0, 0
		for m := range r.members {
			if mf, ok := c.failures[m]; ok {
				mf.blacklistUntilEpoch = 0
			}
		}
	}
}

func (c *BuilderCircuitBreaker) relayHasFailingMember(r *relayRecord) bool {
	for m := range r.members {
		if f, ok := c.failures[m]; ok && f.failed > 0 {
			return true
		}
	}
	return false
}

// ObserveRelayBid records that endpoint served a bid from idx.
//
// It must only be called once the bid's signature has been verified, otherwise an endpoint could
// claim indices it does not serve and weaponize the collateral ban against them.
func (c *BuilderCircuitBreaker) ObserveRelayBid(endpoint string, idx primitives.BuilderIndex, epoch primitives.Epoch) {
	if c == nil || idx == params.BeaconConfig().BuilderIndexSelfBuild {
		return
	}
	if features.Get().DisableBuilderRelayCircuitBreaker {
		return
	}
	key, ok := normalizeRelay(endpoint)
	if !ok {
		return
	}
	cfg := params.BeaconConfig()

	c.lock.Lock()
	defer c.lock.Unlock()

	r, ok := c.relays[key]
	if !ok {
		if uint64(len(c.relays)) >= cfg.BuilderMaxTrackedRelays && !c.evictRelay(epoch) {
			return
		}
		r = &relayRecord{members: make(map[primitives.BuilderIndex]primitives.Epoch)}
		c.relays[key] = r
	}
	if epoch > r.lastSeenEpoch {
		r.lastSeenEpoch = epoch
	}
	if _, member := r.members[idx]; !member {
		if uint64(len(r.members)) >= cfg.BuilderMaxIndicesPerRelay && !c.evictMember(key, r) {
			return
		}
	}
	c.linkRelay(key, r, idx, epoch)
}

// RelayBanned reports whether this endpoint must not be contacted at the given epoch.
func (c *BuilderCircuitBreaker) RelayBanned(endpoint string, epoch primitives.Epoch) bool {
	if c == nil {
		return false
	}
	key, ok := normalizeRelay(endpoint)
	if !ok {
		return false
	}
	c.lock.RLock()
	defer c.lock.RUnlock()
	r, ok := c.relays[key]
	return ok && r.bannedUntil > epoch
}

// Blacklisted reports whether the builder's bids must be ignored at the given epoch, either by its
// own failures or because an endpoint serving it is banned.
func (c *BuilderCircuitBreaker) Blacklisted(idx primitives.BuilderIndex, epoch primitives.Epoch) bool {
	if c == nil {
		return false
	}
	c.lock.RLock()
	defer c.lock.RUnlock()
	return c.directlyBlacklisted(idx, epoch) || c.relayBannedFor(idx, epoch)
}

// SelfBuildOnly reports whether enough builders are concurrently blacklisted by their own failures
// that the node should stop taking foreign bids altogether. Collateral bans do not count, or a
// single endpoint serving many builders would trip the fallback on its first failure.
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

// RelayBannedCount returns the number of direct connection endpoints currently banned.
func (c *BuilderCircuitBreaker) RelayBannedCount(epoch primitives.Epoch) uint64 {
	if c == nil {
		return 0
	}
	c.lock.RLock()
	defer c.lock.RUnlock()
	var count uint64
	for _, r := range c.relays {
		if r.bannedUntil > epoch {
			count++
		}
	}
	return count
}

// CollateralBlacklistedCount returns the builders blacklisted only through a banned endpoint.
func (c *BuilderCircuitBreaker) CollateralBlacklistedCount(epoch primitives.Epoch) uint64 {
	if c == nil {
		return 0
	}
	c.lock.RLock()
	defer c.lock.RUnlock()
	seen := make(map[primitives.BuilderIndex]struct{})
	for _, r := range c.relays {
		if r.bannedUntil <= epoch {
			continue
		}
		for m := range r.members {
			if c.directlyBlacklisted(m, epoch) {
				continue
			}
			seen[m] = struct{}{}
		}
	}
	return uint64(len(seen))
}

// DropInactiveBuilders clears the records and endpoint associations of builders that can no longer
// bid, so that a recycled index does not punish its newcomer. Unresolvable indices keep theirs.
//
// The lock is released across isActive: the caller holds the forkchoice write lock and isActive
// takes the state lock, so calling out from under the breaker lock would nest the three.
func (c *BuilderCircuitBreaker) DropInactiveBuilders(epoch primitives.Epoch, isActive func(primitives.BuilderIndex) (bool, error)) {
	if c == nil {
		return
	}
	c.lock.Lock()
	candidates := make([]primitives.BuilderIndex, 0, len(c.failures))
	for idx := range c.failures {
		candidates = append(candidates, idx)
	}
	if !c.relaySwept || epoch > c.lastRelaySweepEpoch {
		c.lastRelaySweepEpoch, c.relaySwept = epoch, true
		for idx := range c.relaysByIndex {
			if _, ok := c.failures[idx]; !ok {
				candidates = append(candidates, idx)
			}
		}
	}
	c.lock.Unlock()

	inactive := make([]primitives.BuilderIndex, 0, len(candidates))
	for _, idx := range candidates {
		active, err := isActive(idx)
		if err != nil {
			continue
		}
		if !active {
			inactive = append(inactive, idx)
		}
	}
	if len(inactive) == 0 {
		return
	}

	c.lock.Lock()
	defer c.lock.Unlock()
	for _, idx := range inactive {
		delete(c.failures, idx)
		for _, ep := range slices.Clone(c.relaysByIndex[idx]) {
			c.unlinkRelay(ep, idx)
		}
	}
	for ep, r := range c.relays {
		if len(r.members) == 0 {
			c.dropRelay(ep)
		}
	}
}

// Prune drops records whose blacklist and back off periods have both elapsed, recorded roots that
// can no longer be revisited, and endpoint associations that have gone stale.
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
	ttl := params.BeaconConfig().BuilderRelayAssociationTTL
	for ep, r := range c.relays {
		if r.bannedUntil > epoch {
			continue
		}
		r.bannedUntil, r.bannedBy = 0, 0
		for m, seen := range r.members {
			if expired(epoch, seen, ttl) {
				c.unlinkRelay(ep, m)
			}
		}
		if len(r.members) == 0 || expired(epoch, r.lastSeenEpoch, ttl) {
			c.dropRelay(ep)
		}
	}
}

func expired(epoch, seen, ttl primitives.Epoch) bool {
	return epoch > seen && epoch-seen > ttl
}

// linkRelay, unlinkRelay and dropRelay are the only writers of members and relaysByIndex, which
// must stay mirror images of each other. All three require the write lock.
func (c *BuilderCircuitBreaker) linkRelay(key string, r *relayRecord, idx primitives.BuilderIndex, epoch primitives.Epoch) {
	seen, member := r.members[idx]
	if !member {
		c.relaysByIndex[idx] = append(c.relaysByIndex[idx], key)
	}
	if !member || epoch > seen {
		r.members[idx] = epoch
	}
}

func (c *BuilderCircuitBreaker) unlinkRelay(key string, idx primitives.BuilderIndex) {
	if r, ok := c.relays[key]; ok {
		delete(r.members, idx)
	}
	endpoints := c.relaysByIndex[idx]
	if i := slices.Index(endpoints, key); i >= 0 {
		endpoints = slices.Delete(endpoints, i, i+1)
	}
	if len(endpoints) == 0 {
		delete(c.relaysByIndex, idx)
		return
	}
	c.relaysByIndex[idx] = endpoints
}

func (c *BuilderCircuitBreaker) dropRelay(key string) {
	r, ok := c.relays[key]
	if !ok {
		return
	}
	for idx := range r.members {
		c.unlinkRelay(key, idx)
	}
	delete(c.relays, key)
}

func (c *BuilderCircuitBreaker) evictRelay(epoch primitives.Epoch) bool {
	var victim string
	var oldest primitives.Epoch
	for ep, r := range c.relays {
		if r.bannedUntil > epoch {
			continue
		}
		if victim == "" || r.lastSeenEpoch < oldest {
			victim, oldest = ep, r.lastSeenEpoch
		}
	}
	if victim == "" {
		return false
	}
	c.dropRelay(victim)
	return true
}

func (c *BuilderCircuitBreaker) evictMember(key string, r *relayRecord) bool {
	var victim primitives.BuilderIndex
	var oldest primitives.Epoch
	found := false
	for m, seen := range r.members {
		if f, ok := c.failures[m]; ok && f.failed > 0 {
			continue
		}
		if !found || seen < oldest {
			victim, oldest, found = m, seen, true
		}
	}
	if !found {
		return false
	}
	c.unlinkRelay(key, victim)
	return true
}

// directlyBlacklisted requires the caller to hold the lock.
func (c *BuilderCircuitBreaker) directlyBlacklisted(idx primitives.BuilderIndex, epoch primitives.Epoch) bool {
	f, ok := c.failures[idx]
	return ok && f.blacklistUntilEpoch > epoch
}

// relayBannedFor requires the caller to hold the lock.
func (c *BuilderCircuitBreaker) relayBannedFor(idx primitives.BuilderIndex, epoch primitives.Epoch) bool {
	for _, ep := range c.relaysByIndex[idx] {
		if r, ok := c.relays[ep]; ok && r.bannedUntil > epoch {
			return true
		}
	}
	return false
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

// normalizeRelay reduces a validator supplied builder url to a stable identity, so that the same
// endpoint reached with different credentials or spelling is banned once. Credentials are dropped
// rather than stored.
func normalizeRelay(raw string) (string, bool) {
	if raw == "" || len(raw) > maxRelayKeyLen || !printableASCII(raw) {
		return "", false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		host, port, splitErr := net.SplitHostPort(raw)
		if splitErr != nil || host == "" {
			return "", false
		}
		if _, err := strconv.ParseUint(port, 10, 16); err != nil {
			return "", false
		}
		return "http://" + strings.ToLower(net.JoinHostPort(host, port)), true
	}
	return strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host) + strings.TrimSuffix(u.Path, "/"), true
}

func printableASCII(s string) bool {
	return !strings.ContainsFunc(s, func(r rune) bool { return r < '!' || r > '~' })
}

// checkRelayInvariants asserts that relays and relaysByIndex mirror each other. Used by tests.
func (c *BuilderCircuitBreaker) checkRelayInvariants() error {
	c.lock.RLock()
	defer c.lock.RUnlock()
	for ep, r := range c.relays {
		for idx := range r.members {
			if !slices.Contains(c.relaysByIndex[idx], ep) {
				return fmt.Errorf("relay %s has member %d with no reverse entry", ep, idx)
			}
		}
	}
	for idx, endpoints := range c.relaysByIndex {
		if len(endpoints) == 0 {
			return fmt.Errorf("builder %d has an empty reverse entry", idx)
		}
		for _, ep := range endpoints {
			r, ok := c.relays[ep]
			if !ok {
				return fmt.Errorf("builder %d points at untracked relay %s", idx, ep)
			}
			if _, ok := r.members[idx]; !ok {
				return fmt.Errorf("builder %d points at relay %s that does not list it", idx, ep)
			}
		}
	}
	return nil
}
