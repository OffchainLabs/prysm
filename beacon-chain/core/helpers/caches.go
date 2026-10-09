package helpers

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
)

type caches struct {
	committee *cache.CommitteeCache
	balance   *cache.BalanceCache
	sync      *cache.SyncCommitteeCache
}

type cachesKey struct{}

// WithIsolatedCaches returns a context under which the committee, total active balance and sync committee caches
// are private to the caller instead of shared with the rest of the node.
func WithIsolatedCaches(ctx context.Context) context.Context {
	return context.WithValue(ctx, cachesKey{}, &caches{
		committee: cache.NewCommitteesCache(),
		balance:   cache.NewEffectiveBalanceCache(),
		sync:      cache.NewSyncCommittee(),
	})
}

func committeeCacheFrom(ctx context.Context) *cache.CommitteeCache {
	if c, ok := ctx.Value(cachesKey{}).(*caches); ok {
		return c.committee
	}
	return committeeCache
}

func balanceCacheFrom(ctx context.Context) *cache.BalanceCache {
	if c, ok := ctx.Value(cachesKey{}).(*caches); ok {
		return c.balance
	}
	return balanceCache
}

func syncCommitteeCacheFrom(ctx context.Context) *cache.SyncCommitteeCache {
	if c, ok := ctx.Value(cachesKey{}).(*caches); ok {
		return c.sync
	}
	return syncCommitteeCache
}
