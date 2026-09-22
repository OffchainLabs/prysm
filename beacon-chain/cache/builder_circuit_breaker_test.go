package cache

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func root(b byte) [32]byte {
	var r [32]byte
	r[0] = b
	return r
}

// registry returns an isActive resolver. An absent index is unknown to the registry.
func registry(active map[primitives.BuilderIndex]bool) func(primitives.BuilderIndex) (bool, error) {
	return func(idx primitives.BuilderIndex) (bool, error) {
		isActive, ok := active[idx]
		if !ok {
			return false, errors.New("index out of range")
		}
		return isActive, nil
	}
}

func TestBuilderCircuitBreaker_NilSafe(t *testing.T) {
	var c *BuilderCircuitBreaker
	require.Equal(t, false, c.Blacklisted(1, 0))
	require.Equal(t, false, c.RecordFailure(1, root(1), 0).Blacklisted)
	require.Equal(t, false, c.SelfBuildOnly(0))
	require.Equal(t, uint64(0), c.BlacklistedCount(0))
	c.RecordSuccess(1)
	c.Prune(0)
}

func TestBuilderCircuitBreaker_AllowedFailures(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.BuilderAllowedFailures = 1
	cfg.BuilderCriticalFailures = 3
	cfg.BuilderBlacklistPeriod = 2
	cfg.BuilderFailureBackOffPeriod = 5
	require.NoError(t, params.SetActive(cfg))

	c := NewBuilderCircuitBreaker()

	// First failure is within tolerance.
	require.Equal(t, false, c.RecordFailure(1, root(1), 10).Blacklisted)
	require.Equal(t, false, c.Blacklisted(1, 10))

	// Second failure exceeds AllowedFailures.
	require.Equal(t, true, c.RecordFailure(1, root(2), 10).Blacklisted)
	require.Equal(t, true, c.Blacklisted(1, 10))
	require.Equal(t, true, c.Blacklisted(1, 11))
	// blacklistUntilEpoch is 12, so the ban has lifted at 12.
	require.Equal(t, false, c.Blacklisted(1, 12))
}

func TestBuilderCircuitBreaker_CriticalFailuresBanLonger(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.BuilderAllowedFailures = 0
	cfg.BuilderCriticalFailures = 2
	cfg.BuilderBlacklistPeriod = 1
	cfg.BuilderCriticalBlacklistPeriod = 256
	cfg.BuilderFailureBackOffPeriod = 5
	require.NoError(t, params.SetActive(cfg))

	c := NewBuilderCircuitBreaker()
	require.Equal(t, true, c.RecordFailure(1, root(1), 10).Blacklisted)
	require.Equal(t, false, c.Blacklisted(1, 11)) // short ban expired

	// A second failure within the back off window escalates to the critical ban.
	require.Equal(t, true, c.RecordFailure(1, root(2), 12).Blacklisted)
	require.Equal(t, true, c.Blacklisted(1, 200))
	require.Equal(t, true, c.Blacklisted(1, 267))
	require.Equal(t, false, c.Blacklisted(1, 268))
}

// A builder is whitelisted again at blacklistUntilEpoch while its failure counter lives on until
// backOffEpoch, so a repeat offense in between escalates instead of starting over.
func TestBuilderCircuitBreaker_WhitelistedBeforeCounterResets(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.BuilderAllowedFailures = 0
	cfg.BuilderCriticalFailures = 2
	cfg.BuilderBlacklistPeriod = 1      // ban lifts at epoch 6
	cfg.BuilderFailureBackOffPeriod = 3 // counter resets at epoch 8
	cfg.BuilderCriticalBlacklistPeriod = 256
	require.NoError(t, params.SetActive(cfg))

	c := NewBuilderCircuitBreaker()
	require.Equal(t, true, c.RecordFailure(1, root(1), 5).Blacklisted)
	require.Equal(t, true, c.Blacklisted(1, 5))
	require.Equal(t, false, c.Blacklisted(1, 6))

	// Pruning while the back off window is open must not drop the counter.
	c.Prune(6)
	c.lock.RLock()
	require.Equal(t, uint64(1), c.failures[1].failed)
	c.lock.RUnlock()

	// Still inside the window, so this is the second offense and earns the long ban.
	require.Equal(t, true, c.RecordFailure(1, root(2), 7).Blacklisted)
	require.Equal(t, true, c.Blacklisted(1, 100))
	require.Equal(t, true, c.Blacklisted(1, 262))
	require.Equal(t, false, c.Blacklisted(1, 263))
}

