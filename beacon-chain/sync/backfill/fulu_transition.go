package backfill

import (
	"context"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/das"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/pkg/errors"
)

var errAvailabilityCheckerInvalid = errors.New("invalid availability checker state")

type multiStore struct {
	fuluStart   primitives.Slot
	columnStore das.AvailabilityChecker
	blobStore   das.AvailabilityChecker
}

// Persist implements das.AvailabilityStore.
var _ das.AvailabilityChecker = &multiStore{}

// IsDataAvailable implements the das.AvailabilityStore interface.
func (m *multiStore) IsDataAvailable(ctx context.Context, current primitives.Slot, blks ...blocks.ROBlock) error {
	for i := range blks {
		// Slice the blocks and route to the appropriate store based on the fulu transition slot.
		if blks[i].Block().Slot() >= m.fuluStart {
			if err := m.checkAvailabilityWithFallback(ctx, m.columnStore, current, blks[i:]...); err != nil {
				return err
			}
			// If there were any pre-fulu blocks in the batch, route those to the blob store.
			if i > 0 {
				return m.checkAvailabilityWithFallback(ctx, m.blobStore, current, blks[:i]...)
			}
			return nil
		}
	}
	// If we get here, all blocks are before the fulu transition.
	return m.checkAvailabilityWithFallback(ctx, m.blobStore, current, blks...)
}

func (m *multiStore) checkAvailabilityWithFallback(ctx context.Context, ac das.AvailabilityChecker, current primitives.Slot, blks ...blocks.ROBlock) error {
	if ac != nil {
		return ac.IsDataAvailable(ctx, current, blks...)
	}
	// TODO: I think this was a hack and should not be necessary any longer.
	// Perhaps this could happen with lazy initialization of the availability stores
	// if the batch is pre-deneb or if there are no blobs in the batch?
	for _, blk := range blks {
		cmts, err := blk.Block().Body().BlobKzgCommitments()
		if err != nil {
			return err
		}
		if len(cmts) > 0 {
			return errAvailabilityCheckerInvalid
		}
	}
	return nil
}

func newMultiStore(fuluStart primitives.Slot, b batch) *multiStore {
	s := &multiStore{fuluStart: fuluStart}
	if b.blobs != nil && b.blobs.store != nil {
		s.blobStore = b.blobs.store
	}
	if b.columns != nil && b.columns.store != nil {
		s.columnStore = b.columns.store
	}
	return s
}
