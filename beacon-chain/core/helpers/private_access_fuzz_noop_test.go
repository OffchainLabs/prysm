//go:build fuzz

package helpers

import "github.com/OffchainLabs/prysm/v7/beacon-chain/cache"

func CommitteeCache() *cache.FakeCommitteeCache {
	return committeeCache
}

func SyncCommitteeCache() *cache.FakeSyncCommitteeCache {
	return syncCommitteeCache
}

func ProposerIndicesCache() *cache.FakeProposerIndicesCache {
	return proposerIndicesCache
}
