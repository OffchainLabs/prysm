package coverage

import (
	"context"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/db"
	dbiface "github.com/OffchainLabs/prysm/v7/beacon-chain/db/iface"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// errScanChainMismatch reports that canonical candidates stopped forming a
// direct parent-linked chain mid-scan, which happens when the canonical view
// changes underneath an uncommitted page. The scan aborts and retries on the
// next wake.
var errScanChainMismatch = errors.New("canonical blocks no longer form a direct parent-linked chain")

// scanChild is the canonical block acting as child testimony for the pair
// currently being classified.
type scanChild struct {
	root [32]byte
	blk  interfaces.ReadOnlySignedBeaconBlock
	slot primitives.Slot
}

// pendingSlot accumulates canonical selection for one slot across page
// boundaries: a packed slot value can straddle two budgeted pages.
type pendingSlot struct {
	slot  primitives.Slot
	root  [32]byte
	found bool
}

// selectCanonicalPerSlot folds a page of slot index candidates into
// per-slot canonical selections, invoking finalize exactly once per slot that
// has a canonical block (slots without one are skipped by absence). The
// returned pendingSlot carries the selection state of the last slot in the
// page, which may continue into the next page.
func (s *Service) selectCanonicalPerSlot(
	ctx context.Context,
	cands []dbiface.SlotIndexCandidate,
	pending *pendingSlot,
	finalize func(primitives.Slot, [32]byte) (bool, error),
) (*pendingSlot, bool, error) {
	for _, cand := range cands {
		if pending != nil && cand.Slot != pending.slot {
			if pending.found {
				stop, err := finalize(pending.slot, pending.root)
				if stop || err != nil {
					return nil, stop, err
				}
			}
			pending = nil
		}
		if pending == nil {
			pending = &pendingSlot{slot: cand.Slot}
		}
		if !pending.found {
			canonical, err := s.isCanonical(ctx, cand.Root)
			if err != nil {
				return nil, false, err
			}
			if canonical {
				pending.root = cand.Root
				pending.found = true
			}
		}
	}
	return pending, false, nil
}

// extendLower migrates coverage downward from the committed lower bound
// toward the retention floor in budgeted cursor pages, committing the durable
// low bound and the exact index range replacement after every verified page.
// The first page after an empty (head-seeded) interval is deliberately small
// so the node starts serving near-head history quickly. A missing or invalid
// envelope for a revealed pair stops extension above that slot.
func (s *Service) extendLower(ctx context.Context, gen uint64, floor primitives.Slot) error {
	snap := s.store.snapshot()
	if snap.Low <= floor || snap.Low == 0 {
		return nil
	}
	child, err := s.lowestCoveredChild(ctx, snap)
	if err != nil || child == nil {
		return err
	}

	curLow := snap.Low
	seedPhase := snap.Low == snap.High
	cursor := dbiface.SlotIndexCursor{Slot: snap.Low - 1}
	var pending *pendingSlot
	var entries []dbiface.RevealedEnvelopeIndexEntry
	stopped := false
	var stopSlot primitives.Slot

	// finalize classifies canonical parent P at slot p against the current
	// child testimony, records the revealed index entry when applicable, and
	// moves the child down to P.
	finalize := func(p primitives.Slot, root [32]byte) (bool, error) {
		parentBlk, err := s.db.Block(ctx, root)
		if err != nil || parentBlk == nil || parentBlk.IsNil() {
			return false, errors.Wrapf(err, "canonical block %#x at slot %d unavailable", root, p)
		}
		if child.blk.Block().ParentRoot() != root {
			return false, errors.Wrapf(errScanChainMismatch, "child %#x at slot %d does not descend from %#x at slot %d", child.root, child.slot, root, p)
		}
		revealed, err := blocks.BlockBuiltOnParentPayload(parentBlk.Block(), child.blk.Block())
		if err != nil {
			return false, errors.Wrap(err, "classify parent payload fullness")
		}
		if revealed {
			env, fp, err := s.db.ExecutionPayloadEnvelopeWithFingerprint(ctx, root)
			if err != nil {
				if errors.Is(err, db.ErrNotFound) {
					stopped, stopSlot = true, p
					return true, nil
				}
				return false, errors.Wrap(err, "load envelope for revealed slot")
			}
			if verr := ValidateEnvelopeAgainstBlock(env, parentBlk, root); verr != nil {
				log.WithError(verr).WithFields(logrus.Fields{"slot": p}).Warn("Stored envelope failed validation; stopping coverage extension above it")
				stopped, stopSlot = true, p
				return true, nil
			}
			entries = append(entries, dbiface.RevealedEnvelopeIndexEntry{Slot: p, Root: root, PrimaryFingerprint: fp})
		}
		child = &scanChild{root: root, blk: parentBlk, slot: p}
		return false, nil
	}

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if s.staleGeneration(gen) {
			return nil // discard the uncommitted page; restart against the new anchor
		}
		budget := s.pageCandidates
		if seedPhase {
			budget = s.seedPageCandidates
		}
		page, err := s.db.BlockSlotIndexPageDescending(ctx, cursor, floor, budget, s.pageBytes)
		if err != nil {
			if errors.Is(err, dbiface.ErrSlotIndexCursorInvalidated) {
				// Restart the current slot from its beginning.
				cursor = dbiface.SlotIndexCursor{Slot: cursor.Slot}
				pending = nil
				continue
			}
			return errors.Wrap(err, "descending slot index page")
		}
		migrationEntriesTotal.Add(float64(len(page.Candidates)))

		var stop bool
		pending, stop, err = s.selectCanonicalPerSlot(ctx, page.Candidates, pending, finalize)
		if err != nil {
			return err
		}
		carry := !stop && page.Next != nil && page.Next.NextByteOffset > 0 && pending != nil && pending.slot == page.Next.Slot
		if !stop && !carry && pending != nil && pending.found {
			_, err = finalize(pending.slot, pending.root)
			if err != nil {
				return err
			}
		}
		if !carry {
			pending = nil
		}

		newLow := floor
		if page.Next != nil {
			newLow = page.Next.Slot + 1
		}
		if stopped {
			newLow = stopSlot + 1
		}
		if newLow < curLow {
			cov := s.store.clone()
			if cov == nil {
				return errors.New("coverage disappeared during lower extension")
			}
			cov.LowSlot = uint64(newLow)
			repl := []dbiface.EnvelopeIndexReplacement{{Start: newLow, End: curLow, Entries: entries}}
			// Pure extension outside the old interval: not destructive.
			if err := s.store.commit(ctx, cov, repl, false); err != nil {
				return errors.Wrap(err, "commit lower coverage page")
			}
			migrationPagesTotal.Inc()
			s.notePublished(newLow, snap.High)
			curLow = newLow
			entries = nil
		}
		if stopped {
			log.WithFields(logrus.Fields{"slot": stopSlot}).Info("Envelope coverage extension stopped above a revealed slot without a stored envelope")
			return nil
		}
		if page.Next == nil {
			s.noteFloorReached()
			return nil
		}
		cursor = *page.Next
		seedPhase = false
		if s.interPageYield > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(s.interPageYield):
			}
		}
	}
}

