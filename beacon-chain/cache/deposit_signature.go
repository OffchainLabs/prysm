package cache

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Roughly 40MB at 80 bytes per entry.
const depositSignatureCacheCap = 1 << 19

var (
	depositSignatureCacheHit = promauto.NewCounter(prometheus.CounterOpts{
		Name: "deposit_signature_cache_hit_total",
		Help: "Total cache hits on the deposit signature verification cache.",
	})
	depositSignatureCacheMiss = promauto.NewCounter(prometheus.CounterOpts{
		Name: "deposit_signature_cache_miss_total",
		Help: "Total cache misses on the deposit signature verification cache.",
	})
	depositSignatureCacheSize = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "deposit_signature_cache_size",
		Help: "Number of entries in the deposit signature verification cache.",
	})
	depositSignatureCacheRejected = promauto.NewCounter(prometheus.CounterOpts{
		Name: "deposit_signature_cache_rejected_total",
		Help: "Total deposit signature results not cached because the cache is at capacity.",
	})
)

// DepositSignature memoizes deposit signature verification results, which never expire.
var DepositSignature = newDepositSignatureCache()

type DepositSignatureCache struct {
	mu      sync.RWMutex
	entries map[[32]byte]bool
}

func newDepositSignatureCache() *DepositSignatureCache {
	return &DepositSignatureCache{entries: make(map[[32]byte]bool)}
}

// An absent key means unknown, never invalid.
func (c *DepositSignatureCache) Get(key [32]byte) (bool, bool) {
	c.mu.RLock()
	valid, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok {
		depositSignatureCacheMiss.Inc()
		return false, false
	}
	depositSignatureCacheHit.Inc()
	return valid, true
}

// Has does not record a hit or a miss, so pre-verification cannot skew the critical-path hit rate.
func (c *DepositSignatureCache) Has(key [32]byte) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.entries[key]
	return ok
}

func (c *DepositSignatureCache) Put(key [32]byte, valid bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.entries[key]; !ok && len(c.entries) >= depositSignatureCacheCap {
		depositSignatureCacheRejected.Inc()
		return
	}
	c.entries[key] = valid
	depositSignatureCacheSize.Set(float64(len(c.entries)))
}

func (c *DepositSignatureCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

func (c *DepositSignatureCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[[32]byte]bool)
	depositSignatureCacheSize.Set(0)
}