func TestBuilderCircuitBreaker_BackOffResetsCounter(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.BuilderAllowedFailures = 0
	cfg.BuilderCriticalFailures = 2
	cfg.BuilderBlacklistPeriod = 1
	cfg.BuilderCriticalBlacklistPeriod = 256
	cfg.BuilderFailureBackOffPeriod = 5
	require.NoError(t, params.SetActive(cfg))

	c := NewBuilderCircuitBreaker()
	require.Equal(t, true, c.RecordFailure(1, root(1), 10).Blacklisted)

	// Failing again after the back off period is a first offense again, so only a short ban.
	require.Equal(t, true, c.RecordFailure(1, root(2), 20).Blacklisted)
	require.Equal(t, true, c.Blacklisted(1, 20))
	require.Equal(t, false, c.Blacklisted(1, 21))
}

func TestBuilderCircuitBreaker_RecordSuccessClearsBan(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.BuilderAllowedFailures = 0
	cfg.BuilderCriticalFailures = 2
	cfg.BuilderCriticalBlacklistPeriod = 256
	require.NoError(t, params.SetActive(cfg))

	c := NewBuilderCircuitBreaker()
	require.Equal(t, true, c.RecordFailure(1, root(1), 10).Blacklisted)
	require.Equal(t, true, c.RecordFailure(1, root(2), 10).Blacklisted)
	require.Equal(t, true, c.Blacklisted(1, 100))

	c.RecordSuccess(1)
	require.Equal(t, false, c.Blacklisted(1, 100))
	require.Equal(t, uint64(0), c.BlacklistedCount(100))
}

func TestBuilderCircuitBreaker_IdempotentPerRoot(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.BuilderAllowedFailures = 0
	cfg.BuilderCriticalFailures = 2
	cfg.BuilderBlacklistPeriod = 1
	cfg.BuilderCriticalBlacklistPeriod = 256
	require.NoError(t, params.SetActive(cfg))

	c := NewBuilderCircuitBreaker()
	r := root(1)
	require.Equal(t, true, c.RecordFailure(1, r, 10).Blacklisted)
	// Two children building on the same empty parent must not escalate to a critical ban.
	require.Equal(t, false, c.RecordFailure(1, r, 10).Blacklisted)
	require.Equal(t, false, c.Blacklisted(1, 11))
}

func TestBuilderCircuitBreaker_SelfBuildOnlyThreshold(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.BuilderAllowedFailures = 0
	cfg.BuilderBlacklistPeriod = 10
	cfg.BuilderCriticalFailedBuilders = 3
	require.NoError(t, params.SetActive(cfg))

	c := NewBuilderCircuitBreaker()
	require.Equal(t, false, c.SelfBuildOnly(0))

	for i := 0; i < 2; i++ {
		require.Equal(t, true, c.RecordFailure(primitives.BuilderIndex(i), root(byte(i)), 0).Blacklisted)
	}
	require.Equal(t, false, c.SelfBuildOnly(0))

	require.Equal(t, true, c.RecordFailure(2, root(2), 0).Blacklisted)
	require.Equal(t, uint64(3), c.BlacklistedCount(0))
	require.Equal(t, true, c.SelfBuildOnly(0))

	// Bans expire and the breaker re-opens.
	require.Equal(t, false, c.SelfBuildOnly(10))
}

// An exit clears the record, so a recycled index starts clean.
func TestBuilderCircuitBreaker_DropInactiveBuilders(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.BuilderAllowedFailures = 0
	cfg.BuilderCriticalFailures = 2
	cfg.BuilderCriticalBlacklistPeriod = 256
	require.NoError(t, params.SetActive(cfg))

	c := NewBuilderCircuitBreaker()
	require.Equal(t, true, c.RecordFailure(1, root(1), 10).Blacklisted)
	require.Equal(t, true, c.RecordFailure(1, root(2), 10).Blacklisted)
	require.Equal(t, true, c.Blacklisted(1, 100))

	// Still active, so the ban stands.
	c.DropInactiveBuilders(100, registry(map[primitives.BuilderIndex]bool{1: true}))
	require.Equal(t, true, c.Blacklisted(1, 100))

	c.DropInactiveBuilders(100, registry(map[primitives.BuilderIndex]bool{1: false}))
	require.Equal(t, false, c.Blacklisted(1, 100))
	require.Equal(t, uint64(0), c.BlacklistedCount(100))
}

