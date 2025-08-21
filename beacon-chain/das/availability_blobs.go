package das

import (
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/db/filesystem"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/verification"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/runtime/logging"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

var (
	errMixedRoots = errors.New("BlobSidecars must all be for the same block")
)

// LazilyPersistentStoreBlob is an implementation of AvailabilityStore to be used when batch syncing.
// This implementation will hold any blobs passed to Persist until the IsDataAvailable is called for their
// block, at which time they will undergo full verification and be saved to the disk.
type LazilyPersistentStoreBlob struct {
	store    *filesystem.BlobStorage
	cache    *blobCache
	verifier BlobBatchVerifier
}

var _ AvailabilityStore = &LazilyPersistentStoreBlob{}

// BlobBatchVerifier enables LazyAvailabilityStore to manage the verification process
// going from ROBlob->VerifiedROBlob, while avoiding the decision of which individual verifications
// to run and in what order. Since LazilyPersistentStore always tries to verify and save blobs only when
// they are all available, the interface takes a slice of blobs, enabling the implementation to optimize
// batch verification.
type BlobBatchVerifier interface {
	VerifiedROBlobs(ctx context.Context, blk blocks.ROBlock, sc []blocks.ROBlob) ([]blocks.VerifiedROBlob, error)
}

// NewLazilyPersistentStore creates a new LazilyPersistentStore. This constructor should always be used
// when creating a LazilyPersistentStore because it needs to initialize the cache under the hood.
func NewLazilyPersistentStore(store *filesystem.BlobStorage, verifier BlobBatchVerifier) *LazilyPersistentStoreBlob {
	return &LazilyPersistentStoreBlob{
		store:    store,
		cache:    newBlobCache(),
		verifier: verifier,
	}
}

// Persist adds blobs to the working blob cache. Blobs stored in this cache will be persisted
// for at least as long as the node is running. Once IsDataAvailable succeeds, all blobs referenced
// by the given block are guaranteed to be persisted for the remainder of the retention period.
func (s *LazilyPersistentStoreBlob) Persist(current primitives.Slot, sidecars ...blocks.ROBlob) error {
	if len(sidecars) == 0 {
		return nil
	}

	if len(sidecars) > 1 {
		firstRoot := sidecars[0].BlockRoot()
		for _, sidecar := range sidecars[1:] {
			if sidecar.BlockRoot() != firstRoot {
				return errMixedRoots
			}
		}
	}
	if !params.WithinDAPeriod(slots.ToEpoch(sidecars[0].Slot()), slots.ToEpoch(current)) {
		return nil
	}
	key := keyFromSidecar(sidecars[0])
	entry := s.cache.ensure(key)
	for _, blobSidecar := range sidecars {
		if err := entry.stash(&blobSidecar); err != nil {
			return err
		}
	}
	return nil
}

// IsDataAvailable returns nil if all the commitments in the given block are persisted to the db and have been verified.
// BlobSidecars already in the db are assumed to have been previously verified against the block.
func (s *LazilyPersistentStoreBlob) IsDataAvailable(ctx context.Context, current primitives.Slot, b blocks.ROBlock) error {
	blockCommitments, err := commitmentsToCheck(b, current)
	if err != nil {
		return errors.Wrapf(err, "could not check data availability for block %#x", b.Root())
	}
	// Return early for blocks that are pre-deneb or which do not have any commitments.
	if len(blockCommitments) == 0 {
		return nil
	}

	key := keyFromBlock(b)
	entry := s.cache.ensure(key)
	defer s.cache.delete(key)
	root := b.Root()
	entry.setDiskSummary(s.store.Summary(root))

	// Verify we have all the expected sidecars, and fail fast if any are missing or inconsistent.
	// We don't try to salvage problematic batches because this indicates a misbehaving peer and we'd rather
	// ignore their response and decrease their peer score.
	sidecars, err := entry.filter(root, blockCommitments, b.Block().Slot())
	if err != nil {
		return errors.Wrap(err, "incomplete BlobSidecar batch")
	}
	// Do thorough verifications of each BlobSidecar for the block.
	// Same as above, we don't save BlobSidecars if there are any problems with the batch.
	vscs, err := s.verifier.VerifiedROBlobs(ctx, b, sidecars)
	if err != nil {
		var me verification.VerificationMultiError
		ok := errors.As(err, &me)
		if ok {
			fails := me.Failures()
			lf := make(log.Fields, len(fails))
			for i := range fails {
				lf[fmt.Sprintf("fail_%d", i)] = fails[i].Error()
			}
			log.WithFields(lf).WithFields(logging.BlockFieldsFromBlob(sidecars[0])).
				Debug("Invalid BlobSidecars received")
		}
		return errors.Wrapf(err, "invalid BlobSidecars received for block %#x", root)
	}
	// Ensure that each BlobSidecar is written to disk.
	for i := range vscs {
		if err := s.store.Save(vscs[i]); err != nil {
			return errors.Wrapf(err, "failed to save BlobSidecar index %d for block %#x", vscs[i].Index, root)
		}
	}
	// All BlobSidecars are persisted - da check succeeds.
	return nil
}

func commitmentsToCheck(b blocks.ROBlock, current primitives.Slot) ([][]byte, error) {
	if b.Version() < version.Deneb {
		return nil, nil
	}

	// We are only required to check within MIN_EPOCHS_FOR_BLOB_SIDECARS_REQUEST
	if !params.WithinDAPeriod(slots.ToEpoch(b.Block().Slot()), slots.ToEpoch(current)) {
		return nil, nil
	}

	kzgCommitments, err := b.Block().Body().BlobKzgCommitments()
	if err != nil {
		return nil, err
	}

	maxBlobCount := params.BeaconConfig().MaxBlobsPerBlock(b.Block().Slot())
	if len(kzgCommitments) > maxBlobCount {
		return nil, errIndexOutOfBounds
	}

	result := make([][]byte, len(kzgCommitments))
	copy(result, kzgCommitments)

	return result, nil
}
