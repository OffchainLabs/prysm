package blst

import "github.com/OffchainLabs/prysm/v7/crypto/bls/common"

// Note: These functions are for tests to access private globals, such as pubkeyCache.

// DisableCaches clears the pubkey cache.
func DisableCaches() {
	pubkeyCache.mu.Lock()
	pubkeyCache.items = make(map[[48]byte]common.PublicKey)
	pubkeyCache.mu.Unlock()
}

// EnableCaches is a no-op since the map-based cache has no size limit.
func EnableCaches() {}
