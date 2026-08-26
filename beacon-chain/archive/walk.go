package archive

import (
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db/filters"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db/kv"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stategen"
	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/math"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// verifyEveryNBlocks is how often the walk checks a replayed state against the block's committed state root.
// Every check is a full hash_tree_root, so this is a periodic integrity probe rather than a per-block check.
// The first block of every run is always checked, which is what verifies the operator-supplied origin state.
const verifyEveryNBlocks = 8192

// walk regenerates every state-diff tree boundary in (frontier, target] and persists it. It carries the
// working state in memory across boundaries, so the tree is only ever written, never read back.
func (s *Service) walk(ctx context.Context, target primitives.Slot) error {
	as := s.status()
	st, err := s.db.StateBySlotFromDiffTree(ctx, as.RegeneratedThroughSlot)
	if err != nil {
		return errors.Wrapf(err, "could not load the resume state at slot %d", as.RegeneratedThroughSlot)
	}

	// Always check the first replayed block. On a fresh run this is the only proof the origin state the
	// operator supplied is the real state at that slot; on a resume it proves the resume point is intact.
	sinceVerify := verifyEveryNBlocks
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		b := nextBoundary(as.OriginSlot, st.Slot())
		if b > target {
			return nil
		}
		// The origin is the floor: backfill downloads every block at or above it, so a descent below it
		// means the chain the tree needs is not in the database rather than that the slot was empty.
		highSlot, root, err := stategen.CanonicalBlockAtOrBelow(ctx, s.db, b, as.OriginSlot)
		if err != nil {
			return errors.Wrapf(err, "could not resolve the canonical block at or below boundary %d", b)
		}

		if highSlot > st.Slot() {
			blks, err := s.canonicalBlocks(ctx, st.Slot()+1, highSlot, root)
			if err != nil {
				return err
			}
			for _, blk := range blks {
				st, sinceVerify, err = replayBlock(ctx, st, blk, sinceVerify)
				if err != nil {
					return err
				}
				blocksReplayed.Inc()
			}
		}

		// Advance through any slots the boundary sits in that had no block, so the persisted state is
		// exactly at the boundary slot the tree is keyed by.
		if st.Slot() < b {
			st, err = stategen.ReplayProcessSlots(ctx, st, b)
			if err != nil {
				return errors.Wrapf(err, "could not process slots to boundary %d", b)
			}
		}

		if err := s.db.SaveState(ctx, st, root); err != nil {
			return errors.Wrapf(err, "could not save boundary state at slot %d", b)
		}
		boundariesSaved.Inc()

		as.RegeneratedThroughSlot = b
		if err := s.db.SaveArchiveStatus(ctx, as); err != nil {
			return errors.Wrap(err, "could not persist archive progress")
		}
		s.setStatus(as)
		regeneratedThroughSlot.Set(float64(b))
		s.logProgress(as, target)
	}
}

func replayBlock(
	ctx context.Context,
	st state.BeaconState,
	blk interfaces.ReadOnlySignedBeaconBlock,
	sinceVerify int,
) (state.BeaconState, int, error) {
	slot := blk.Block().Slot()
	st, err := stategen.ReplayProcessSlots(ctx, st, slot)
	if err != nil {
		return nil, 0, errors.Wrapf(err, "could not process slots to block slot %d", slot)
	}
	// No signature or execution-layer verification: these blocks are already finalized and were verified by
	// backfill or by the live chain. ProcessBlockForStateRoot still enforces parent-root continuity, so a hole
	// in the chain surfaces here as an error rather than as a silently wrong state.
	st, err = transition.ProcessBlockForStateRoot(ctx, st, blk)
	if err != nil {
		return nil, 0, errors.Wrapf(err, "could not process block at slot %d", slot)
	}

	sinceVerify++
	if sinceVerify >= verifyEveryNBlocks {
		root, err := st.HashTreeRoot(ctx)
		if err != nil {
			return nil, 0, errors.Wrap(err, "could not compute the replayed state root")
		}
		if root != blk.Block().StateRoot() {
			return nil, 0, fmt.Errorf(
				"replayed state root %#x does not match the state root committed by the block at slot %d (%#x); "+
					"the archive origin state or the regenerated tree is not the chain this node is on",
				root, slot, blk.Block().StateRoot())
		}
		sinceVerify = 0
	}
	return st, sinceVerify, nil
}

// canonicalBlocks returns the canonical chain in [from, to] in ascending slot order, given the root of the
// block at slot to.
func (s *Service) canonicalBlocks(
	ctx context.Context,
	from, to primitives.Slot,
	toRoot [32]byte,
) ([]interfaces.ReadOnlySignedBeaconBlock, error) {
	query := filters.AncestryQuery{
		Earliest:   from,
		Descendent: filters.SlotRoot{Slot: to, Root: toRoot},
	}
	blks, _, err := s.db.Blocks(ctx, filters.NewFilter().SetAncestryQuery(query))
	if err != nil {
		return nil, errors.Wrapf(err, "could not load blocks in [%d, %d]", from, to)
	}
	return blks, nil
}

// nextBoundary returns the lowest tree boundary slot strictly above from. The deepest level's span divides
// every shallower level's span, so multiples of it are exactly the set of boundary slots.
func nextBoundary(offset, from primitives.Slot) primitives.Slot {
	span := deepestSpan()
	if from < offset {
		return offset
	}
	rel := uint64(from - offset)
	return offset + primitives.Slot((rel/span+1)*span)
}

// deepestSpan is the slot distance between adjacent boundaries of the deepest level.
func deepestSpan() uint64 {
	exponents := flags.Get().StateDiffExponents
	if len(exponents) == 0 {
		// Unreachable once flags are configured; avoids a panic in the service goroutine if they are not.
		return math.PowerOf2(5)
	}
	return math.PowerOf2(uint64(exponents[len(exponents)-1]))
}

func (s *Service) logProgress(as *kv.ArchiveStatus, target primitives.Slot) {
	fields := logrus.Fields{
		"regeneratedThroughSlot": as.RegeneratedThroughSlot,
		"targetSlot":             target,
		"originSlot":             as.OriginSlot,
	}
	if target > as.OriginSlot && as.RegeneratedThroughSlot >= as.OriginSlot {
		done := float64(as.RegeneratedThroughSlot - as.OriginSlot)
		total := float64(target - as.OriginSlot)
		fields["completion"] = fmt.Sprintf("%.2f%%", done/total*100)
	}
	s.progressLogger.WithFields(fields).Info("Archive state regeneration in progress")
}
