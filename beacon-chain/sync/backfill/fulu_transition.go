package backfill

import (
	"context"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/das"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/pkg/errors"
)

var errMissingAvailabilityChecker = errors.Wrap(errUnrecoverable, "batch is missing required availability checker")
var errInvalidCheckSlice = errors.Wrap(errUnrecoverable, "invalid slice indices for availability check")

type checkMultiplexer struct {
	sliceIdx struct {
		blobs   [2]int
		columns [2]int
	}
	blobCheck das.AvailabilityChecker
	colCheck  das.AvailabilityChecker
}

// Persist implements das.AvailabilityStore.
var _ das.AvailabilityChecker = &checkMultiplexer{}

// IsDataAvailable implements the das.AvailabilityStore interface.
func (m *checkMultiplexer) IsDataAvailable(ctx context.Context, current primitives.Slot, blks ...blocks.ROBlock) error {
	if doAvailabilityCheck(ctx, m.blobCheck, current, blks, m.sliceIdx.blobs) != nil {
		return errors.Wrap(errUnrecoverable, "blob store availability check failed")
	}
	if doAvailabilityCheck(ctx, m.colCheck, current, blks, m.sliceIdx.columns) != nil {
		return errors.Wrap(errUnrecoverable, "column store availability check failed")
	}
	return nil
}

func doAvailabilityCheck(ctx context.Context, check das.AvailabilityChecker, current primitives.Slot, blks []blocks.ROBlock, idxs [2]int) error {
	// No blocks found for this type (uninitialized indices / resulting slice would be zero-len).
	if idxs[0] == idxs[1] {
		return nil
	}
	// Check that minimum bound is safe.
	if idxs[0] < 0 || idxs[1] < idxs[0] {
		return errInvalidCheckSlice
	}
	// Check that upper bound is safe.
	if idxs[0] >= len(blks) || idxs[1] > len(blks) {
		return errInvalidCheckSlice
	}

	// Double check that the checker is non-nil.
	if check == nil {
		return errMissingAvailabilityChecker
	}
	return check.IsDataAvailable(ctx, current, blks[idxs[0]:idxs[1]]...)
}

// newCheckMultiplexer initializes an AvailabilityChecker that multiplexes to the BlobSidecar and DataColumnSidecar
// AvailabilityCheckers present in the batch. It precomputes the indices of the blocks in the batch that
// correspond to each store based on the slot and presence of commitments.
// Returns an error if the batch doesn't contain the necessary AvailabilityCheckers for the blocks present.
func newCheckMultiplexer(fuluStart, denebStart primitives.Slot, b batch) (*checkMultiplexer, error) {
	s := &checkMultiplexer{}
	if b.blobs != nil && b.blobs.store != nil {
		s.blobCheck = b.blobs.store
	}
	if b.columns != nil && b.columns.store != nil {
		s.colCheck = b.columns.store
	}

	// Determine the indices for BlobSidecars and DataColumnSidecars in the batch.
	for i, blk := range b.blocks {
		slot := blk.Block().Slot()

		// Avoid checking commitments on pre-deneb blocks.
		if slot < denebStart {
			continue
		}

		// Ignore blocks with no commitments for the purpose of finding deneb/fulu ranges.
		cmts, err := blk.Block().Body().BlobKzgCommitments()
		if err != nil {
			return nil, err
		}
		if len(cmts) == 0 {
			continue
		}

		// Find the start and end of slices in the batch that correspond to deneb and fulu blocks.
		// Check that the necessary AvailabilityChecker is present for each type of block.
		if slot >= fuluStart {
			if s.colCheck == nil {
				return nil, errors.Wrap(errMissingAvailabilityChecker, "batch with DataColumnSidecar commitments but no availability checker")
			}
			// Only set the start index the first time we see a fulu block with commitments.
			if s.sliceIdx.columns[1] == 0 {
				s.sliceIdx.columns[0] = i
			}
			// Assume rest of slice is fulu; set stop index to end of slice and break the loop.
			s.sliceIdx.columns[1] = len(b.blocks)
			break
		}

		if slot >= denebStart {
			if s.blobCheck == nil {
				return nil, errors.Wrap(errMissingAvailabilityChecker, "batch with BlobSidecar commitments but no availability checker")
			}
			if s.sliceIdx.blobs[1] == 0 {
				s.sliceIdx.blobs[0] = i
			}
			s.sliceIdx.blobs[1] = i + 1 // update each time so we end at the highest index
		}
	}

	return s, nil
}