// A lookup failure must not become a way to shed a ban.
func TestBuilderCircuitBreaker_DropInactiveBuildersKeepsUnresolved(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.BuilderAllowedFailures = 0
	cfg.BuilderBlacklistPeriod = 10
	require.NoError(t, params.SetActive(cfg))

	c := NewBuilderCircuitBreaker()
	require.Equal(t, true, c.RecordFailure(1, root(1), 10).Blacklisted)

	c.DropInactiveBuilders(100, registry(nil))
	require.Equal(t, true, c.Blacklisted(1, 10))
}

func TestBuilderCircuitBreaker_Prune(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.BuilderAllowedFailures = 0
	cfg.BuilderBlacklistPeriod = 1
	cfg.BuilderFailureBackOffPeriod = 2
	require.NoError(t, params.SetActive(cfg))

	c := NewBuilderCircuitBreaker()
	require.Equal(t, true, c.RecordFailure(1, root(1), 10).Blacklisted)

	// Still inside the back off window, the record must survive so a repeat offense escalates.
	c.Prune(11)
	c.lock.RLock()
	require.Equal(t, 1, len(c.failures))
	require.Equal(t, 1, len(c.recorded))
	c.lock.RUnlock()

	c.Prune(13)
	c.lock.RLock()
	require.Equal(t, 0, len(c.failures))
	require.Equal(t, 0, len(c.recorded))
	c.lock.RUnlock()
}

func relayTestConfig(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.BuilderAllowedFailures = 0
	cfg.BuilderCriticalFailures = 2
	cfg.BuilderBlacklistPeriod = 10
	cfg.BuilderCriticalBlacklistPeriod = 256
	cfg.BuilderFailureBackOffPeriod = 5
	cfg.BuilderCriticalFailedBuilders = 7
	cfg.BuilderRelayBlacklistPeriod = 32
	cfg.BuilderRelayAssociationTTL = 64
	cfg.BuilderMaxTrackedRelays = 64
	cfg.BuilderMaxIndicesPerRelay = 16
	require.NoError(t, params.SetActive(cfg))
}

func TestNormalizeRelay(t *testing.T) {
	tests := []struct {
		raw  string
		want string
		ok   bool
	}{
		{"https://relay.io", "https://relay.io", true},
		{"https://Relay.IO/", "https://relay.io", true},
		{"https://user:pass@relay.io/eth/v1?x=1", "https://relay.io/eth/v1", true},
		{"http://relay.io/eth/v1", "http://relay.io/eth/v1", true},
		{"localhost:8551", "http://localhost:8551", true},
		{"[::1]:443", "http://[::1]:443", true},
		{"", "", false},
		{"ftp://relay.io", "", false},
		{"https://relay.io/ maliciously spaced", "", false},
		{strings.Repeat("a", maxRelayKeyLen+1), "", false},
	}
	for _, tt := range tests {
		got, ok := normalizeRelay(tt.raw)
		require.Equal(t, tt.ok, ok, tt.raw)
		require.Equal(t, tt.want, got, tt.raw)
	}
}

// The same endpoint spelled differently must be one relay, or a ban is trivially evaded.
func TestBuilderCircuitBreaker_RelayIdentityIsNormalized(t *testing.T) {
	relayTestConfig(t)
	c := NewBuilderCircuitBreaker()
	c.ObserveRelayBid("https://user:pass@Relay.IO/", 1, 10)
	require.Equal(t, true, c.RecordFailure(1, root(1), 10).Blacklisted)
	require.Equal(t, true, c.RelayBanned("https://relay.io", 10))
	require.NoError(t, c.checkRelayInvariants())
}

func TestBuilderCircuitBreaker_RelayBanIsCollateral(t *testing.T) {
	relayTestConfig(t)
	c := NewBuilderCircuitBreaker()
	c.ObserveRelayBid("https://a.io", 1, 10)
	c.ObserveRelayBid("https://a.io", 2, 10)
	c.ObserveRelayBid("https://b.io", 3, 10)

	out := c.RecordFailure(1, root(1), 10)
	require.Equal(t, true, out.Blacklisted)
	require.DeepEqual(t, []string{"https://a.io"}, out.BannedRelays)
	require.Equal(t, 1, out.Collateral)

	require.Equal(t, true, c.RelayBanned("https://a.io", 10))
	require.Equal(t, false, c.RelayBanned("https://b.io", 10))
	require.Equal(t, true, c.Blacklisted(1, 10))
	require.Equal(t, true, c.Blacklisted(2, 10))
	require.Equal(t, false, c.Blacklisted(3, 10))
	require.NoError(t, c.checkRelayInvariants())
}

