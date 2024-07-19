package das

import (
	"context"
	"fmt"

	"github.com/ethereum/go-ethereum/p2p/enode"
	errors "github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/db/filesystem"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/verification"
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
	store    *filesystem.BlobStorage
	cache    *cache
	verifier ColumnBatchVerifier
	nodeID   enode.ID
}

type ColumnBatchVerifier interface {
	VerifiedRODataColumns(ctx context.Context, blk blocks.ROBlock, sc []blocks.RODataColumn) ([]blocks.VerifiedRODataColumn, error)
}

func NewLazilyPersistentStoreColumn(store *filesystem.BlobStorage, verifier ColumnBatchVerifier, id enode.ID) *LazilyPersistentStoreColumn {
	return &LazilyPersistentStoreColumn{
		store:    store,
		cache:    newCache(),
		verifier: verifier,
		nodeID:   id,
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
func (s *LazilyPersistentStoreColumn) IsDataAvailable(ctx context.Context, current primitives.Slot, b blocks.ROBlock) error {
	blockCommitments, err := fullCommitmentsToCheck(b, current)
	if err != nil {
		return errors.Wrapf(err, "could check data availability for block %#x", b.Root())
	}
	// Return early for blocks that are pre-deneb or which do not have any commitments.
	if blockCommitments.count() == 0 {
		return nil
	}

	key := keyFromBlock(b)
	entry := s.cache.ensure(key)
	defer s.cache.delete(key)
	root := b.Root()
	sumz, err := s.store.WaitForSummarizer(ctx)
	if err != nil {
		log.WithField("root", fmt.Sprintf("%#x", b.Root())).
			WithError(err).
			Debug("Failed to receive BlobStorageSummarizer within IsDataAvailable")
	} else {
		entry.setDiskSummary(sumz.Summary(root))
	}

	// Verify we have all the expected sidecars, and fail fast if any are missing or inconsistent.
	// We don't try to salvage problematic batches because this indicates a misbehaving peer and we'd rather
	// ignore their response and decrease their peer score.
	sidecars, err := entry.filterColumns(root, &blockCommitments)
	if err != nil {
		return errors.Wrap(err, "incomplete BlobSidecar batch")
	}
	// Do thorough verifications of each BlobSidecar for the block.
	// Same as above, we don't save BlobSidecars if there are any problems with the batch.
	vscs, err := s.verifier.VerifiedRODataColumns(ctx, b, sidecars)
	if err != nil {
		var me verification.VerificationMultiError
		ok := errors.As(err, &me)
		if ok {
			fails := me.Failures()
			lf := make(log.Fields, len(fails))
			for i := range fails {
				lf[fmt.Sprintf("fail_%d", i)] = fails[i].Error()
			}
			log.WithFields(lf).
				Debug("invalid ColumnSidecars received")
		}
		return errors.Wrapf(err, "invalid ColumnSidecars received for block %#x", root)
	}
	// Ensure that each column sidecar is written to disk.
	for i := range vscs {
		if err := s.store.SaveDataColumn(vscs[i]); err != nil {
			return errors.Wrapf(err, "failed to save ColumnSidecar index %d for block %#x", vscs[i].ColumnIndex, root)
		}
	}
	// All ColumnSidecars are persisted - da check succeeds.
	return nil
}

func fullCommitmentsToCheck(b blocks.ROBlock, current primitives.Slot) (safeCommitmentsArray, error) {
	var ar safeCommitmentsArray
	if b.Version() < version.Deneb {
		return ar, nil
	}
	// We are only required to check within MIN_EPOCHS_FOR_DATA_COLUMN_SIDECARS_REQUESTS
	if !params.WithinDAPeriod(slots.ToEpoch(b.Block().Slot()), slots.ToEpoch(current)) {
		return ar, nil
	}
	kc, err := b.Block().Body().BlobKzgCommitments()
	if err != nil {
		return ar, err
	}
	for i := range ar {
		copy(ar[i], kc)
	}
	return ar, nil
}
