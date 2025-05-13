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
	columnStore das.AvailabilityStore
	blobStore   das.AvailabilityStore
}

// Persist implements das.AvailabilityStore.
var _ das.AvailabilityChecker = &multiStore{}

// IsDataAvailable implements the das.AvailabilityStore interface.
func (m *multiStore) IsDataAvailable(ctx context.Context, current primitives.Slot, blk blocks.ROBlock) error {
	if blk.Block().Slot() < m.fuluStart {
		return m.checkAvailabilityWithFallback(ctx, m.blobStore, current, blk)
	}
	return m.checkAvailabilityWithFallback(ctx, m.columnStore, current, blk)
}

func (m *multiStore) checkAvailabilityWithFallback(ctx context.Context, ac das.AvailabilityChecker, current primitives.Slot, blk blocks.ROBlock) error {
	if ac != nil {
		return ac.IsDataAvailable(ctx, current, blk)
	}
	cmts, err := blk.Block().Body().BlobKzgCommitments()
	if err != nil {
		return err
	}
	if len(cmts) > 0 {
		return errAvailabilityCheckerInvalid
	}
	return nil
}

func newMultiStore(fuluStart primitives.Slot, b batch) *multiStore {
	s := &multiStore{fuluStart: fuluStart}
	if b.bs != nil && b.bs.store != nil {
		s.blobStore = b.bs.store
	}
	if b.cs != nil && b.cs.store != nil {
		s.columnStore = b.cs.store
	}
	return s
}
