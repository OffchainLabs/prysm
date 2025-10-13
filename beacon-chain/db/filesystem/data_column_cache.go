package filesystem

import (
	"sync"

	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/pkg/errors"
)

var errDataColumnIndexOutOfBounds = errors.New("data column index too high")

// DataColumnStorageSummary represents cached information about the DataColumnSidecars on disk for each root the cache knows about.
type DataColumnStorageSummary struct {
	epoch primitives.Epoch
	mask  [fieldparams.NumberOfColumns]bool
}

// NewDataColumnStorageSummary creates a new DataColumnStorageSummary for a given epoch and mask.
func NewDataColumnStorageSummary(epoch primitives.Epoch, mask [fieldparams.NumberOfColumns]bool) DataColumnStorageSummary {
	return DataColumnStorageSummary{
		epoch: epoch,
		mask:  mask,
	}
}

// HasIndex returns true if the DataColumnSidecar at the given index is available in the filesystem.
func (s DataColumnStorageSummary) HasIndex(index uint64) bool {
	if index >= uint64(fieldparams.NumberOfColumns) {
		return false
	}
	return s.mask[index]
}

// HasAtLeastOneIndex returns true if at least one of the DataColumnSidecars at the given indices is available in the filesystem.
func (s DataColumnStorageSummary) HasAtLeastOneIndex(indices []uint64) bool {
	size := uint64(len(s.mask))
	for _, index := range indices {
		if index < size && s.mask[index] {
			return true
		}
	}

	return false
}

// Count returns the number of available data columns.
func (s DataColumnStorageSummary) Count() uint64 {
	count := uint64(0)

	for _, available := range s.mask {
		if available {
			count++
		}
	}

	return count
}

// AllAvailable returns true if we have all data columns for corresponding indices.
func (s DataColumnStorageSummary) AllAvailable(indices map[uint64]bool) bool {
	if len(indices) > len(s.mask) {
		return false
	}

	for index := range indices {
		if !s.mask[index] {
			return false
		}
	}

	return true
}

// Stored returns a map of all stored data columns.
func (s DataColumnStorageSummary) Stored() map[uint64]bool {
	stored := make(map[uint64]bool, fieldparams.NumberOfColumns)
	for index, exists := range s.mask {
		if exists {
			stored[uint64(index)] = true
		}
	}

	return stored
}

type dataColumnStorageSummaryCache struct {
	mu                 sync.RWMutex
	dataColumnCount    float64
	lowestCachedEpoch  primitives.Epoch
	highestCachedEpoch primitives.Epoch
	cache              map[[fieldparams.RootLength]byte]DataColumnStorageSummary
}

func newDataColumnStorageSummaryCache() *dataColumnStorageSummaryCache {
	return &dataColumnStorageSummaryCache{
		cache:             make(map[[fieldparams.RootLength]byte]DataColumnStorageSummary),
		lowestCachedEpoch: params.BeaconConfig().FarFutureEpoch,
	}
}

// Summary returns the DataColumnStorageSummary for `root`.
// The DataColumnStorageSummary can be used to check for the presence of DataColumnSidecars based on Index.
func (sc *dataColumnStorageSummaryCache) Summary(root [fieldparams.RootLength]byte) DataColumnStorageSummary {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	return sc.cache[root]
}

func (sc *dataColumnStorageSummaryCache) HighestEpoch() primitives.Epoch {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.highestCachedEpoch
}

// set updates the cache.
func (sc *dataColumnStorageSummaryCache) set(dataColumnsIdent DataColumnsIdent) error {
	numberOfColumns := params.BeaconConfig().NumberOfColumns

	sc.mu.Lock()
	defer sc.mu.Unlock()

	summary := sc.cache[dataColumnsIdent.Root]
	summary.epoch = dataColumnsIdent.Epoch

	count := uint64(0)
	for _, index := range dataColumnsIdent.Indices {
		if index >= numberOfColumns {
			return errDataColumnIndexOutOfBounds
		}

		if summary.mask[index] {
			continue
		}

		count++

		summary.mask[index] = true
		sc.lowestCachedEpoch = min(sc.lowestCachedEpoch, dataColumnsIdent.Epoch)
		sc.highestCachedEpoch = max(sc.highestCachedEpoch, dataColumnsIdent.Epoch)
	}

	sc.cache[dataColumnsIdent.Root] = summary

	countFloat := float64(count)
	sc.dataColumnCount += countFloat
	dataColumnDiskCount.Set(sc.dataColumnCount)
	dataColumnWrittenCounter.Add(countFloat)

	return nil
}

// get returns the DataColumnStorageSummary for the given block root.
// If the root is not in the cache, the second return value will be false.
func (sc *dataColumnStorageSummaryCache) get(blockRoot [fieldparams.RootLength]byte) (DataColumnStorageSummary, bool) {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	v, ok := sc.cache[blockRoot]
	return v, ok
}

// evict removes the DataColumnStorageSummary for the given block root from the cache.
func (s *dataColumnStorageSummaryCache) evict(blockRoot [fieldparams.RootLength]byte) int {
	deleted := 0

	s.mu.Lock()
	defer s.mu.Unlock()

	summary, ok := s.cache[blockRoot]
	if !ok {
		return 0
	}

	for i := range summary.mask {
		if summary.mask[i] {
			deleted += 1
		}
	}

	delete(s.cache, blockRoot)
	if deleted > 0 {
		s.dataColumnCount -= float64(deleted)
		dataColumnDiskCount.Set(s.dataColumnCount)
	}

	// The lowest and highest cached epoch may no longer be valid here,
	// but is not worth the effort to recalculate.

	return deleted
}

// pruneUpTo removes all entries from the cache up to the given target epoch included.
func (sc *dataColumnStorageSummaryCache) pruneUpTo(targetEpoch primitives.Epoch) uint64 {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	prunedCount := uint64(0)
	newLowestCachedEpoch := params.BeaconConfig().FarFutureEpoch
	newHighestCachedEpoch := primitives.Epoch(0)

	for blockRoot, summary := range sc.cache {
		epoch := summary.epoch

		if epoch > targetEpoch {
			newLowestCachedEpoch = min(newLowestCachedEpoch, epoch)
			newHighestCachedEpoch = max(newHighestCachedEpoch, epoch)
		}

		if epoch <= targetEpoch {
			for i := range summary.mask {
				if summary.mask[i] {
					prunedCount += 1
				}
			}

			delete(sc.cache, blockRoot)
		}
	}

	if prunedCount > 0 {
		sc.lowestCachedEpoch = newLowestCachedEpoch
		sc.highestCachedEpoch = newHighestCachedEpoch
		sc.dataColumnCount -= float64(prunedCount)
		dataColumnDiskCount.Set(sc.dataColumnCount)
	}

	return prunedCount
}

// clear removes all entries from the cache.
func (sc *dataColumnStorageSummaryCache) clear() uint64 {
	return sc.pruneUpTo(params.BeaconConfig().FarFutureEpoch)
}
