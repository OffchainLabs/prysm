package coverage

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/das"
	dbiface "github.com/OffchainLabs/prysm/v7/beacon-chain/db/iface"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/proto/dbval"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// reconcile drives durable-state reconciliation until it settles or the
// context is canceled. A head generation change during a pass re-runs the
// pass against the new canonical view.
func (s *Service) reconcile(ctx context.Context) {
	for ctx.Err() == nil {
		gen := s.notifier.headGeneration()
		if err := s.reconcileOnce(ctx, gen); err != nil {
			if ctx.Err() == nil {
				log.WithError(err).Warn("Envelope coverage reconciliation failed")
			}
			return
		}
		if s.notifier.headGeneration() == gen {
			return
		}
	}
}

// reconcileOnce performs one full reconciliation pass: retention floor
// movement, anchor verification with reorg shrink/discard, upper extension
// toward the canonical head, lower extension/migration toward the retention
// floor, and bounded below-floor index cleanup. Scans discard their
// uncommitted page and return early when the head generation changes.
func (s *Service) reconcileOnce(ctx context.Context, gen uint64) error {
	headRoot, headBlk, err := s.resolveHead(ctx)
	if err != nil {
		return err
	}
	if headBlk == nil {
		return nil // no durable canonical head yet
	}
	headSlot := headBlk.Block().Slot()
	floor := das.EnvSpan(s.clock.CurrentSlot()).Begin

	// Bootstrap a missing record as an empty interval anchored at the first
	// durable canonical Gloas head.
	if !s.store.snapshot().Initialized {
		if headBlk.Version() < version.Gloas {
			return nil
		}
		cov := &dbval.EnvelopeCoverage{
			FormatVersion:  1,
			LowSlot:        uint64(headSlot),
			HighSlot:       uint64(headSlot),
			HighAnchorRoot: headRoot[:],
		}
		if err := s.store.commit(ctx, cov, nil, false); err != nil {
			return errors.Wrap(err, "bootstrap empty coverage interval")
		}
		log.WithFields(logrus.Fields{"slot": headSlot}).Info("Initialized empty envelope coverage interval at canonical head")
	}

	snap := s.store.snapshot()

	// A retention floor at or above the upper bound discards the interval:
	// re-anchor empty at the current canonical head and rescan. Stale index
	// keys below the new bound are pruned in bounded pages below.
	if floor >= snap.High && !(snap.Low == snap.High && snap.AnchorRoot == headRoot) {
		if headBlk.Version() < version.Gloas {
			return nil
		}
		if err := s.reanchorEmpty(ctx, headSlot, headRoot, "retention floor passed upper bound"); err != nil {
			return err
		}
		snap = s.store.snapshot()
	} else if floor > snap.Low {
		// Raising the lower bound invalidates previously published contents.
		cov := s.store.clone()
		cov.LowSlot = uint64(floor)
		if err := s.store.commit(ctx, cov, nil, true); err != nil {
			return errors.Wrap(err, "raise coverage lower bound to retention floor")
		}
		snap = s.store.snapshot()
	}

	// Verify the upper anchor and shrink/discard on reorg.
	if snap.Initialized {
		canonical, err := s.isCanonical(ctx, snap.AnchorRoot)
		if err != nil {
			return errors.Wrap(err, "check anchor canonicality")
		}
		if !canonical {
			if err := s.shrinkToCommonAncestor(ctx, snap, headSlot, headRoot, headBlk); err != nil {
				return err
			}
			snap = s.store.snapshot()
		}
	}

	if s.staleGeneration(gen) {
		return nil
	}

	// Extend the upper bound toward the canonical head.
	if snap.Initialized && snap.AnchorRoot != headRoot && snap.High <= headSlot {
		if err := s.extendUpper(ctx, gen, headSlot); err != nil {
			return err
		}
		snap = s.store.snapshot()
	}

	if s.staleGeneration(gen) {
		return nil
	}

	// Extend the lower bound toward the retention floor.
	if snap.Initialized && snap.Low > floor {
		if err := s.extendLower(ctx, gen, floor); err != nil {
			return err
		}
		snap = s.store.snapshot()
	}

	// Bounded cleanup of stale index keys below the published lower bound.
	if snap.Initialized {
		if _, err := s.db.PruneRevealedEnvelopeIndexBelow(ctx, snap.Low, s.pruneBudget); err != nil {
			return errors.Wrap(err, "prune revealed envelope index below floor")
		}
	}
	return nil
}

