package das

import (
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db/filesystem"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/pkg/errors"
)

var (
	errDuplicateSidecar   = errors.New("duplicate sidecar stashed in AvailabilityStore")
	errColumnIndexTooHigh = errors.New("column index too high")
	errCommitmentMismatch = errors.New("commitment of sidecar in cache did not match block commitment")
	errMissingSidecar     = errors.New("no sidecar in cache for block commitment")
)

type dataColumnCache struct {
	entries map[cacheKey]*dataColumnCacheEntry
}

func newDataColumnCache() *dataColumnCache {
	return &dataColumnCache{entries: make(map[cacheKey]*dataColumnCacheEntry)}
}

// entry returns the entry for the given key, creating it if it isn't already present.
func (c *dataColumnCache) entry(key cacheKey) *dataColumnCacheEntry {
	entry, ok := c.entries[key]
	if !ok {
		entry = newDataColumnCacheEntry(key.root)
		c.entries[key] = entry
	}

	return entry
}

func (c *dataColumnCache) cleanup(blks []blocks.ROBlock) {
	for _, block := range blks {
		key := cacheKey{slot: block.Block().Slot(), root: block.Root()}
		c.delete(key)
	}
}

// delete removes the cache entry from the cache.
func (c *dataColumnCache) delete(key cacheKey) {
	delete(c.entries, key)
}

func (c *dataColumnCache) stash(sc blocks.RODataColumn) error {
	key := cacheKey{slot: sc.Slot(), root: sc.BlockRoot()}
	entry := c.entry(key)
	return entry.stash(sc)
}

func newDataColumnCacheEntry(root [32]byte) *dataColumnCacheEntry {
	return &dataColumnCacheEntry{scs: make(map[uint64]blocks.RODataColumn), root: &root}
}

// dataColumnCacheEntry is the set of RODataColumns for a given block.
type dataColumnCacheEntry struct {
	root *[32]byte
	scs  map[uint64]blocks.RODataColumn
}

// stash adds an item to the in-memory cache of DataColumnSidecars.
// stash will return an error if the given data column Index is out of bounds.
// It will overwrite any existing entry for the same index.
func (e *dataColumnCacheEntry) stash(sc blocks.RODataColumn) error {
	if sc.Index >= fieldparams.NumberOfColumns {
		return errors.Wrapf(errColumnIndexTooHigh, "index=%d", sc.Index)
	}
	e.scs[sc.Index] = sc
	return nil
}

// append appends the requested root and indices from the cache to the given sidecars slice and returns the result.
// If any of the given indices are missing, an error will be returned and the sidecars slice will be unchanged.
func (e *dataColumnCacheEntry) append(sidecars []blocks.RODataColumn, indices peerdas.ColumnIndices) ([]blocks.RODataColumn, error) {
	needed := indices.ToMap()
	for col := range needed {
		_, ok := e.scs[col]
		if !ok {
			return nil, errors.Wrapf(errMissingSidecar, "root=%#x, index=%#x", e.root, col)
		}
	}
	// Loop twice so we can avoid touching the slice if any of the blobs are missing.
	for col := range needed {
		sidecars = append(sidecars, e.scs[col])
	}
	return sidecars, nil
}

// IndicesNotStored filters the list of indices to only include those that are not found in the storage summary.
func IndicesNotStored(sum filesystem.DataColumnStorageSummary, indices peerdas.ColumnIndices) peerdas.ColumnIndices {
	indices = indices.Copy()
	for col := range indices {
		if sum.HasIndex(col) {
			indices.Unset(col)
		}
	}
	return indices
}
