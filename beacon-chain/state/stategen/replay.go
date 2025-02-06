package stategen

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v6/beacon-chain/core/transition"
	"github.com/prysmaticlabs/prysm/v6/beacon-chain/db/filters"
	"github.com/prysmaticlabs/prysm/v6/beacon-chain/state"
	"github.com/prysmaticlabs/prysm/v6/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v6/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v6/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v6/monitoring/tracing/trace"
	"github.com/sirupsen/logrus"
)

// ReplayBlocks replays the input blocks on the input state until the target slot is reached.
//
// WARNING Blocks passed to the function must be in decreasing slots order.
func (*State) replayBlocks(
	ctx context.Context,
	state state.BeaconState,
	signed []interfaces.ReadOnlySignedBeaconBlock,
	targetSlot primitives.Slot,
) (state.BeaconState, error) {
	ctx, span := trace.StartSpan(ctx, "stateGen.replayBlocks")
	defer span.End()
	var err error

	start := time.Now()
	rLog := log.WithFields(logrus.Fields{
		"startSlot": state.Slot(),
		"endSlot":   targetSlot,
		"diff":      targetSlot - state.Slot(),
	})
	rLog.Debug("Replaying state")
	// The input block list is sorted in decreasing slots order.
	if len(signed) > 0 {
		for i := len(signed) - 1; i >= 0; i-- {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if state.Slot() >= targetSlot {
				break
			}
			// A node shouldn't process the block if the block slot is lower than the state slot.
			if state.Slot() >= signed[i].Block().Slot() {
				continue
			}
			state, err = executeStateTransitionStateGen(ctx, state, signed[i])
			if err != nil {
				return nil, err
			}
		}
	}

	// If there are skip slots at the end.
	if targetSlot > state.Slot() {
		state, err = ReplayProcessSlots(ctx, state, targetSlot)
		if err != nil {
			return nil, err
		}
	}

	duration := time.Since(start)
	rLog.WithFields(logrus.Fields{
		"duration": duration,
	}).Debug("Replayed state")

	replayBlocksSummary.Observe(float64(duration.Milliseconds()))

	return state, nil
}

// loadBlocks loads the blocks between start slot and end slot by recursively fetching from end block root.
// The Blocks are returned in slot-descending order.
func (s *State) loadBlocks(ctx context.Context, startSlot, endSlot primitives.Slot, endBlockRoot [32]byte) ([]interfaces.ReadOnlySignedBeaconBlock, error) {
	// Nothing to load for invalid range.
	if startSlot > endSlot {
		return nil, fmt.Errorf("start slot %d > end slot %d", startSlot, endSlot)
	}
	filter := filters.NewFilter().SetStartSlot(startSlot).SetEndSlot(endSlot)
	blocks, blockRoots, err := s.beaconDB.Blocks(ctx, filter)
	if err != nil {
		return nil, err
	}
	// The retrieved blocks and block roots have to be in the same length given same filter.
	if len(blocks) != len(blockRoots) {
		return nil, errors.New("length of blocks and roots don't match")
	}
	// Return early if there's no block given the input.
	length := len(blocks)
	if length == 0 {
		return nil, nil
	}

	// The last retrieved block root has to match input end block root.
	// Covers the edge case if there's multiple blocks on the same end slot,
	// the end root may not be the last index in `blockRoots`.
	for length >= 3 && blocks[length-1].Block().Slot() == blocks[length-2].Block().Slot() && blockRoots[length-1] != endBlockRoot {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		length--
		if blockRoots[length-2] == endBlockRoot {
			length--
			break
		}
	}

	if blockRoots[length-1] != endBlockRoot {
		return nil, errors.New("end block roots don't match")
	}

	filteredBlocks := []interfaces.ReadOnlySignedBeaconBlock{blocks[length-1]}
	// Starting from second to last index because the last block is already in the filtered block list.
	for i := length - 2; i >= 0; i-- {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		b := filteredBlocks[len(filteredBlocks)-1]
		if b.Block().ParentRoot() != blockRoots[i] {
			continue
		}
		filteredBlocks = append(filteredBlocks, blocks[i])
	}

	return filteredBlocks, nil
}

// executeStateTransitionStateGen applies state transition on input historical state and block for state gen usages.
// There's no signature verification involved given state gen only works with stored block and state in DB.
// If the objects are already in stored in DB, one can omit redundant signature checks and ssz hashing calculations.
//
// WARNING: This method should not be used on an unverified new block.
func executeStateTransitionStateGen(
	ctx context.Context,
	state state.BeaconState,
	signed interfaces.ReadOnlySignedBeaconBlock,
) (state.BeaconState, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err := blocks.BeaconBlockIsNil(signed); err != nil {
		return nil, err
	}
	ctx, span := trace.StartSpan(ctx, "stategen.executeStateTransitionStateGen")
	defer span.End()
	var err error

	// Execute per slots transition.
	// Given this is for state gen, a node uses the version of process slots without skip slots cache.
	state, err = ReplayProcessSlots(ctx, state, signed.Block().Slot())
	if err != nil {
		return nil, errors.Wrap(err, "could not process slot")
	}

	// Execute per block transition.
	// Given this is for state gen, a node only cares about the post state without proposer
	// and randao signature verifications.
	state, err = transition.ProcessBlockForStateRoot(ctx, state, signed)
	if err != nil {
		return nil, errors.Wrap(err, "could not process block")
	}
	return state, nil
}

// ReplayProcessSlots to process old slots for state gen usages.
// There's no skip slot cache involved given state gen only works with already stored block and state in DB.
//
// WARNING: This method should not be used for future slot.
func ReplayProcessSlots(ctx context.Context, state state.BeaconState, slot primitives.Slot) (state.BeaconState, error) {
	ctx, span := trace.StartSpan(ctx, "stategen.ReplayProcessSlots")
	defer span.End()
	if state == nil || state.IsNil() {
		return nil, errUnknownState
	}
	if state.Slot() > slot {
		err := fmt.Errorf("expected state.slot %d <= slot %d", state.Slot(), slot)
		return nil, err
	}

	if state.Slot() == slot {
		return state, nil
	}

	return transition.ProcessSlotsCore(ctx, span, state, slot, nil)
}

// Given the start slot and the end slot, this returns the finalized beacon blocks in between.
// Since hot states don't have finalized blocks, this should ONLY be used for replaying cold state.
func (s *State) loadFinalizedBlocks(ctx context.Context, startSlot, endSlot primitives.Slot) ([]interfaces.ReadOnlySignedBeaconBlock, error) {
	f := filters.NewFilter().SetStartSlot(startSlot).SetEndSlot(endSlot)
	bs, bRoots, err := s.beaconDB.Blocks(ctx, f)
	if err != nil {
		return nil, err
	}
	if len(bs) != len(bRoots) {
		return nil, errors.New("length of blocks and roots don't match")
	}
	fbs := make([]interfaces.ReadOnlySignedBeaconBlock, 0, len(bs))
	for i := len(bs) - 1; i >= 0; i-- {
		if s.beaconDB.IsFinalizedBlock(ctx, bRoots[i]) {
			fbs = append(fbs, bs[i])
		}
	}
	return fbs, nil
}