// extendUpper advances the upper bound toward the canonical head in budgeted
// ascending pages. Every page range-replaces [oldHigh, newHigh) and publishes
// the new anchor in the same combined commit. A revealed pair whose envelope
// is missing or invalid stops the advance at that pair, leaving the anchor
// below the head so crossing requests refuse until the gap is repaired.
func (s *Service) extendUpper(ctx context.Context, gen uint64, headSlot primitives.Slot) error {
	snap := s.store.snapshot()
	if snap.High > headSlot {
		return nil
	}
	anchorBlk, err := s.db.Block(ctx, snap.AnchorRoot)
	if err != nil || anchorBlk == nil || anchorBlk.IsNil() {
		return errors.Wrapf(err, "coverage anchor block %#x unavailable", snap.AnchorRoot)
	}
	prev := &scanChild{root: snap.AnchorRoot, blk: anchorBlk, slot: snap.High}
	curHigh := snap.High
	cursor := dbiface.SlotIndexCursor{Slot: snap.High + 1}
	var pending *pendingSlot
	var entries []dbiface.RevealedEnvelopeIndexEntry
	stopped := false

	finalize := func(k primitives.Slot, root [32]byte) (bool, error) {
		childBlk, err := s.db.Block(ctx, root)
		if err != nil || childBlk == nil || childBlk.IsNil() {
			return false, errors.Wrapf(err, "canonical block %#x at slot %d unavailable", root, k)
		}
		if childBlk.Block().ParentRoot() != prev.root {
			return false, errors.Wrapf(errScanChainMismatch, "canonical child %#x at slot %d does not link to %#x at slot %d", root, k, prev.root, prev.slot)
		}
		revealed, err := blocks.BlockBuiltOnParentPayload(prev.blk.Block(), childBlk.Block())
		if err != nil {
			return false, errors.Wrap(err, "classify parent payload fullness")
		}
		if revealed {
			env, fp, err := s.db.ExecutionPayloadEnvelopeWithFingerprint(ctx, prev.root)
			if err != nil {
				if errors.Is(err, db.ErrNotFound) {
					stopped = true
					return true, nil
				}
				return false, errors.Wrap(err, "load envelope for revealed slot")
			}
			if verr := ValidateEnvelopeAgainstBlock(env, prev.blk, prev.root); verr != nil {
				log.WithError(verr).WithFields(logrus.Fields{"slot": prev.slot}).Warn("Stored envelope failed validation; stopping coverage advance at it")
				stopped = true
				return true, nil
			}
			entries = append(entries, dbiface.RevealedEnvelopeIndexEntry{Slot: prev.slot, Root: prev.root, PrimaryFingerprint: fp})
		}
		prev = &scanChild{root: root, blk: childBlk, slot: k}
		return false, nil
	}

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if s.staleGeneration(gen) {
			return nil
		}
		page, err := s.db.BlockSlotIndexPageAscending(ctx, cursor, headSlot, s.pageCandidates, s.pageBytes)
		if err != nil {
			if errors.Is(err, dbiface.ErrSlotIndexCursorInvalidated) {
				cursor = dbiface.SlotIndexCursor{Slot: cursor.Slot}
				pending = nil
				continue
			}
			return errors.Wrap(err, "ascending slot index page")
		}
		migrationEntriesTotal.Add(float64(len(page.Candidates)))

		var stop bool
		pending, stop, err = s.selectCanonicalPerSlot(ctx, page.Candidates, pending, finalize)
		if err != nil {
			return err
		}
		carry := !stop && page.Next != nil && page.Next.NextByteOffset > 0 && pending != nil && pending.slot == page.Next.Slot
		if !stop && !carry && pending != nil && pending.found {
			_, err = finalize(pending.slot, pending.root)
			if err != nil {
				return err
			}
		}
		if !carry {
			pending = nil
		}

		if prev.slot > curHigh {
			cov := s.store.clone()
			if cov == nil {
				return errors.New("coverage disappeared during upper extension")
			}
			cov.HighSlot = uint64(prev.slot)
			cov.HighAnchorRoot = prev.root[:]
			repl := []dbiface.EnvelopeIndexReplacement{{Start: curHigh, End: prev.slot, Entries: entries}}
			// Pure extension outside the old interval: not destructive.
			if err := s.store.commit(ctx, cov, repl, false); err != nil {
				return errors.Wrap(err, "commit upper coverage page")
			}
			migrationPagesTotal.Inc()
			s.notePublished(s.store.snapshot().Low, prev.slot)
			curHigh = prev.slot
			entries = nil
		}
		if stopped {
			log.WithFields(logrus.Fields{"slot": prev.slot}).Info("Envelope coverage advance stopped at a revealed slot without a stored envelope")
			return nil
		}
		if page.Next == nil {
			return nil
		}
		cursor = *page.Next
		if s.interPageYield > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(s.interPageYield):
			}
		}
	}
}

