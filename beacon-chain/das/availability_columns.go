package das

import (
	"context"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db/filesystem"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/verification"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/ethereum/go-ethereum/p2p/enode"
	errors "github.com/pkg/errors"
)

// LazilyPersistentStoreColumn is an implementation of AvailabilityStore to be used when batch syncing data columns.
// This implementation will hold any data columns passed to Persist until the IsDataAvailable is called for their
// block, at which time they will undergo full verification and be saved to the disk.
type LazilyPersistentStoreColumn struct {
	store                  *filesystem.DataColumnStorage
	nodeID                 enode.ID
	cache                  *dataColumnCache
	custodyInfo            *peerdas.CustodyInfo
	newDataColumnsVerifier verification.NewDataColumnsVerifier
}

var _ AvailabilityStore = &LazilyPersistentStoreColumn{}

// DataColumnsVerifier enables LazilyPersistentStoreColumn to manage the verification process
// going from RODataColumn->VerifiedRODataColumn, while avoiding the decision of which individual verifications
// to run and in what order. Since LazilyPersistentStoreColumn always tries to verify and save data columns only when
// they are all available, the interface takes a slice of data column sidecars.
type DataColumnsVerifier interface {
	VerifiedRODataColumns(ctx context.Context, blk blocks.ROBlock, scs []blocks.RODataColumn) ([]blocks.VerifiedRODataColumn, error)
}

// NewLazilyPersistentStoreColumn creates a new LazilyPersistentStoreColumn.
// WARNING: The resulting LazilyPersistentStoreColumn is NOT thread-safe.
func NewLazilyPersistentStoreColumn(store *filesystem.DataColumnStorage, nodeID enode.ID, newDataColumnsVerifier verification.NewDataColumnsVerifier, custodyInfo *peerdas.CustodyInfo) *LazilyPersistentStoreColumn {
	return &LazilyPersistentStoreColumn{
		store:                  store,
		nodeID:                 nodeID,
		cache:                  newDataColumnCache(),
		custodyInfo:            custodyInfo,
		newDataColumnsVerifier: newDataColumnsVerifier,
	}
}

// PersistColumns adds columns to the working column cache. Columns stored in this cache will be persisted
// for at least as long as the node is running. Once IsDataAvailable succeeds, all columns referenced
// by the given block are guaranteed to be persisted for the remainder of the retention period.
func (s *LazilyPersistentStoreColumn) Persist(current primitives.Slot, sidecars ...blocks.ROSidecar) error {
	if len(sidecars) == 0 {
		return nil
	}

	dataColumnSidecars, err := blocks.DataColumnSidecarsFromSidecars(sidecars)
	if err != nil {
		return errors.Wrap(err, "blob sidecars from sidecars")
	}

	// It is safe to retrieve the first sidecar.
	firstSidecar := dataColumnSidecars[0]

	if len(sidecars) > 1 {
		firstRoot := firstSidecar.BlockRoot()
		for _, sidecar := range dataColumnSidecars[1:] {
			if sidecar.BlockRoot() != firstRoot {
				return errMixedRoots
			}
		}
	}

	firstSidecarEpoch, currentEpoch := slots.ToEpoch(firstSidecar.Slot()), slots.ToEpoch(current)
	if !params.WithinDAPeriod(firstSidecarEpoch, currentEpoch) {
		return nil
	}

	key := cacheKey{slot: firstSidecar.Slot(), root: firstSidecar.BlockRoot()}
	entry := s.cache.ensure(key)

	for _, sidecar := range dataColumnSidecars {
		if err := entry.stash(&sidecar); err != nil {
			return errors.Wrap(err, "stash DataColumnSidecar")
		}
	}

	return nil
}

// IsDataAvailable returns nil if all the commitments in the given block are persisted to the db and have been verified.
// DataColumnsSidecars already in the db are assumed to have been previously verified against the block.
func (s *LazilyPersistentStoreColumn) IsDataAvailable(ctx context.Context, currentSlot primitives.Slot, block blocks.ROBlock) error {
	blockCommitments, err := s.fullCommitmentsToCheck(s.nodeID, block, currentSlot)
	if err != nil {
		return errors.Wrapf(err, "full commitments to check with block root `%#x` and current slot `%d`", block.Root(), currentSlot)
	}

	// Return early for blocks that do not have any commitments.
	if blockCommitments.count() == 0 {
		return nil
	}

	// Get the root of the block.
	blockRoot := block.Root()

	// Build the cache key for the block.
	key := cacheKey{slot: block.Block().Slot(), root: blockRoot}

	// Retrieve the cache entry for the block, or create an empty one if it doesn't exist.
	entry := s.cache.ensure(key)

	// Delete the cache entry for the block at the end.
	defer s.cache.delete(key)

	// Set the disk summary for the block in the cache entry.
	entry.setDiskSummary(s.store.Summary(blockRoot))

	// Verify we have all the expected sidecars, and fail fast if any are missing or inconsistent.
	// We don't try to salvage problematic batches because this indicates a misbehaving peer and we'd rather
	// ignore their response and decrease their peer score.
	roDataColumns, err := entry.filter(blockRoot, blockCommitments)
	if err != nil {
		return errors.Wrap(err, "entry filter")
	}

	// https://github.com/ethereum/consensus-specs/blob/master/specs/fulu/p2p-interface.md#datacolumnsidecarsbyrange-v1
	verifier := s.newDataColumnsVerifier(roDataColumns, verification.ByRangeRequestDataColumnSidecarRequirements)

	if err := verifier.ValidFields(); err != nil {
		return errors.Wrap(err, "valid")
	}

	if err := verifier.SidecarInclusionProven(); err != nil {
		return errors.Wrap(err, "sidecar inclusion proven")
	}

	if err := verifier.SidecarKzgProofVerified(); err != nil {
		return errors.Wrap(err, "sidecar KZG proof verified")
	}

	verifiedRoDataColumns, err := verifier.VerifiedRODataColumns()
	if err != nil {
		return errors.Wrap(err, "verified RO data columns - should never happen")
	}

	if err := s.store.Save(verifiedRoDataColumns); err != nil {
		return errors.Wrap(err, "save data column sidecars")
	}

	return nil
}

// fullCommitmentsToCheck returns the commitments to check for a given block.
func (s *LazilyPersistentStoreColumn) fullCommitmentsToCheck(nodeID enode.ID, block blocks.ROBlock, currentSlot primitives.Slot) (*safeCommitmentsArray, error) {
	// Return early for blocks that are pre-Fulu.
	if block.Version() < version.Fulu {
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

	// Retrieve the groups count.
	custodyGroupCount := s.custodyInfo.ActualGroupCount()

	// Retrieve peer info.
	peerInfo, _, err := peerdas.Info(nodeID, custodyGroupCount)
	if err != nil {
		return nil, errors.Wrap(err, "peer info")
	}

	// Create a safe commitments array for the custody columns.
	commitmentsArray := &safeCommitmentsArray{}
	commitmentsArraySize := uint64(len(commitmentsArray))

	for column := range peerInfo.CustodyColumns {
		if column >= commitmentsArraySize {
			return nil, errors.Errorf("custody column index %d too high (max allowed %d) - should never happen", column, commitmentsArraySize)
		}

		commitmentsArray[column] = kzgCommitments
	}

	return commitmentsArray, nil
}
