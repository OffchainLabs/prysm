package stategen

import (
	"context"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// MigrateToCold moves finalized states to cold storage and advances the migration cursor.
func (s *State) MigrateToCold(ctx context.Context, fRoot [32]byte) error {
	ctx, span := trace.StartSpan(ctx, "stateGen.MigrateToCold")
	defer span.End()

	// When migrating states we choose to acquire the migration lock before
	// proceeding. This is to prevent multiple migration routines from overwriting each
	// other.
	s.migrationLock.Lock()
	defer s.migrationLock.Unlock()

	if features.Get().EnableStateDiff {
		return s.migrateToColdHdiff(ctx, fRoot)
	}

	oldFSlot := s.migratedSlot

	fBlock, err := s.beaconDB.Block(ctx, fRoot)
	if err != nil {
		return err
	}
	fSlot := fBlock.Block().Slot()
	if oldFSlot > fSlot {
		return nil
	}

	// Calculate the first archived point slot >= oldFSlot (but > 0).
	// This avoids iterating through every slot and only visits archived points directly.
	var startSlot primitives.Slot
	if oldFSlot == 0 {
		startSlot = s.slotsPerArchivedPoint
	} else {
		// Round up to the next archived point
		startSlot = (oldFSlot + s.slotsPerArchivedPoint - 1) / s.slotsPerArchivedPoint * s.slotsPerArchivedPoint
	}

	// Start at the first archived point after old finalized slot, stop before current finalized slot.
	// Jump directly between archived points.
	for slot := startSlot; slot < fSlot; slot += s.slotsPerArchivedPoint {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		cached, exists, err := s.epochBoundaryStateCache.getBySlot(slot)
		if err != nil {
			return fmt.Errorf("could not get epoch boundary state for slot %d", slot)
		}

		var aRoot [32]byte
		var aState state.BeaconState

		// When the epoch boundary state is not in cache due to skip slot scenario,
		// we have to regenerate the state which will represent epoch boundary.
		// By finding the highest available block below epoch boundary slot, we
		// generate the state for that block root.
		if exists {
			aRoot = cached.root
			aState = cached.state
		} else {
			_, roots, err := s.beaconDB.HighestRootsBelowSlot(ctx, slot)
			if err != nil {
				return err
			}
			// Given the block has been finalized, the db should not have more than one block in a given slot.
			// We should error out when this happens.
			if len(roots) != 1 {
				return errUnknownBlock
			}
			aRoot = roots[0]
			// There's no need to generate the state if the state already exists in the DB.
			// We can skip saving the state.
			if !s.beaconDB.HasState(ctx, aRoot) {
				aState, err = s.StateByRoot(ctx, aRoot)
				if err != nil {
					return err
				}
			}
		}
		if s.beaconDB.HasState(ctx, aRoot) {
			s.migrateHotToCold(aRoot)
			continue
		}

		saveStart := time.Now()
		if err := s.beaconDB.SaveState(ctx, aState, aRoot); err != nil {
			return err
		}

		duration := time.Since(saveStart)
		saveStateToColdSummary.Observe(float64(duration.Milliseconds()))

		log.WithFields(
			logrus.Fields{
				"slot":     aState.Slot(),
				"root":     hex.EncodeToString(bytesutil.Trunc(aRoot[:])),
				"duration": duration,
			}).Info("Saved state in DB")
	}

	// Update finalized info in memory.
	fInfo, ok, err := s.epochBoundaryStateCache.getByBlockRoot(fRoot)
	if err != nil {
		return err
	}
	if ok {
		s.SaveFinalizedState(fRoot, fInfo.state)
	}
	// The migration can complete without the finalized state being cached. Keep
	// finalizedInfo coherent and advance its independent migration cursor.
	s.migratedSlot = fSlot

	return nil
}

// migrateToColdHdiff saves the state-diffs for slots that are in the state diff tree after finalization
func (s *State) migrateToColdHdiff(ctx context.Context, fRoot [32]byte) error {
	oldFSlot := s.migratedSlot
	fSlot, err := s.beaconDB.SlotByBlockRoot(ctx, fRoot)
	if err != nil {
		return errors.Wrap(err, "could not get slot by block root")
	}
	for slot := oldFSlot; slot < fSlot; slot++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		offset, lvl, err := s.beaconDB.SlotInDiffTree(slot)
		if err != nil {
			log.WithError(err).Errorf("could not determine if slot %d is in diff tree", slot)
			continue
		}
		if lvl == -1 {
			continue
		}
		if uint64(slot) == offset {
			continue
		}
		// The state needs to be saved.
		// Try the epoch boundary cache first.
		cached, exists, err := s.epochBoundaryStateCache.getBySlot(slot)
		if err != nil {
			log.WithError(err).Errorf("could not get epoch boundary state for slot %d", slot)
			cached = nil
			exists = false
		}
		// The cache is populated for every epoch boundary block that gets processed and is only ever
		// evicted by size or when a block turns out to be invalid, never when a block loses fork choice.
		// Its slot index is also first-write-wins, so a reorged sibling processed ahead of the canonical
		// block at the same slot keeps the slot key. Either way the cached root can be non-canonical, and
		// the diff tree is keyed by slot, so it has to be checked before the state is committed to a boundary.
		if exists && !s.beaconDB.IsFinalizedBlock(ctx, cached.root) {
			log.WithFields(logrus.Fields{
				"slot": slot,
				"root": fmt.Sprintf("%#x", cached.root),
			}).Debug("Ignoring non-canonical epoch boundary state while migrating to cold")
			exists = false
		}
		var aRoot [32]byte
		var aState state.BeaconState
		if exists {
			aRoot = cached.root
			aState = cached.state
		} else {
			aRoot, err = s.canonicalRootAtOrBelow(ctx, slot, oldFSlot)
			if err != nil {
				return err
			}
			// Different than the legacy MigrateToCold, we need to always get the state even if
			// the state exists in DB as part of the hot state db, because we need to process slots
			// to the state diff tree slots.
			aState, err = s.StateByRoot(ctx, aRoot)
			if err != nil {
				return err
			}
		}

		// advance slots to the target slot
		if aState.Slot() < slot {
			aState, err = transition.ProcessSlots(ctx, aState, slot)
			if err != nil {
				return errors.Wrapf(err, "could not process slots to slot %d", slot)
			}
		}
		saveStart := time.Now()
		if err := s.beaconDB.SaveState(ctx, aState, aRoot); err != nil {
			return err
		}
		duration := time.Since(saveStart)
		saveStateToColdSummary.Observe(float64(duration.Milliseconds()))

		log.WithFields(
			logrus.Fields{
				"slot":     aState.Slot(),
				"root":     fmt.Sprintf("%#x", aRoot),
				"duration": duration,
			}).Info("Saved state in DB")
	}
	// Update finalized info in memory.
	fInfo, ok, err := s.epochBoundaryStateCache.getByBlockRoot(fRoot)
	if err != nil {
		return err
	}
	if ok {
		s.SaveFinalizedState(fRoot, fInfo.state)
	}
	// The migration can complete without the finalized state being cached. Keep
	// finalizedInfo coherent and advance its independent migration cursor.
	s.migratedSlot = fSlot
	return nil
}

