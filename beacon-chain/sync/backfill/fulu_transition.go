package backfill

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/das"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/pkg/errors"
)

var errMissingAvailabilityChecker = errors.Wrap(errUnrecoverable, "batch is missing required availability checker")
var errUnsafeRange = errors.Wrap(errUnrecoverable, "invalid slice indices")
var errUnresolvedPayloadFullness = errors.New("gloas block with unresolved payload fullness cannot be imported")

type checkMultiplexer struct {
	blobCheck    das.AvailabilityChecker
	colCheck     das.AvailabilityChecker
	currentNeeds das.CurrentNeeds
	columns      *columnSync
}

// Persist implements das.AvailabilityStore.
var _ das.AvailabilityChecker = &checkMultiplexer{}

// newCheckMultiplexer initializes an AvailabilityChecker that multiplexes to the BlobSidecar and DataColumnSidecar
// AvailabilityCheckers present in the batch.
func newCheckMultiplexer(needs das.CurrentNeeds, b batch) *checkMultiplexer {
	s := &checkMultiplexer{currentNeeds: needs, columns: b.columns}
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
		// An unresolved gloas tail is retryable; the importer settles it against the canonical child.
		if errors.Is(err, errUnresolvedPayloadFullness) {
			return err
		}
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

// divideByChecker slices the given blocks into two slices: one for deneb blocks (BlobSidecar)
// and one for fulu blocks (DataColumnSidecar). Blocks that are pre-deneb or have no
// blob commitments are skipped, as are gloas blocks whose payload was withheld, since no
// columns exist for them. A gloas block with unresolved fullness must not be imported on a guess.
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
			if blk.Block().Version() >= version.Gloas {
				switch m.columns.fullness(blk.Root()) {
				case fullnessWithheld:
					continue
				case fullnessUnknown:
					return needs, errors.Wrapf(errUnresolvedPayloadFullness, "root=%#x, slot=%d", blk.Root(), slot)
				}
			}
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
