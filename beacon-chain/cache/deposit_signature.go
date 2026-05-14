package cache

import (
	"fmt"
	"sync"
	"time"

	lruwrpr "github.com/OffchainLabs/prysm/v7/cache/lru"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
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

// DepositSig is the package-wide deposit signature pre-verification cache.
// Lives at package level because the reader sits deep inside state transition.
var DepositSig = NewDepositSigCache()

// DepositSigCache stores per-deposit BLS verdicts keyed by execution_requests_root.
// Content-addressed reuse is safe because deposit signatures are fork-agnostic.
type DepositSigCache struct {
	cache *lru.Cache
	mu    sync.Mutex
}

func NewDepositSigCache() *DepositSigCache {
	return &DepositSigCache{cache: lruwrpr.New(depositSigCacheSize)}
}

// Get returns the verdict slice for root, parallel to ExecutionRequests.Deposits.
func (c *DepositSigCache) Get(root [32]byte) ([]bool, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.cache.Get(root)
	if !ok {
		depositSigCacheMiss.Inc()
		log.WithField("root", fmt.Sprintf("%#x", bytesutil.Trunc(root[:]))).Info("Deposit sig cache miss")
		return nil, false
	}
	depositSigCacheHit.Inc()
	log.WithField("root", fmt.Sprintf("%#x", bytesutil.Trunc(root[:]))).Info("Deposit sig cache hit")
	return v.([]bool), true
}

// Put stores the verdict slice for root. verifyDuration is how long the
// upstream BLS batch verify took. Callers must not mutate valid after.
func (c *DepositSigCache) Put(root [32]byte, valid []bool, verifyDuration time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache.Add(root, valid)
	verified := 0
	for _, v := range valid {
		if v {
			verified++
		}
	}
	log.WithFields(map[string]any{
		"root":     fmt.Sprintf("%#x", bytesutil.Trunc(root[:])),
		"deposits": len(valid),
		"verified": verified,
		"verifyMs": verifyDuration.Milliseconds(),
	}).Info("Wrote deposit sig verdict to cache")
}