// reanchorEmpty destructively discards the interval and re-anchors an empty
// one at the given canonical head.
func (s *Service) reanchorEmpty(ctx context.Context, headSlot primitives.Slot, headRoot [32]byte, reason string) error {
	cov := &dbval.EnvelopeCoverage{
		FormatVersion:  1,
		LowSlot:        uint64(headSlot),
		HighSlot:       uint64(headSlot),
		HighAnchorRoot: headRoot[:],
	}
	if err := s.store.commit(ctx, cov, nil, true); err != nil {
		return errors.Wrap(err, "re-anchor empty coverage interval")
	}
	anchorShrinksTotal.Inc()
	log.WithFields(logrus.Fields{"slot": headSlot, "reason": reason}).Info("Re-anchored empty envelope coverage interval at canonical head")
	return nil
}

// shrinkToCommonAncestor handles an orphaned upper anchor: it walks the old
// chain down from the anchor to the highest block that is still canonical.
// At slot C >= low the interval shrinks to C (whose index key is deleted
// together with [C, oldHigh): the new child can flip C's fullness testimony
// while C's beacon root stays canonical). A common ancestor below low has no
// valid shrink: the interval is discarded and re-anchored empty at the new
// canonical head.
func (s *Service) shrinkToCommonAncestor(ctx context.Context, snap Snapshot, headSlot primitives.Slot, headRoot [32]byte, headBlk interfaces.ReadOnlySignedBeaconBlock) error {
	discard := func(reason string) error {
		if headBlk.Version() < version.Gloas {
			return errors.Errorf("cannot re-anchor coverage at pre-Gloas head %#x", headRoot)
		}
		return s.reanchorEmpty(ctx, headSlot, headRoot, reason)
	}
	cur := snap.AnchorRoot
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		blk, err := s.db.Block(ctx, cur)
		if err != nil || blk == nil || blk.IsNil() {
			// The old chain can no longer be walked; the only safe transition
			// is the destructive one.
			return discard("orphaned anchor chain unavailable")
		}
		slot := blk.Block().Slot()
		canonical, err := s.isCanonical(ctx, cur)
		if err != nil {
			return errors.Wrap(err, "check canonicality during anchor shrink")
		}
		if canonical {
			if slot < snap.Low {
				return discard("reorg common ancestor below covered interval")
			}
			cov := s.store.clone()
			cov.HighSlot = uint64(slot)
			cov.HighAnchorRoot = bytesutil.SafeCopyBytes(cur[:])
			// Exact range replacement: delete [C, oldHigh) including C.
			repl := []dbiface.EnvelopeIndexReplacement{{Start: slot, End: snap.High}}
			if err := s.store.commit(ctx, cov, repl, true); err != nil {
				return errors.Wrap(err, "shrink coverage to reorg common ancestor")
			}
			anchorShrinksTotal.Inc()
			log.WithFields(logrus.Fields{
				"commonAncestorSlot": slot,
				"oldHighSlot":        snap.High,
			}).Info("Shrank envelope coverage to reorg common ancestor")
			return nil
		}
		if slot <= snap.Low {
			return discard("reorg common ancestor below covered interval")
		}
		cur = blk.Block().ParentRoot()
	}
}

// resolveHead returns the canonical head root and its durable block, or a
// nil block when the chain view is not bound yet or the head block is not
// durable.
func (s *Service) resolveHead(ctx context.Context) ([32]byte, interfaces.ReadOnlySignedBeaconBlock, error) {
	cv := s.chainView()
	if cv == nil {
		return [32]byte{}, nil, nil
	}
	hr, err := cv.HeadRoot(ctx)
	if err != nil {
		return [32]byte{}, nil, errors.Wrap(err, "resolve head root")
	}
	root := bytesutil.ToBytes32(hr)
	if root == ([32]byte{}) {
		return [32]byte{}, nil, nil
	}
	blk, err := s.db.Block(ctx, root)
	if err != nil || blk == nil || blk.IsNil() {
		return root, nil, nil
	}
	return root, blk, nil
}

func (s *Service) isCanonical(ctx context.Context, root [32]byte) (bool, error) {
	cv := s.chainView()
	if cv == nil {
		return false, errors.New("chain view not bound")
	}
	return cv.IsCanonical(ctx, root)
}

func (s *Service) staleGeneration(gen uint64) bool {
	return s.notifier.headGeneration() != gen
}
