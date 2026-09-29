package cache

import (
	"errors"
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
	require.Equal(t, false, c.RecordFailure(1, root(1), 0))
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
	require.Equal(t, false, c.RecordFailure(1, root(1), 10))
	require.Equal(t, false, c.Blacklisted(1, 10))

	// Second failure exceeds AllowedFailures.
	require.Equal(t, true, c.RecordFailure(1, root(2), 10))
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
	require.Equal(t, true, c.RecordFailure(1, root(1), 10))
	require.Equal(t, false, c.Blacklisted(1, 11)) // short ban expired

	// A second failure within the back off window escalates to the critical ban.
	require.Equal(t, true, c.RecordFailure(1, root(2), 12))
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
	require.Equal(t, true, c.RecordFailure(1, root(1), 5))
	require.Equal(t, true, c.Blacklisted(1, 5))
	require.Equal(t, false, c.Blacklisted(1, 6))

	// Pruning while the back off window is open must not drop the counter.
	c.Prune(6)
	c.lock.RLock()
	require.Equal(t, uint64(1), c.failures[1].failed)
	c.lock.RUnlock()

	// Still inside the window, so this is the second offense and earns the long ban.
	require.Equal(t, true, c.RecordFailure(1, root(2), 7))
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
	require.Equal(t, true, c.RecordFailure(1, root(1), 10))

	// Failing again after the back off period is a first offense again, so only a short ban.
	require.Equal(t, true, c.RecordFailure(1, root(2), 20))
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
	require.Equal(t, true, c.RecordFailure(1, root(1), 10))
	require.Equal(t, true, c.RecordFailure(1, root(2), 10))
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
	require.Equal(t, true, c.RecordFailure(1, r, 10))
	// Two children building on the same empty parent must not escalate to a critical ban.
	require.Equal(t, false, c.RecordFailure(1, r, 10))
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
		require.Equal(t, true, c.RecordFailure(primitives.BuilderIndex(i), root(byte(i)), 0))
	}
	require.Equal(t, false, c.SelfBuildOnly(0))

	require.Equal(t, true, c.RecordFailure(2, root(2), 0))
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
	require.Equal(t, true, c.RecordFailure(1, root(1), 10))
	require.Equal(t, true, c.RecordFailure(1, root(2), 10))
	require.Equal(t, true, c.Blacklisted(1, 100))

	// Still active, so the ban stands.
	c.DropInactiveBuilders(registry(map[primitives.BuilderIndex]bool{1: true}))
	require.Equal(t, true, c.Blacklisted(1, 100))

	c.DropInactiveBuilders(registry(map[primitives.BuilderIndex]bool{1: false}))
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
	require.Equal(t, true, c.RecordFailure(1, root(1), 10))

	c.DropInactiveBuilders(registry(nil))
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
	require.Equal(t, true, c.RecordFailure(1, root(1), 10))

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
