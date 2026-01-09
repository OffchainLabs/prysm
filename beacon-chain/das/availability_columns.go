package das

import (
	"context"
	"io"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db/filesystem"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/ethereum/go-ethereum/p2p/enode"
	errors "github.com/pkg/errors"
)

// LazilyPersistentStoreColumn is an implementation of AvailabilityStore to be used when batch syncing data columns.
// This implementation will hold any data columns passed to Persist until the IsDataAvailable is called for their
// block, at which time they will undergo full verification and be saved to the disk.
type LazilyPersistentStoreColumn struct {
	store                  *filesystem.DataColumnStorage
	cache                  *dataColumnCache
	newDataColumnsVerifier verification.NewDataColumnsVerifier
	custody                *custodyRequirement
	bisector               Bisector
	shouldRetain           RetentionChecker
}

var _ AvailabilityChecker = &LazilyPersistentStoreColumn{}

// DataColumnsVerifier enables LazilyPersistentStoreColumn to manage the verification process
// going from RODataColumn->VerifiedRODataColumn, while avoiding the decision of which individual verifications
// to run and in what order. Since LazilyPersistentStoreColumn always tries to verify and save data columns only when
// they are all available, the interface takes a slice of data column sidecars.
type DataColumnsVerifier interface {
	VerifiedRODataColumns(ctx context.Context, blk blocks.ROBlock, scs []blocks.RODataColumn) ([]blocks.VerifiedRODataColumn, error)
}

// NewLazilyPersistentStoreColumn creates a new LazilyPersistentStoreColumn.
// WARNING: The resulting LazilyPersistentStoreColumn is NOT thread-safe.
func NewLazilyPersistentStoreColumn(
	store *filesystem.DataColumnStorage,
	newDataColumnsVerifier verification.NewDataColumnsVerifier,
	nodeID enode.ID,
	cgc uint64,
	bisector Bisector,
	shouldRetain RetentionChecker,
) *LazilyPersistentStoreColumn {
	return &LazilyPersistentStoreColumn{
		store:                  store,
		cache:                  newDataColumnCache(),
		newDataColumnsVerifier: newDataColumnsVerifier,
		custody:                &custodyRequirement{nodeID: nodeID, cgc: cgc},
		bisector:               bisector,
		shouldRetain:           shouldRetain,
	}
}

// PersistColumns adds columns to the working column cache. Columns stored in this cache will be persisted
// for at least as long as the node is running. Once IsDataAvailable succeeds, all columns referenced
// by the given block are guaranteed to be persisted for the remainder of the retention period.
func (s *LazilyPersistentStoreColumn) Persist(_ primitives.Slot, sidecars ...blocks.RODataColumn) error {
	for _, sidecar := range sidecars {
		if err := s.cache.stash(sidecar); err != nil {
			return errors.Wrap(err, "stash DataColumnSidecar")
		}
	}

	return nil
}

// IsDataAvailable returns nil if all the commitments in the given block are persisted to the db and have been verified.
// DataColumnsSidecars already in the db are assumed to have been previously verified against the block.
func (s *LazilyPersistentStoreColumn) IsDataAvailable(ctx context.Context, _ primitives.Slot, blks ...blocks.ROBlock) error {
	toVerify := make([]blocks.RODataColumn, 0)
	for _, block := range blks {
		indices, err := s.required(block)
		if err != nil {
			return errors.Wrapf(err, "full commitments to check with block root `%#x`", block.Root())
		}
		if indices.Count() == 0 {
			continue
		}

		key := keyFromBlock(block)
		entry := s.cache.entry(key)
		toVerify, err = entry.append(toVerify, IndicesNotStored(s.store.Summary(block.Root()), indices))
		if err != nil {
			return errors.Wrap(err, "entry filter")
		}
	}

	if err := s.verifyAndSave(toVerify); err != nil {
		log.Warn("Batch verification failed, bisecting columns by peer")
		if err := s.bisectVerification(toVerify); err != nil {
			return errors.Wrap(err, "bisect verification")
		}
	}

	s.cache.cleanup(blks)
	return nil
}

// required returns the set of column indices to check for a given block.
func (s *LazilyPersistentStoreColumn) required(block blocks.ROBlock) (peerdas.ColumnIndices, error) {
	if !s.shouldRetain(block.Block().Slot()) {
		return peerdas.NewColumnIndices(), nil
	}

	// If there are any commitments in the block, there are blobs,
	// and if there are blobs, we need the columns bisecting those blobs.
	commitments, err := block.Block().Body().BlobKzgCommitments()
	if err != nil {
		return nil, errors.Wrap(err, "blob KZG commitments")
	}
	// No DA check needed if the block has no blobs.
	if len(commitments) == 0 {
		return peerdas.NewColumnIndices(), nil
	}

	return s.custody.required()
}