// One endpoint fronting many builders must not trip the global self-build fallback.
func TestBuilderCircuitBreaker_CollateralDoesNotTripSelfBuild(t *testing.T) {
	relayTestConfig(t)
	c := NewBuilderCircuitBreaker()
	for i := 0; i < 16; i++ {
		c.ObserveRelayBid("https://a.io", primitives.BuilderIndex(i), 10)
	}
	require.Equal(t, true, c.RecordFailure(0, root(1), 10).Blacklisted)

	require.Equal(t, uint64(1), c.BlacklistedCount(10))
	require.Equal(t, false, c.SelfBuildOnly(10))
	require.Equal(t, uint64(1), c.RelayBannedCount(10))
	require.Equal(t, uint64(15), c.CollateralBlacklistedCount(10))
	for i := 0; i < 16; i++ {
		require.Equal(t, true, c.Blacklisted(primitives.BuilderIndex(i), 10))
	}
}

// A collaterally banned builder must not drag in the other endpoints that serve it.
func TestBuilderCircuitBreaker_BanStopsAtDepthOne(t *testing.T) {
	relayTestConfig(t)
	c := NewBuilderCircuitBreaker()
	c.ObserveRelayBid("https://a.io", 1, 10)
	c.ObserveRelayBid("https://a.io", 2, 10)
	c.ObserveRelayBid("https://b.io", 2, 10)
	c.ObserveRelayBid("https://b.io", 3, 10)

	require.Equal(t, true, c.RecordFailure(1, root(1), 10).Blacklisted)
	require.Equal(t, true, c.RelayBanned("https://a.io", 10))
	require.Equal(t, false, c.RelayBanned("https://b.io", 10))
	require.Equal(t, true, c.Blacklisted(2, 10))
	require.Equal(t, false, c.Blacklisted(3, 10))
}

func TestBuilderCircuitBreaker_RelayBanCappedByRelayPeriod(t *testing.T) {
	relayTestConfig(t)
	cfg := params.BeaconConfig().Copy()
	cfg.BuilderRelayBlacklistPeriod = 4
	require.NoError(t, params.SetActive(cfg))

	c := NewBuilderCircuitBreaker()
	c.ObserveRelayBid("https://a.io", 1, 10)
	c.ObserveRelayBid("https://a.io", 2, 10)
	require.Equal(t, true, c.RecordFailure(1, root(1), 10).Blacklisted)
	require.Equal(t, true, c.RecordFailure(1, root(2), 10+1).Blacklisted) // critical, 256 epochs

	require.Equal(t, true, c.Blacklisted(1, 200)) // offender keeps the long ban
	require.Equal(t, true, c.Blacklisted(2, 14))  // collateral ends with the relay ban
	require.Equal(t, false, c.Blacklisted(2, 15))
	require.Equal(t, false, c.RelayBanned("https://a.io", 15))
}

// The offender revealing a payload releases the endpoint and everything it serves.
func TestBuilderCircuitBreaker_RecordSuccessReleasesRelay(t *testing.T) {
	relayTestConfig(t)
	c := NewBuilderCircuitBreaker()
	c.ObserveRelayBid("https://a.io", 1, 10)
	c.ObserveRelayBid("https://a.io", 2, 10)
	require.Equal(t, true, c.RecordFailure(1, root(1), 10).Blacklisted)

	c.RecordSuccess(1)
	require.Equal(t, false, c.RelayBanned("https://a.io", 10))
	require.Equal(t, false, c.Blacklisted(1, 10))
	require.Equal(t, false, c.Blacklisted(2, 10))
	require.NoError(t, c.checkRelayInvariants())
}

// A member that was never charged cannot vacuously lift the ban its sibling earned.
func TestBuilderCircuitBreaker_CollateralSuccessDoesNotRelease(t *testing.T) {
	relayTestConfig(t)
	c := NewBuilderCircuitBreaker()
	c.ObserveRelayBid("https://a.io", 1, 10)
	c.ObserveRelayBid("https://a.io", 2, 10)
	require.Equal(t, true, c.RecordFailure(1, root(1), 10).Blacklisted)

	c.RecordSuccess(2)
	require.Equal(t, true, c.RelayBanned("https://a.io", 10))
	require.Equal(t, true, c.Blacklisted(2, 10))
}

