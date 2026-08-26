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

// CanonicalBlockAtOrBelow returns the slot and root of the highest canonical block at or below the given
// slot, descending past slots that hold only blocks which lost fork choice. Those are never deleted from the
// slot index, so the slot index alone cannot answer this question: canonicality comes from the finalized
// index, which is decisive below the finalized checkpoint slot and is also populated for backfilled blocks.
//
// floor is a slot already known to be canonical; descending past it is an error rather than a walk to
// genesis. Every failure wraps errUnknownBlock.
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
			// Every block whose epoch equals the finalized checkpoint's epoch is in the finalized index,
			// canonical or not: that part of the index is a re-indexing marker rather than a canonicality
			// claim. The checkpoint root is authoritative for its own slot, so prefer it when it is here.
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