// verifyAndSave calls Save on the column store if the columns pass verification.
func (s *LazilyPersistentStoreColumn) verifyAndSave(columns []blocks.RODataColumn) error {
	if len(columns) == 0 {
		return nil
	}
	verified, err := s.verifyColumns(columns)
	if err != nil {
		return errors.Wrap(err, "verify columns")
	}
	if err := s.store.Save(verified); err != nil {
		return errors.Wrap(err, "save data column sidecars")
	}

	return nil
}

func (s *LazilyPersistentStoreColumn) verifyColumns(columns []blocks.RODataColumn) ([]blocks.VerifiedRODataColumn, error) {
	if len(columns) == 0 {
		return nil, nil
	}
	verifier := s.newDataColumnsVerifier(columns, verification.ByRangeRequestDataColumnSidecarRequirements)
	if err := verifier.ValidFields(); err != nil {
		return nil, errors.Wrap(err, "valid fields")
	}
	if err := verifier.SidecarInclusionProven(); err != nil {
		return nil, errors.Wrap(err, "sidecar inclusion proven")
	}
	if err := verifier.SidecarKzgProofVerified(); err != nil {
		return nil, errors.Wrap(err, "sidecar KZG proof verified")
	}

	return verifier.VerifiedRODataColumns()
}

// bisectVerification is used when verification of a batch of columns fails. Since the batch could
// span multiple blocks or have been fetched from multiple peers, this pattern enables code using the
// store to break the verification into smaller units and learn the results, in order to plan to retry
// retrieval of the unusable columns.
func (s *LazilyPersistentStoreColumn) bisectVerification(columns []blocks.RODataColumn) error {
	if len(columns) == 0 {
		return nil
	}
	if s.bisector == nil {
		return errors.New("bisector not initialized")
	}
	iter, err := s.bisector.Bisect(columns)
	if err != nil {
		return errors.Wrap(err, "Bisector.Bisect")
	}
	// It's up to the bisector how to chunk up columns for verification,
	// which could be by block, or by peer, or any other strategy.
	// For the purposes of range syncing or backfill this will be by peer,
	// so that the node can learn which peer is giving us bad data and downscore them.
	for columns, err := iter.Next(); columns != nil; columns, err = iter.Next() {
		if err != nil {
			if !errors.Is(err, io.EOF) {
				return errors.Wrap(err, "Bisector.Next")
			}
			break // io.EOF signals end of iteration
		}
		// We save the parts of the batch that have been verified successfully even though we don't know
		// if all columns for the block will be available until the block is imported.
		if err := s.verifyAndSave(s.columnsNotStored(columns)); err != nil {
			iter.OnError(err)
			continue
		}
	}
	// This should give us a single error representing any unresolved errors seen via onError.
	return iter.Error()
}

// columnsNotStored filters the list of ROColumnSidecars to only include those that are not found in the storage summary.
func (s *LazilyPersistentStoreColumn) columnsNotStored(sidecars []blocks.RODataColumn) []blocks.RODataColumn {
	// We use this method to filter a set of sidecars that were previously seen to be unavailable on disk. So our base assumption
	// is that they are still available and we don't need to copy the list. Instead we make a slice of any indices that are unexpectedly
	// stored and only when we find that the storage view has changed do we need to create a new slice.
	stored := make(map[int]struct{}, 0)
	lastRoot := [32]byte{}
	var sum filesystem.DataColumnStorageSummary
	for i, sc := range sidecars {
		if sc.BlockRoot() != lastRoot {
			sum = s.store.Summary(sc.BlockRoot())
			lastRoot = sc.BlockRoot()
		}
		if sum.HasIndex(sc.Index) {
			stored[i] = struct{}{}
		}
	}
	// If the view on storage hasn't changed, return the original list.
	if len(stored) == 0 {
		return sidecars
	}
	shift := 0
	for i := range sidecars {
		if _, ok := stored[i]; ok {
			// If the index is stored, skip and overwrite it.
			// Track how many spaces down to shift unseen sidecars (to overwrite the previously shifted or seen).
			shift++
			continue
		}
		if shift > 0 {
			// If the index is not stored and we have seen stored indices,
			// we need to shift the current index down.
			sidecars[i-shift] = sidecars[i]
		}
	}
	return sidecars[:len(sidecars)-shift]
}

type custodyRequirement struct {
	nodeID  enode.ID
	cgc     uint64 // custody group count
	indices peerdas.ColumnIndices
}

func (c *custodyRequirement) required() (peerdas.ColumnIndices, error) {
	peerInfo, _, err := peerdas.Info(c.nodeID, c.cgc)
	if err != nil {
		return peerdas.NewColumnIndices(), errors.Wrap(err, "peer info")
	}
	return peerdas.NewColumnIndicesFromMap(peerInfo.CustodyColumns), nil
}
