package das

import (
	"bytes"
	"slices"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/db/filesystem"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/pkg/errors"
)

var (
	ErrDuplicateSidecar   = errors.New("duplicate sidecar stashed in AvailabilityStore")
	errColumnIndexTooHigh = errors.New("column index too high")
	errCommitmentMismatch = errors.New("KzgCommitment of sidecar in cache did not match block commitment")
	errMissingSidecar     = errors.New("no sidecar in cache for block commitment")
)

type dataColumnCache struct {
	entries map[cacheKey]*dataColumnCacheEntry
}

func newDataColumnCache() *dataColumnCache {
	return &dataColumnCache{entries: make(map[cacheKey]*dataColumnCacheEntry)}
}

// ensure returns the entry for the given key, creating it if it isn't already present.
func (c *dataColumnCache) ensure(key cacheKey) *dataColumnCacheEntry {
	entry, ok := c.entries[key]
	if !ok {
		entry = &dataColumnCacheEntry{}
		c.entries[key] = entry
	}

	return entry
}

// delete removes the cache entry from the cache.
func (c *dataColumnCache) delete(key cacheKey) {
	delete(c.entries, key)
}

// dataColumnCacheEntry holds a fixed-length cache of BlobSidecars.
type dataColumnCacheEntry struct {
	scs         [fieldparams.NumberOfColumns]*blocks.RODataColumn
	diskSummary filesystem.DataColumnStorageSummary
}

func (e *dataColumnCacheEntry) setDiskSummary(sum filesystem.DataColumnStorageSummary) {
	e.diskSummary = sum
}

// stash adds an item to the in-memory cache of DataColumnSidecars.
// Only the first DataColumnSidecar of a given Index will be kept in the cache.
// stash will return an error if the given data colunn is already in the cache, or if the Index is out of bounds.
func (e *dataColumnCacheEntry) stash(sc *blocks.RODataColumn) error {
	if sc.Index >= fieldparams.NumberOfColumns {
		return errors.Wrapf(errColumnIndexTooHigh, "index=%d", sc.Index)
	}

	if e.scs[sc.Index] != nil {
		return errors.Wrapf(ErrDuplicateSidecar, "root=%#x, index=%d, commitment=%#x", sc.BlockRoot(), sc.Index, sc.KzgCommitments)
	}

	e.scs[sc.Index] = sc

	return nil
}

func (e *dataColumnCacheEntry) filter(root [32]byte, commitmentsArray *safeCommitmentsArray) ([]blocks.RODataColumn, error) {
	nonEmptyIndices := commitmentsArray.nonEmptyIndices()
	if e.diskSummary.AllAvailable(nonEmptyIndices) {
		return nil, nil
	}

	commitmentsCount := commitmentsArray.count()
	sidecars := make([]blocks.RODataColumn, 0, commitmentsCount)

	for i := range nonEmptyIndices {
		if e.diskSummary.HasIndex(i) {
			continue
		}

		if e.scs[i] == nil {
			return nil, errors.Wrapf(errMissingSidecar, "root=%#x, index=%#x", root, i)
		}

		if !sliceBytesEqual(commitmentsArray[i], e.scs[i].KzgCommitments) {
			return nil, errors.Wrapf(errCommitmentMismatch, "root=%#x, index=%#x, commitment=%#x, block commitment=%#x", root, i, e.scs[i].KzgCommitments, commitmentsArray[i])
		}

		sidecars = append(sidecars, *e.scs[i])
	}

	return sidecars, nil
}

// safeCommitmentsArray is a fixed size array of commitments.
// This is helpful for avoiding gratuitous bounds checks.
type safeCommitmentsArray [fieldparams.NumberOfColumns][][]byte

// count returns the number of commitments in the array.
func (s *safeCommitmentsArray) count() int {
	count := 0

	for i := range s {
		if s[i] != nil {
			count++
		}
	}

	return count
}

// nonEmptyIndices returns a map of indices that are non-nil in the array.
func (s *safeCommitmentsArray) nonEmptyIndices() map[uint64]bool {
	columns := make(map[uint64]bool)

	for i := range s {
		if s[i] != nil {
			columns[uint64(i)] = true
		}
	}

	return columns
}

func sliceBytesEqual(a, b [][]byte) bool {
	return slices.EqualFunc(a, b, bytes.Equal)
}
