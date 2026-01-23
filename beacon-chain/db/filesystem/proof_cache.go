package filesystem

import (
	"slices"
	"sync"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
)

// ProofStorageSummary represents cached information about the proofs on disk for each root the cache knows about.
type ProofStorageSummary struct {
	epoch    primitives.Epoch
	proofIDs map[uint64]bool
}

// HasProof returns true if the proof with the given proofID is available in the filesystem.
func (s ProofStorageSummary) HasProof(proofID uint64) bool {
	if s.proofIDs == nil {
		return false
	}
	_, ok := s.proofIDs[proofID]
	return ok
}

// Count returns the number of available proofs.
func (s ProofStorageSummary) Count() int {
	return len(s.proofIDs)
}

// All returns all stored proofIDs sorted in ascending order.
func (s ProofStorageSummary) All() []uint64 {
	if s.proofIDs == nil {
		return nil
	}
	ids := make([]uint64, 0, len(s.proofIDs))
	for id := range s.proofIDs {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

type proofCache struct {
	mu                 sync.RWMutex
	proofCount         float64
	lowestCachedEpoch  primitives.Epoch
	highestCachedEpoch primitives.Epoch
	cache              map[[fieldparams.RootLength]byte]ProofStorageSummary
}

func newProofCache() *proofCache {
	return &proofCache{
		cache:             make(map[[fieldparams.RootLength]byte]ProofStorageSummary),
		lowestCachedEpoch: params.BeaconConfig().FarFutureEpoch,
	}
}

// Summary returns the ProofStorageSummary for `root`.
// The ProofStorageSummary can be used to check for the presence of proofs based on proofID.
func (pc *proofCache) Summary(root [fieldparams.RootLength]byte) ProofStorageSummary {
	pc.mu.RLock()
	defer pc.mu.RUnlock()

	return pc.cache[root]
}

// HighestEpoch returns the highest cached epoch.
func (pc *proofCache) HighestEpoch() primitives.Epoch {
	pc.mu.RLock()
	defer pc.mu.RUnlock()

	return pc.highestCachedEpoch
}

// set adds a proof to the cache.
func (pc *proofCache) set(ident ProofIdent) {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	summary := pc.cache[ident.BlockRoot]
	if summary.proofIDs == nil {
		summary.proofIDs = make(map[uint64]bool)
	}
	summary.epoch = ident.Epoch

	if _, exists := summary.proofIDs[ident.ProofID]; exists {
		pc.cache[ident.BlockRoot] = summary
		return
	}

	summary.proofIDs[ident.ProofID] = true
	pc.lowestCachedEpoch = min(pc.lowestCachedEpoch, ident.Epoch)
	pc.highestCachedEpoch = max(pc.highestCachedEpoch, ident.Epoch)

	pc.cache[ident.BlockRoot] = summary

	pc.proofCount++
	proofDiskCount.Set(pc.proofCount)
	proofWrittenCounter.Inc()
}

// setMultiple adds multiple proofs to the cache.
func (pc *proofCache) setMultiple(ident ProofsIdent) {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	summary := pc.cache[ident.BlockRoot]
	if summary.proofIDs == nil {
		summary.proofIDs = make(map[uint64]bool)
	}
	summary.epoch = ident.Epoch

	addedCount := 0
	for _, proofID := range ident.ProofIDs {
		if _, exists := summary.proofIDs[proofID]; exists {
			continue
		}
		summary.proofIDs[proofID] = true
		addedCount++
	}

	if addedCount == 0 {
		pc.cache[ident.BlockRoot] = summary
		return
	}

	pc.lowestCachedEpoch = min(pc.lowestCachedEpoch, ident.Epoch)
	pc.highestCachedEpoch = max(pc.highestCachedEpoch, ident.Epoch)

	pc.cache[ident.BlockRoot] = summary

	pc.proofCount += float64(addedCount)
	proofDiskCount.Set(pc.proofCount)
	proofWrittenCounter.Add(float64(addedCount))
}

// get returns the ProofStorageSummary for the given block root.
// If the root is not in the cache, the second return value will be false.
func (pc *proofCache) get(blockRoot [fieldparams.RootLength]byte) (ProofStorageSummary, bool) {
	pc.mu.RLock()
	defer pc.mu.RUnlock()

	summary, ok := pc.cache[blockRoot]
	return summary, ok
}

// evict removes the ProofStorageSummary for the given block root from the cache.
func (pc *proofCache) evict(blockRoot [fieldparams.RootLength]byte) int {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	summary, ok := pc.cache[blockRoot]
	if !ok {
		return 0
	}

	deleted := len(summary.proofIDs)
	delete(pc.cache, blockRoot)

	if deleted > 0 {
		pc.proofCount -= float64(deleted)
		proofDiskCount.Set(pc.proofCount)
	}

	return deleted
}

// pruneUpTo removes all entries from the cache up to the given target epoch included.
func (pc *proofCache) pruneUpTo(targetEpoch primitives.Epoch) uint64 {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	prunedCount := uint64(0)
	newLowestCachedEpoch := params.BeaconConfig().FarFutureEpoch
	newHighestCachedEpoch := primitives.Epoch(0)

	for blockRoot, summary := range pc.cache {
		epoch := summary.epoch

		if epoch > targetEpoch {
			newLowestCachedEpoch = min(newLowestCachedEpoch, epoch)
			newHighestCachedEpoch = max(newHighestCachedEpoch, epoch)
		}

		if epoch <= targetEpoch {
			prunedCount += uint64(len(summary.proofIDs))
			delete(pc.cache, blockRoot)
		}
	}

	if prunedCount > 0 {
		pc.lowestCachedEpoch = newLowestCachedEpoch
		pc.highestCachedEpoch = newHighestCachedEpoch
		pc.proofCount -= float64(prunedCount)
		proofDiskCount.Set(pc.proofCount)
	}

	return prunedCount
}

// clear removes all entries from the cache.
func (pc *proofCache) clear() uint64 {
	return pc.pruneUpTo(params.BeaconConfig().FarFutureEpoch)
}
