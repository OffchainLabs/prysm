package stategen

import (
	"context"
	"slices"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
)

// CanonicalBlockDB is the database subset needed to resolve the canonical block at or below a slot.
type CanonicalBlockDB interface {
	HighestRootsBelowSlot(ctx context.Context, slot primitives.Slot) (primitives.Slot, [][32]byte, error)
	IsFinalizedBlock(ctx context.Context, blockRoot [32]byte) bool
	FinalizedCheckpoint(ctx context.Context) (*ethpb.Checkpoint, error)
}

// CanonicalBlockAtOrBelow returns the highest canonical block at or below slot, skipping orphaned slots.
func CanonicalBlockAtOrBelow(
	ctx context.Context,
	db CanonicalBlockDB,
	slot, floor primitives.Slot,
) (primitives.Slot, [32]byte, error) {
	// HighestRootsBelowSlot reports a strictly lower slot, so next decreases every round.
	for next := slot + 1; ; {
		high, roots, err := db.HighestRootsBelowSlot(ctx, next)
		if err != nil {
			return 0, [32]byte{}, err
		}
		if high < floor {
			return 0, [32]byte{}, errors.Wrapf(errUnknownBlock, "no canonical block in [%d, %d]", floor, slot)
		}
		canonical := make([][32]byte, 0, 1)
		for _, r := range roots {
			if db.IsFinalizedBlock(ctx, r) {
				canonical = append(canonical, r)
			}
		}
		switch len(canonical) {
		case 1:
			return high, canonical[0], nil
		case 0:
			// The slot holds only orphans. Keep descending: the canonical chain simply has no block here.
			if high == 0 {
				return 0, [32]byte{}, errors.Wrapf(errUnknownBlock, "no canonical block at or below slot %d", slot)
			}
			next = high
		default:
			// The checkpoint root is authoritative for its own slot, so prefer it when it is here.
			cpRoot, err := finalizedCheckpointRoot(ctx, db)
			if err != nil {
				return 0, [32]byte{}, err
			}
			if slices.Contains(canonical, cpRoot) {
				return high, cpRoot, nil
			}
			return 0, [32]byte{}, errors.Wrapf(errUnknownBlock,
				"slot %d has %d canonical candidates", high, len(canonical))
		}
	}
}

func finalizedCheckpointRoot(ctx context.Context, db CanonicalBlockDB) ([32]byte, error) {
	cp, err := db.FinalizedCheckpoint(ctx)
	if err != nil {
		return [32]byte{}, errors.Wrap(err, "could not read the finalized checkpoint")
	}
	return bytesutil.ToBytes32(cp.Root), nil
}