func TestBuilderCircuitBreaker_ReleaseWaitsForEveryFailingMember(t *testing.T) {
	relayTestConfig(t)
	c := NewBuilderCircuitBreaker()
	c.ObserveRelayBid("https://a.io", 1, 10)
	c.ObserveRelayBid("https://a.io", 2, 10)
	require.Equal(t, true, c.RecordFailure(1, root(1), 10).Blacklisted)
	require.Equal(t, true, c.RecordFailure(2, root(2), 10).Blacklisted)

	c.RecordSuccess(1)
	require.Equal(t, true, c.RelayBanned("https://a.io", 10))
	c.RecordSuccess(2)
	require.Equal(t, false, c.RelayBanned("https://a.io", 10))
}

// A builder stays banned while any endpoint serving it is banned, with no refcounting.
func TestBuilderCircuitBreaker_MultiRelayMember(t *testing.T) {
	relayTestConfig(t)
	c := NewBuilderCircuitBreaker()
	c.ObserveRelayBid("https://a.io", 1, 10)
	c.ObserveRelayBid("https://a.io", 2, 10)
	c.ObserveRelayBid("https://b.io", 2, 10)
	c.ObserveRelayBid("https://b.io", 3, 10)

	require.Equal(t, true, c.RecordFailure(1, root(1), 10).Blacklisted)
	require.Equal(t, true, c.RecordFailure(3, root(2), 10).Blacklisted)
	require.Equal(t, true, c.Blacklisted(2, 10))

	c.RecordSuccess(1)
	require.Equal(t, true, c.Blacklisted(2, 10)) // b.io still banned
	c.RecordSuccess(3)
	require.Equal(t, false, c.Blacklisted(2, 10))
	require.NoError(t, c.checkRelayInvariants())
}

// The relay ban never outlives the offender's record, so a release stays possible.
func TestBuilderCircuitBreaker_PruneKeepsOffenderDuringRelayBan(t *testing.T) {
	relayTestConfig(t)
	c := NewBuilderCircuitBreaker()
	c.ObserveRelayBid("https://a.io", 1, 10)
	c.ObserveRelayBid("https://a.io", 2, 10)
	require.Equal(t, true, c.RecordFailure(1, root(1), 10).Blacklisted)

	c.lock.RLock()
	require.Equal(t, true, c.relays["https://a.io"].bannedUntil <= c.failures[1].blacklistUntilEpoch)
	c.lock.RUnlock()

	for e := primitives.Epoch(10); e < 20; e++ {
		c.Prune(e)
		if c.RelayBanned("https://a.io", e) {
			c.lock.RLock()
			_, ok := c.failures[1]
			c.lock.RUnlock()
			require.Equal(t, true, ok, "offender record dropped while relay still banned")
		}
	}
	require.NoError(t, c.checkRelayInvariants())
}

func TestBuilderCircuitBreaker_PruneDropsStaleRelays(t *testing.T) {
	relayTestConfig(t)
	c := NewBuilderCircuitBreaker()
	c.ObserveRelayBid("https://a.io", 1, 10)
	require.Equal(t, true, c.RecordFailure(1, root(1), 10).Blacklisted)

	// The offender's own ban ends at 20, and the relay ban is capped by it.
	c.Prune(15)
	require.Equal(t, true, c.RelayBanned("https://a.io", 15))
	c.Prune(25)
	require.Equal(t, false, c.RelayBanned("https://a.io", 25))
	c.lock.RLock()
	require.Equal(t, 1, len(c.relays))
	c.lock.RUnlock()

	c.Prune(200) // past the association TTL
	c.lock.RLock()
	require.Equal(t, 0, len(c.relays))
	require.Equal(t, 0, len(c.relaysByIndex))
	c.lock.RUnlock()
	require.NoError(t, c.checkRelayInvariants())
}

