package backfill

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/das"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/pkg/errors"
)

var errMissingAvailabilityChecker = errors.Wrap(errUnrecoverable, "batch is missing required availability checker")
var errUnsafeRange = errors.Wrap(errUnrecoverable, "invalid slice indices")

type checkMultiplexer struct {
	blobCheck    das.AvailabilityChecker
	colCheck     das.AvailabilityChecker
	currentNeeds das.CurrentNeeds
}

// Persist implements das.AvailabilityStore.
var _ das.AvailabilityChecker = &checkMultiplexer{}

// newCheckMultiplexer initializes an AvailabilityChecker that multiplexes to the BlobSidecar and DataColumnSidecar
// AvailabilityCheckers present in the batch.
func newCheckMultiplexer(needs das.CurrentNeeds, b batch) *checkMultiplexer {
	s := &checkMultiplexer{currentNeeds: needs}
	if b.blobs != nil && b.blobs.store != nil {
		s.blobCheck = b.blobs.store
	}
	if b.columns != nil && b.columns.store != nil {
		s.colCheck = b.columns.store
	}

	return s
}

// IsDataAvailable implements the das.AvailabilityStore interface.
func (m *checkMultiplexer) IsDataAvailable(ctx context.Context, current primitives.Slot, blks ...blocks.ROBlock) error {
	needs, err := m.divideByChecker(blks)
	if err != nil {
		return errors.Wrap(errUnrecoverable, "failed to slice blocks by DA type")
	}
	if err := doAvailabilityCheck(ctx, m.blobCheck, current, needs.blobs); err != nil {
		return errors.Wrap(err, "blob store availability check failed")
	}
	if err := doAvailabilityCheck(ctx, m.colCheck, current, needs.cols); err != nil {
		return errors.Wrap(err, "column store availability check failed")
	}
	return nil
}

func doAvailabilityCheck(ctx context.Context, check das.AvailabilityChecker, current primitives.Slot, blks []blocks.ROBlock) error {
	if len(blks) == 0 {
		return nil
	}
	// Double check that the checker is non-nil.
	if check == nil {
		return errMissingAvailabilityChecker
	}
	return check.IsDataAvailable(ctx, current, blks...)
}

// daGroups is a helper type that groups blocks by their DA type.
type daGroups struct {
	blobs []blocks.ROBlock
	cols  []blocks.ROBlock
}

// blocksByDaType slices the given blocks into two slices: one for deneb blocks (BlobSidecar)
// and one for fulu blocks (DataColumnSidecar). Blocks that are pre-deneb or have no
// blob commitments are skipped.
func (m *checkMultiplexer) divideByChecker(blks []blocks.ROBlock) (daGroups, error) {
	needs := daGroups{}
	for _, blk := range blks {
		slot := blk.Block().Slot()

		if !m.currentNeeds.Blob.At(slot) && !m.currentNeeds.Col.At(slot) {
			continue
		}
		cmts, err := blk.Block().Body().BlobKzgCommitments()
		if err != nil {
			return needs, err
		}
		if len(cmts) == 0 {
			continue
		}
		if m.currentNeeds.Col.At(slot) {
			needs.cols = append(needs.cols, blk)
			continue
		}
		if m.currentNeeds.Blob.At(slot) {
			needs.blobs = append(needs.blobs, blk)
			continue
		}
	}

	return needs, nil
}

// safeRange is a helper type that enforces safe slicing.
type safeRange struct {
	start uint
	end   uint
}

// isZero returns true if the range is zero-length.
func (r safeRange) isZero() bool {
	return r.start == r.end
}

// subSlice returns the subslice of s defined by sub
// if it can be safely sliced, or an error if the range is invalid
// with respect to the slice.
func subSlice[T any](s []T, sub safeRange) ([]T, error) {
	slen := uint(len(s))
	if slen == 0 || sub.isZero() {
		return nil, nil
	}

	// Check that minimum bound is safe.
	if sub.end < sub.start {
		return nil, errUnsafeRange
	}
	// Check that upper bound is safe.
	if sub.start >= slen || sub.end > slen {
		return nil, errUnsafeRange
	}
	return s[sub.start:sub.end], nil
}
