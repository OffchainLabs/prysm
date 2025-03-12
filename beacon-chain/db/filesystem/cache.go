package filesystem

import (
	"fmt"
	"sync"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/db"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
)

// dataIndexMask is a bitmask representing the set of blob or data column indices that are currently set.
type dataIndexMask []bool

// BlobStorageSummary represents cached information about the BlobSidecars or DataColumnSidecars on disk for each root the cache knows about.
type BlobStorageSummary struct {
	epoch primitives.Epoch
	mask  dataIndexMask
}

// HasIndex returns true if the BlobSidecar or DataColumnSidecar at the given index is available in the filesystem.
func (s BlobStorageSummary) HasIndex(idx uint64) bool {
	if idx >= uint64(len(s.mask)) {
		return false
	}
	return s.mask[idx]
}

// HasDataColumnIndex returns true if the DataColumnSidecar at the given index is available in the filesystem.
func (s BlobStorageSummary) HasDataColumnIndex(idx uint64) bool {
	// Protect from panic, but assume callers are sophisticated enough to not need an error telling them they have an invalid idx.
	numberOfColumns := params.BeaconConfig().NumberOfColumns
	if idx >= numberOfColumns {
		return false
	}

	if idx >= uint64(len(s.mask)) {
		return false
	}

	return s.mask[idx]
}

// AllAvailable returns true if we have all blobs for all indices from 0 to count-1.
func (s BlobStorageSummary) AllAvailable(count int) bool {
	if count > len(s.mask) {
		return false
	}
	for i := 0; i < count; i++ {
		if !s.mask[i] {
			return false
		}
	}
	return true
}

// AllDataColumnsAvailable returns true if we have all data columns for corresponding indices.
func (s BlobStorageSummary) AllDataColumnsAvailable(indices map[uint64]bool) bool {
	if len(indices) > len(s.mask) {
		return false
	}

	for indice := range indices {
		if !s.mask[indice] {
			return false
		}
	}

	return true
}

func (s BlobStorageSummary) MaxBlobsForEpoch() uint64 {
	return uint64(params.BeaconConfig().MaxBlobsPerBlockAtEpoch(s.epoch))
}

// NewBlobStorageSummary creates a new BlobStorageSummary for a given epoch and mask.
func NewBlobStorageSummary(epoch primitives.Epoch, mask []bool) (BlobStorageSummary, error) {
	c := params.BeaconConfig().MaxBlobsPerBlockAtEpoch(epoch)
	if len(mask) != c {
		return BlobStorageSummary{}, fmt.Errorf("mask length %d does not match expected %d for epoch %d", len(mask), c, epoch)
	}
	return BlobStorageSummary{
		epoch: epoch,
		mask:  mask,
	}, nil
}

// BlobStorageSummarizer can be used to receive a summary of metadata about blobs on disk for a given root.
// The BlobStorageSummary can be used to check which indices (if any) are available for a given block by root.
type BlobStorageSummarizer interface {
	Summary(root [32]byte) BlobStorageSummary
}

type blobStorageSummaryCache struct {
	mu     sync.RWMutex
	nBlobs float64
	cache  map[[32]byte]BlobStorageSummary
}

var _ BlobStorageSummarizer = &blobStorageSummaryCache{}

func newBlobStorageCache() *blobStorageSummaryCache {
	return &blobStorageSummaryCache{
		cache: make(map[[32]byte]BlobStorageSummary),
	}
}

// Summary returns the BlobStorageSummary for `root`. The BlobStorageSummary can be used to check for the presence of
// BlobSidecars based on Index.
func (s *blobStorageSummaryCache) Summary(root [32]byte) BlobStorageSummary {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cache[root]
}

func (s *blobStorageSummaryCache) ensure(ident blobIdent) error {
	maskSize := uint64(params.BeaconConfig().MaxBlobsPerBlockAtEpoch(ident.epoch))

	fuluForkEpoch := params.BeaconConfig().FuluForkEpoch
	if ident.epoch >= fuluForkEpoch {
		maskSize = params.BeaconConfig().NumberOfColumns
	}

	if ident.index >= maskSize {
		return errIndexOutOfBounds
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.cache[ident.root]
	v.epoch = ident.epoch
	if v.mask == nil {
		v.mask = make(dataIndexMask, maskSize)
	}
	if !v.mask[ident.index] {
		s.updateMetrics(1)
	}
	v.mask[ident.index] = true
	s.cache[ident.root] = v
	return nil
}

func (s *blobStorageSummaryCache) get(key [32]byte) (BlobStorageSummary, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.cache[key]
	return v, ok
}

func (s *blobStorageSummaryCache) identForIdx(key [32]byte, idx uint64) (blobIdent, error) {
	v, ok := s.get(key)
	if !ok || !v.HasIndex(idx) {
		return blobIdent{}, db.ErrNotFound
	}
	return blobIdent{
		root:  key,
		index: idx,
		epoch: v.epoch,
	}, nil
}

func (s *blobStorageSummaryCache) identForRoot(key [32]byte) (blobIdent, error) {
	v, ok := s.get(key)
	if !ok {
		return blobIdent{}, db.ErrNotFound
	}
	return blobIdent{
		root:  key,
		epoch: v.epoch,
	}, nil
}

func (s *blobStorageSummaryCache) evict(key [32]byte) int {
	deleted := 0
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.cache[key]
	if !ok {
		return 0
	}
	for i := range v.mask {
		if v.mask[i] {
			deleted += 1
		}
	}
	delete(s.cache, key)
	if deleted > 0 {
		s.updateMetrics(-float64(deleted))
	}
	return deleted
}

func (s *blobStorageSummaryCache) updateMetrics(delta float64) {
	s.nBlobs += delta
	blobDiskCount.Set(s.nBlobs)
	blobDiskSize.Set(s.nBlobs * fieldparams.BlobSidecarSize)
}