// lowestCoveredChild locates the child testimony for descending extension:
// the lowest canonical block at or above the covered lower bound, falling
// back to the anchor for an empty or all-skipped interval.
func (s *Service) lowestCoveredChild(ctx context.Context, snap Snapshot) (*scanChild, error) {
	anchor := func() (*scanChild, error) {
		blk, err := s.db.Block(ctx, snap.AnchorRoot)
		if err != nil || blk == nil || blk.IsNil() {
			return nil, errors.Wrapf(err, "coverage anchor block %#x unavailable", snap.AnchorRoot)
		}
		return &scanChild{root: snap.AnchorRoot, blk: blk, slot: snap.High}, nil
	}
	if snap.Low == snap.High {
		return anchor()
	}
	cursor := dbiface.SlotIndexCursor{Slot: snap.Low}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		page, err := s.db.BlockSlotIndexPageAscending(ctx, cursor, snap.High-1, s.pageCandidates, s.pageBytes)
		if err != nil {
			if errors.Is(err, dbiface.ErrSlotIndexCursorInvalidated) {
				cursor = dbiface.SlotIndexCursor{Slot: cursor.Slot}
				continue
			}
			return nil, errors.Wrap(err, "seek lowest covered canonical block")
		}
		for _, cand := range page.Candidates {
			canonical, err := s.isCanonical(ctx, cand.Root)
			if err != nil {
				return nil, err
			}
			if !canonical {
				continue
			}
			blk, err := s.db.Block(ctx, cand.Root)
			if err != nil || blk == nil || blk.IsNil() {
				return nil, errors.Wrapf(err, "canonical block %#x at slot %d unavailable", cand.Root, cand.Slot)
			}
			return &scanChild{root: cand.Root, blk: blk, slot: cand.Slot}, nil
		}
		if page.Next == nil {
			return anchor()
		}
		cursor = *page.Next
	}
}
