package das

import (
	"bytes"
	"reflect"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/db/filesystem"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
)

var (
	ErrDuplicateSidecar   = errors.New("duplicate sidecar stashed in AvailabilityStore")
	errIndexOutOfBounds   = errors.New("sidecar.index > MAX_BLOBS_PER_BLOCK")
	errCommitmentMismatch = errors.New("KzgCommitment of sidecar in cache did not match block commitment")
	errMissingSidecar     = errors.New("no sidecar in cache for block commitment")
)

// cacheKey includes the slot so that we can easily iterate through the cache and compare
// slots for eviction purposes. Whether the input is the block or the sidecar, we always have
// the root+slot when interacting with the cache, so it isn't an inconvenience to use both.
type cacheKey struct {
	slot primitives.Slot
	root [32]byte
}

type cache struct {
	entries map[cacheKey]*cacheEntry
}

func newCache() *cache {
	return &cache{entries: make(map[cacheKey]*cacheEntry)}
}

// keyFromSidecar is a convenience method for constructing a cacheKey from a BlobSidecar value.
func keyFromSidecar(sc blocks.ROBlob) cacheKey {
	return cacheKey{slot: sc.Slot(), root: sc.BlockRoot()}
}

func keyFromColumn(sc blocks.RODataColumn) cacheKey {
	return cacheKey{slot: sc.Slot(), root: sc.BlockRoot()}
}

// keyFromBlock is a convenience method for constructing a cacheKey from a ROBlock value.
func keyFromBlock(b blocks.ROBlock) cacheKey {
	return cacheKey{slot: b.Block().Slot(), root: b.Root()}
}

// ensure returns the entry for the given key, creating it if it isn't already present.
func (c *cache) ensure(key cacheKey) *cacheEntry {
	e, ok := c.entries[key]
	if !ok {
		e = &cacheEntry{}
		c.entries[key] = e
	}
	return e
}

// delete removes the cache entry from the cache.
func (c *cache) delete(key cacheKey) {
	delete(c.entries, key)
}

// cacheEntry holds a fixed-length cache of BlobSidecars.
type cacheEntry struct {
	scs         [fieldparams.MaxBlobsPerBlock]*blocks.ROBlob
	colScs      [fieldparams.NumberOfColumns]*blocks.RODataColumn
	diskSummary filesystem.BlobStorageSummary
}

func (e *cacheEntry) setDiskSummary(sum filesystem.BlobStorageSummary) {
	e.diskSummary = sum
}

// stash adds an item to the in-memory cache of BlobSidecars.
// Only the first BlobSidecar of a given Index will be kept in the cache.
// stash will return an error if the given blob is already in the cache, or if the Index is out of bounds.
func (e *cacheEntry) stash(sc *blocks.ROBlob) error {
	if sc.Index >= fieldparams.MaxBlobsPerBlock {
		return errors.Wrapf(errIndexOutOfBounds, "index=%d", sc.Index)
	}
	if e.scs[sc.Index] != nil {
		return errors.Wrapf(ErrDuplicateSidecar, "root=%#x, index=%d, commitment=%#x", sc.BlockRoot(), sc.Index, sc.KzgCommitment)
	}
	e.scs[sc.Index] = sc
	return nil
}

func (e *cacheEntry) stashColumns(sc *blocks.RODataColumn) error {
	if sc.ColumnIndex >= fieldparams.NumberOfColumns {
		return errors.Wrapf(errIndexOutOfBounds, "index=%d", sc.ColumnIndex)
	}
	if e.colScs[sc.ColumnIndex] != nil {
		return errors.Wrapf(ErrDuplicateSidecar, "root=%#x, index=%d, commitment=%#x", sc.BlockRoot(), sc.ColumnIndex, sc.KzgCommitments)
	}
	e.colScs[sc.ColumnIndex] = sc
	return nil
}

// filter evicts sidecars that are not committed to by the block and returns custom
// errors if the cache is missing any of the commitments, or if the commitments in
// the cache do not match those found in the block. If err is nil, then all expected
// commitments were found in the cache and the sidecar slice return value can be used
// to perform a DA check against the cached sidecars.
// filter only returns blobs that need to be checked. Blobs already available on disk will be excluded.
func (e *cacheEntry) filter(root [32]byte, kc safeCommitmentArray) ([]blocks.ROBlob, error) {
	if e.diskSummary.AllAvailable(kc.count()) {
		return nil, nil
	}
	scs := make([]blocks.ROBlob, 0, kc.count())
	for i := uint64(0); i < fieldparams.MaxBlobsPerBlock; i++ {
		// We already have this blob, we don't need to write it or validate it.
		if e.diskSummary.HasIndex(i) {
			continue
		}
		if kc[i] == nil {
			if e.scs[i] != nil {
				return nil, errors.Wrapf(errCommitmentMismatch, "root=%#x, index=%#x, commitment=%#x, no block commitment", root, i, e.scs[i].KzgCommitment)
			}
			continue
		}

		if e.scs[i] == nil {
			return nil, errors.Wrapf(errMissingSidecar, "root=%#x, index=%#x", root, i)
		}
		if !bytes.Equal(kc[i], e.scs[i].KzgCommitment) {
			return nil, errors.Wrapf(errCommitmentMismatch, "root=%#x, index=%#x, commitment=%#x, block commitment=%#x", root, i, e.scs[i].KzgCommitment, kc[i])
		}
		scs = append(scs, *e.scs[i])
	}

	return scs, nil
}

func (e *cacheEntry) filterColumns(root [32]byte, commitmentsArray *safeCommitmentsArray) ([]blocks.RODataColumn, error) {
	nonEmptyIndices := commitmentsArray.nonEmptyIndices()
	if e.diskSummary.AllDataColumnsAvailable(nonEmptyIndices) {
		return nil, nil
	}

	commitmentsCount := commitmentsArray.count()
	sidecars := make([]blocks.RODataColumn, 0, commitmentsCount)

	for i := uint64(0); i < fieldparams.NumberOfColumns; i++ {
		// Skip if we arleady store this data column.
		if e.diskSummary.HasIndex(i) {
			continue
		}

		if commitmentsArray[i] == nil {
			continue
		}

		if e.colScs[i] == nil {
			return nil, errors.Wrapf(errMissingSidecar, "root=%#x, index=%#x", root, i)
		}

		if !reflect.DeepEqual(commitmentsArray[i], e.colScs[i].KzgCommitments) {
			return nil, errors.Wrapf(errCommitmentMismatch, "root=%#x, index=%#x, commitment=%#x, block commitment=%#x", root, i, e.colScs[i].KzgCommitments, commitmentsArray[i])
		}

		sidecars = append(sidecars, *e.colScs[i])
	}

	return sidecars, nil
}

// safeCommitmentArray is a fixed size array of commitment byte slices. This is helpful for avoiding
// gratuitous bounds checks.
type safeCommitmentArray [fieldparams.MaxBlobsPerBlock][]byte

func (s safeCommitmentArray) count() int {
	for i := range s {
		if s[i] == nil {
			return i
		}
	}
	return fieldparams.MaxBlobsPerBlock
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
