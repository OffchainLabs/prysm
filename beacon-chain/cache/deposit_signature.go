package cache

import (
	lruwrpr "github.com/OffchainLabs/prysm/v7/cache/lru"
	lru "github.com/hashicorp/golang-lru"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const depositSigCacheSize = 4

var (
	depositSigCacheHit = promauto.NewCounter(prometheus.CounterOpts{
		Name: "deposit_sig_cache_hit",
		Help: "Total cache hits on the deposit signature pre-verification cache.",
	})
	depositSigCacheMiss = promauto.NewCounter(prometheus.CounterOpts{
		Name: "deposit_sig_cache_miss",
		Help: "Total cache misses on the deposit signature pre-verification cache.",
	})
)

// DepositSig caches per-deposit BLS verdicts keyed by execution_requests_root.
var DepositSig = NewDepositSigCache()

type DepositSigCache struct {
	cache *lru.Cache
}

func NewDepositSigCache() *DepositSigCache {
	return &DepositSigCache{cache: lruwrpr.New(depositSigCacheSize)}
}

func (c *DepositSigCache) Get(root [32]byte) ([]bool, bool) {
	v, ok := c.cache.Get(root)
	if !ok {
		depositSigCacheMiss.Inc()
		return nil, false
	}
	depositSigCacheHit.Inc()
	return v.([]bool), true
}

func (c *DepositSigCache) Put(root [32]byte, valid []bool) {
	c.cache.Add(root, valid)
}