// A recycled index must not inherit a collateral ban, even with no failure record of its own.
func TestBuilderCircuitBreaker_DropInactiveUnlinksRecordlessMembers(t *testing.T) {
	relayTestConfig(t)
	c := NewBuilderCircuitBreaker()
	c.ObserveRelayBid("https://a.io", 1, 10)
	c.ObserveRelayBid("https://a.io", 2, 10)
	require.Equal(t, true, c.RecordFailure(1, root(1), 10).Blacklisted)
	require.Equal(t, true, c.Blacklisted(2, 10))

	c.DropInactiveBuilders(11, registry(map[primitives.BuilderIndex]bool{1: true, 2: false}))
	require.Equal(t, false, c.Blacklisted(2, 10))
	require.Equal(t, true, c.Blacklisted(1, 10))
	require.NoError(t, c.checkRelayInvariants())

	c.DropInactiveBuilders(12, registry(map[primitives.BuilderIndex]bool{1: false}))
	c.lock.RLock()
	require.Equal(t, 0, len(c.relays))
	c.lock.RUnlock()
	require.NoError(t, c.checkRelayInvariants())
}

func TestBuilderCircuitBreaker_RelayCapEvictsUnbannedOnly(t *testing.T) {
	relayTestConfig(t)
	cfg := params.BeaconConfig().Copy()
	cfg.BuilderMaxTrackedRelays = 2
	require.NoError(t, params.SetActive(cfg))

	c := NewBuilderCircuitBreaker()
	c.ObserveRelayBid("https://a.io", 1, 10)
	c.ObserveRelayBid("https://b.io", 2, 11)
	require.Equal(t, true, c.RecordFailure(1, root(1), 10).Blacklisted)

	// a.io is banned, so b.io is the only eligible victim.
	c.ObserveRelayBid("https://c.io", 3, 12)
	require.Equal(t, true, c.RelayBanned("https://a.io", 12))
	c.lock.RLock()
	_, hasB := c.relays["https://b.io"]
	_, hasC := c.relays["https://c.io"]
	c.lock.RUnlock()
	require.Equal(t, false, hasB)
	require.Equal(t, true, hasC)

	// With every tracked relay banned, a new endpoint is refused rather than evicting a ban.
	require.Equal(t, true, c.RecordFailure(3, root(2), 12).Blacklisted)
	c.ObserveRelayBid("https://d.io", 4, 13)
	c.lock.RLock()
	_, hasD := c.relays["https://d.io"]
	c.lock.RUnlock()
	require.Equal(t, false, hasD)
	require.NoError(t, c.checkRelayInvariants())
}

func TestBuilderCircuitBreaker_MemberCapKeepsFailingMembers(t *testing.T) {
	relayTestConfig(t)
	cfg := params.BeaconConfig().Copy()
	cfg.BuilderMaxIndicesPerRelay = 2
	require.NoError(t, params.SetActive(cfg))

	c := NewBuilderCircuitBreaker()
	c.ObserveRelayBid("https://a.io", 1, 10)
	c.ObserveRelayBid("https://a.io", 2, 11)
	require.Equal(t, true, c.RecordFailure(1, root(1), 10).Blacklisted)

	// Builder 2 is the only evictable member, builder 1 is the reason for the ban.
	c.ObserveRelayBid("https://a.io", 3, 12)
	require.Equal(t, true, c.Blacklisted(1, 12))
	require.Equal(t, true, c.Blacklisted(3, 12))
	require.Equal(t, false, c.Blacklisted(2, 12))
	require.NoError(t, c.checkRelayInvariants())
}

func TestBuilderCircuitBreaker_ObserveIgnoresSelfBuild(t *testing.T) {
	relayTestConfig(t)
	c := NewBuilderCircuitBreaker()
	c.ObserveRelayBid("https://a.io", params.BeaconConfig().BuilderIndexSelfBuild, 10)
	c.lock.RLock()
	require.Equal(t, 0, len(c.relays))
	c.lock.RUnlock()
}

func TestBuilderCircuitBreaker_RelayConcurrency(t *testing.T) {
	relayTestConfig(t)
	c := NewBuilderCircuitBreaker()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(3)
		go func(i int) {
			defer wg.Done()
			c.ObserveRelayBid(fmt.Sprintf("https://r%d.io", i%3), primitives.BuilderIndex(i), primitives.Epoch(i))
		}(i)
		go func(i int) {
			defer wg.Done()
			c.RecordFailure(primitives.BuilderIndex(i), root(byte(i)), primitives.Epoch(i))
		}(i)
		go func(i int) {
			defer wg.Done()
			c.Blacklisted(primitives.BuilderIndex(i), primitives.Epoch(i))
			c.RelayBanned(fmt.Sprintf("https://r%d.io", i%3), primitives.Epoch(i))
		}(i)
	}
	wg.Wait()
	require.NoError(t, c.checkRelayInvariants())
}