// canonicalRootAtOrBelow returns the root of the highest canonical block at or below the given slot,
// skipping slots that hold only blocks which lost fork choice. Those are never deleted from the slot index.
// Canonicality comes from the finalized index, which is decisive below the finalized checkpoint slot.
// floor is a slot already known to be canonical; resolving past it is an error rather than a walk to genesis.
func (s *State) canonicalRootAtOrBelow(ctx context.Context, slot, floor primitives.Slot) ([32]byte, error) {
	// HighestRootsBelowSlot reports a strictly lower slot, so next decreases every round.
	for next := slot + 1; ; {
		high, roots, err := s.beaconDB.HighestRootsBelowSlot(ctx, next)
		if err != nil {
			return [32]byte{}, err
		}
		if high < floor {
			return [32]byte{}, errUnknownBlock
		}
		canonical := make([][32]byte, 0, 1)
		for _, r := range roots {
			if s.beaconDB.IsFinalizedBlock(ctx, r) {
				canonical = append(canonical, r)
			}
		}
		switch len(canonical) {
		case 1:
			return canonical[0], nil
		case 0:
			if high == 0 {
				return [32]byte{}, errUnknownBlock
			}
			next = high
		default:
			return [32]byte{}, errUnknownBlock
		}
	}
}

func (s *State) migrateHotToCold(aRoot [32]byte) {
	// If you are migrating a state and its already part of the hot state cache saved to the db,
	// you can just remove it from the hot state cache as it becomes redundant.
	s.saveHotStateDB.lock.Lock()
	roots := s.saveHotStateDB.blockRootsOfSavedStates
	for i := range roots {
		if aRoot == roots[i] {
			s.saveHotStateDB.blockRootsOfSavedStates = append(roots[:i], roots[i+1:]...)
			// There shouldn't be duplicated roots in `blockRootsOfSavedStates`.
			// Break here is ok.
			break
		}
	}
	s.saveHotStateDB.lock.Unlock()
}
