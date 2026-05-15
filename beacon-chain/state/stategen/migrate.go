package stategen

import (
	"context"
	"encoding/hex"
	"fmt"
	"slices"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// MigrateToCold advances the finalized info in between the cold and hot state sections.
// It moves the recent finalized states from the hot section to the cold section and
// only preserves the ones that are on archived point.
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

	s.finalizedInfo.lock.RLock()
	oldFSlot := s.finalizedInfo.slot
	s.finalizedInfo.lock.RUnlock()

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

		if err := s.beaconDB.SaveState(ctx, aState, aRoot); err != nil {
			return err
		}
		log.WithFields(
			logrus.Fields{
				"slot": aState.Slot(),
				"root": hex.EncodeToString(bytesutil.Trunc(aRoot[:])),
			}).Info("Saved state in DB")
	}

	// Update finalized info in memory.
	fInfo, ok, err := s.epochBoundaryStateCache.getByBlockRoot(fRoot)
	if err != nil {
		return err
	}
	if ok {
		s.SaveFinalizedState(fSlot, fRoot, fInfo.state)
	}

	return nil
}

// migrateToColdHdiff saves the state-diffs for slots that are in the state diff tree after finalization
func (s *State) migrateToColdHdiff(ctx context.Context, fRoot [32]byte) error {
	s.finalizedInfo.lock.RLock()
	oldFSlot := s.finalizedInfo.slot
	oldFRoot := s.finalizedInfo.root
	oldFState := s.finalizedInfo.state
	s.finalizedInfo.lock.RUnlock()
	fSlot, err := s.beaconDB.SlotByBlockRoot(ctx, fRoot)
	if err != nil {
		return errors.Wrap(err, "could not get slot by block root")
	}

	if oldFState == nil || oldFState.IsNil() {
		return errors.New("finalized state is nil")
	}

	slotsToSave := make([]primitives.Slot, 0)
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
		slotsToSave = append(slotsToSave, slot)
	}

	if len(slotsToSave) == 0 {
		// Update finalized info in memory.
		fInfo, ok, err := s.epochBoundaryStateCache.getByBlockRoot(fRoot)
		if err != nil {
			return err
		}
		if ok {
			s.SaveFinalizedState(fSlot, fRoot, fInfo.state)
		}
		return nil
	}

	blocks, err := s.loadBlocks(ctx, oldFSlot+1, fSlot, fRoot)
	if err != nil {
		return errors.Wrap(err, "could not load blocks for hdiff migration")
	}
	slices.SortFunc(blocks, func(a, b interfaces.ReadOnlySignedBeaconBlock) int {
		switch {
		case a.Block().Slot() < b.Block().Slot():
			return -1
		case a.Block().Slot() > b.Block().Slot():
			return 1
		default:
			return 0
		}
	})

	currState := oldFState.Copy()
	currRoot := oldFRoot
	nextBlockIdx := 0

	for _, slot := range slotsToSave {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Replay all canonical blocks up to target slot.
		for nextBlockIdx < len(blocks) && blocks[nextBlockIdx].Block().Slot() <= slot {
			currState, err = executeStateTransitionStateGen(ctx, currState, blocks[nextBlockIdx])
			if err != nil {
				return errors.Wrapf(err, "could not replay block at slot %d during hdiff migration", blocks[nextBlockIdx].Block().Slot())
			}
			currRoot, err = blocks[nextBlockIdx].Block().HashTreeRoot()
			if err != nil {
				return errors.Wrap(err, "could not compute block root during hdiff migration")
			}
			nextBlockIdx++
		}

		// If target slot is skipped, advance with process slots.
		if currState.Slot() < slot {
			currState, err = transition.ProcessSlots(ctx, currState, slot)
			if err != nil {
				return errors.Wrapf(err, "could not process slots to slot %d during hdiff migration", slot)
			}
		}
		if currState.Slot() != slot {
			return errors.Errorf("unexpected replay state slot %d while targeting %d", currState.Slot(), slot)
		}

		// Save to the finalized state-diff tree. This does not store unfinalized states in the diff tree
		// because migration only runs after finalized checkpoint advancement.
		if err := s.beaconDB.SaveState(ctx, currState, currRoot); err != nil {
			return err
		}
		s.migrateHotToCold(currRoot)
		log.WithFields(
			logrus.Fields{
				"slot": currState.Slot(),
				"root": fmt.Sprintf("%#x", currRoot),
			}).Info("Saved state in DB")
	}

	// Update finalized info in memory.
	fInfo, ok, err := s.epochBoundaryStateCache.getByBlockRoot(fRoot)
	if err != nil {
		return err
	}
	if ok {
		s.SaveFinalizedState(fSlot, fRoot, fInfo.state)
	}
	return nil
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
