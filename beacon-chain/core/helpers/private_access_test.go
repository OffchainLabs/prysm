//go:build !fuzz

package helpers

import "github.com/OffchainLabs/prysm/v7/beacon-chain/cache"

func CommitteeCache() *cache.CommitteeCache {
	return committeeCache
}

func SyncCommitteeCache() *cache.SyncCommitteeCache {
	return syncCommitteeCache
}

func ProposerIndicesCache() *cache.ProposerIndicesCache {
	return proposerIndicesCache
}
