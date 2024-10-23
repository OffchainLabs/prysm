package das

import (
	"context"
	"fmt"

	"github.com/ethereum/go-ethereum/p2p/enode"
	errors "github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/peerdas"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/db/filesystem"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
	log "github.com/sirupsen/logrus"
)

// LazilyPersistentStoreColumn is an implementation of AvailabilityStore to be used when batch syncing data columns.
// This implementation will hold any blobs passed to Persist until the IsDataAvailable is called for their
// block, at which time they will undergo full verification and be saved to the disk.
type LazilyPersistentStoreColumn struct {
	store *filesystem.BlobStorage
	cache *cache
}

func NewLazilyPersistentStoreColumn(store *filesystem.BlobStorage) *LazilyPersistentStoreColumn {
	return &LazilyPersistentStoreColumn{
		store: store,
		cache: newCache(),
	}
}

// Persist do nothing at the moment.
// TODO: Very Ugly, change interface to allow for columns and blobs
func (*LazilyPersistentStoreColumn) Persist(_ primitives.Slot, _ ...blocks.ROBlob) error {
	return nil
}

// PersistColumns adds columns to the working column cache. columns stored in this cache will be persisted
// for at least as long as the node is running. Once IsDataAvailable succeeds, all blobs referenced
// by the given block are guaranteed to be persisted for the remainder of the retention period.
func (s *LazilyPersistentStoreColumn) PersistColumns(current primitives.Slot, sc ...blocks.RODataColumn) error {
	if len(sc) == 0 {
		return nil
	}
	if len(sc) > 1 {
		first := sc[0].BlockRoot()
		for i := 1; i < len(sc); i++ {
			if first != sc[i].BlockRoot() {
				return errMixedRoots
			}
		}
	}
	if !params.WithinDAPeriod(slots.ToEpoch(sc[0].Slot()), slots.ToEpoch(current)) {
		return nil
	}
	key := keyFromColumn(sc[0])
	entry := s.cache.ensure(key)
	for i := range sc {
		if err := entry.stashColumns(&sc[i]); err != nil {
			return err
		}
	}
	return nil
}

// IsDataAvailable returns nil if all the commitments in the given block are persisted to the db and have been verified.
// BlobSidecars already in the db are assumed to have been previously verified against the block.
func (s *LazilyPersistentStoreColumn) IsDataAvailable(
	ctx context.Context,
	nodeID enode.ID,
	currentSlot primitives.Slot,
	block blocks.ROBlock,
) error {
	blockCommitments, err := fullCommitmentsToCheck(nodeID, block, currentSlot)
	if err != nil {
		return errors.Wrapf(err, "full commitments to check with block root `%#x` and current slot `%d`", block.Root(), currentSlot)
	}

	// Return early for blocks that do not have any commitments.
	if blockCommitments.count() == 0 {
		return nil
	}

	// Build the cache key for the block.
	key := keyFromBlock(block)

	// Retrieve the cache entry for the block, or create an empty one if it doesn't exist.
	entry := s.cache.ensure(key)

	// Delete the cache entry for the block at the end.
	defer s.cache.delete(key)

	// Get the root of the block.
	blockRoot := block.Root()

	// Wait for the summarizer to be ready before proceeding.
	summarizer, err := s.store.WaitForSummarizer(ctx)
	if err != nil {
		log.
			WithField("root", fmt.Sprintf("%#x", blockRoot)).
			WithError(err).
			Debug("Failed to receive BlobStorageSummarizer within IsDataAvailable")
	} else {
		// Get the summary for the block, and set it in the cache entry.
		summary := summarizer.Summary(blockRoot)
		entry.setDiskSummary(summary)
	}

	// Verify we have all the expected sidecars, and fail fast if any are missing or inconsistent.
	// We don't try to salvage problematic batches because this indicates a misbehaving peer and we'd rather
	// ignore their response and decrease their peer score.
	roDataColumns, err := entry.filterColumns(blockRoot, blockCommitments)
	if err != nil {
		return errors.Wrap(err, "incomplete BlobSidecar batch")
	}

	// Create verified RO data columns from RO data columns.
	verifiedRODataColumns := make([]blocks.VerifiedRODataColumn, 0, len(roDataColumns))

	for _, roDataColumn := range roDataColumns {
		verifiedRODataColumn := blocks.NewVerifiedRODataColumn(roDataColumn)
		verifiedRODataColumns = append(verifiedRODataColumns, verifiedRODataColumn)
	}

	// Ensure that each column sidecar is written to disk.
	for _, verifiedRODataColumn := range verifiedRODataColumns {
		if err := s.store.SaveDataColumn(verifiedRODataColumn); err != nil {
			return errors.Wrapf(err, "save data columns for index `%d` for block `%#x`", verifiedRODataColumn.ColumnIndex, blockRoot)
		}
	}

	// All ColumnSidecars are persisted - data availability check succeeds.
	return nil
}

// fullCommitmentsToCheck returns the commitments to check for a given block.
func fullCommitmentsToCheck(nodeID enode.ID, block blocks.ROBlock, currentSlot primitives.Slot) (*safeCommitmentsArray, error) {
	// Return early for blocks that are pre-deneb.
	if block.Version() < version.Deneb {
		return &safeCommitmentsArray{}, nil
	}

	// Compute the block epoch.
	blockSlot := block.Block().Slot()
	blockEpoch := slots.ToEpoch(blockSlot)

	// Compute the current spoch.
	currentEpoch := slots.ToEpoch(currentSlot)

	// Return early if the request is out of the MIN_EPOCHS_FOR_DATA_COLUMN_SIDECARS_REQUESTS window.
	if !params.WithinDAPeriod(blockEpoch, currentEpoch) {
		return &safeCommitmentsArray{}, nil
	}

	// Retrieve the KZG commitments for the block.
	kzgCommitments, err := block.Block().Body().BlobKzgCommitments()
	if err != nil {
		return nil, errors.Wrap(err, "blob KZG commitments")
	}

	// Return early if there are no commitments in the block.
	if len(kzgCommitments) == 0 {
		return &safeCommitmentsArray{}, nil
	}

	// Retrieve the custody columns.
	custodySubnetCount := peerdas.CustodySubnetCount()
	custodyColumns, err := peerdas.CustodyColumns(nodeID, custodySubnetCount)
	if err != nil {
		return nil, errors.Wrap(err, "custody columns")
	}

	// Create a safe commitments array for the custody columns.
	commitmentsArray := &safeCommitmentsArray{}
	for column := range custodyColumns {
		commitmentsArray[column] = kzgCommitments
	}

	return commitmentsArray, nil
}
