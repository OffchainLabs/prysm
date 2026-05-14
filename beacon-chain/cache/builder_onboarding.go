package cache

import (
	"crypto/sha256"
	"encoding/binary"
	"sync"

	lruwrpr "github.com/OffchainLabs/prysm/v7/cache/lru"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	lru "github.com/hashicorp/golang-lru"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const builderOnboardingCacheSize = 1 << 18

var (
	builderOnboardingCacheHit = promauto.NewCounter(prometheus.CounterOpts{
		Name: "builder_onboarding_cache_hit",
		Help: "Total cache hits on the pending-deposit signature cache used by Gloas builder onboarding.",
	})
	builderOnboardingCacheMiss = promauto.NewCounter(prometheus.CounterOpts{
		Name: "builder_onboarding_cache_miss",
		Help: "Total cache misses on the pending-deposit signature cache used by Gloas builder onboarding.",
	})
)

// BuilderOnboardingSig stores per-pending-deposit BLS validity verdicts.
// Populated as deposits enter pending_deposits pre-Gloas; consulted by
// OnboardBuildersFromPendingDeposits at the Gloas fork upgrade so the BLS
// cost is amortized across many slots instead of spiking at the fork.
var BuilderOnboardingSig = NewBuilderOnboardingCache()

// BuilderOnboardingCache is a content-addressed LRU of pending-deposit
// signature verdicts. Keys are sha256(pubkey || creds || amount_le || sig).
// Deposit signatures are fork-agnostic, so a verdict for a given key never
// changes.
type BuilderOnboardingCache struct {
	cache *lru.Cache
	mu    sync.Mutex
}

func NewBuilderOnboardingCache() *BuilderOnboardingCache {
	return &BuilderOnboardingCache{cache: lruwrpr.New(builderOnboardingCacheSize)}
}

func (c *BuilderOnboardingCache) Get(key [32]byte) (bool, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.cache.Get(key)
	if !ok {
		builderOnboardingCacheMiss.Inc()
		return false, false
	}
	builderOnboardingCacheHit.Inc()
	return v.(bool), true
}

func (c *BuilderOnboardingCache) Put(key [32]byte, valid bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache.Add(key, valid)
}

// PendingDepositKey computes the cache key for a pending deposit.
func PendingDepositKey(pd *ethpb.PendingDeposit) [32]byte {
	h := sha256.New()
	h.Write(pd.PublicKey)
	h.Write(pd.WithdrawalCredentials)
	var amountBuf [8]byte
	binary.LittleEndian.PutUint64(amountBuf[:], pd.Amount)
	h.Write(amountBuf[:])
	h.Write(pd.Signature)
	var out [32]byte
	h.Sum(out[:0])
	return out
}
