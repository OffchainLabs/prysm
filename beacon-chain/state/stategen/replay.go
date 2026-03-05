package stategen

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/gloas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db/filters"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// ReplayBlocks replays the input blocks on the input state until the target slot is reached.
func (s *State) replayBlocks(
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
	for _, blk := range signed {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if state.Slot() >= targetSlot {
			break
		}
		// A node shouldn't process the block if the block slot is lower than the state slot.
		if state.Slot() >= blk.Block().Slot() {
			continue
		}

		var envelope *ethpb.SignedBlindedExecutionPayloadEnvelope
		if blk.Version() >= version.Gloas {
			root, err := blk.Block().HashTreeRoot()
			if err != nil {
				return nil, errors.Wrap(err, "could not compute block root for execution payload envelope lookup")
			}
			envelope, err = s.beaconDB.ExecutionPayloadEnvelope(ctx, root)
			if err != nil && !errors.Is(err, db.ErrNotFound) {
				return nil, errors.Wrap(err, "could not retrieve execution payload envelope")
			}
		}

		state, err = executeStateTransitionStateGen(ctx, state, blk, envelope)
		if err != nil {
			return nil, err
		}
		if blk.Block().Slot() < targetSlot {
			state, err = s.applyBlindedExecutionPayloadEnvelopeStateGen(ctx, state, blk)
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

func (s *State) applyBlindedExecutionPayloadEnvelopeStateGen(
	ctx context.Context,
	st state.BeaconState,
	signed interfaces.ReadOnlySignedBeaconBlock,
) (state.BeaconState, error) {
	if st == nil || st.IsNil() || st.Version() < version.Gloas || signed.Block().Version() < version.Gloas {
		return st, nil
	}

	blockRoot, err := signed.Block().HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "could not compute block root for blinded envelope lookup")
	}
	if !s.beaconDB.HasExecutionPayloadEnvelope(ctx, blockRoot) {
		return st, nil
	}

	blindedEnvelope, err := s.beaconDB.ExecutionPayloadEnvelope(ctx, blockRoot)
	if err != nil {
		return nil, errors.Wrapf(err, "could not load execution payload envelope for block %#x", blockRoot)
	}
	if err := gloas.ApplyBlindedExecutionPayloadEnvelopeForStateGen(ctx, st, blindedEnvelope); err != nil {
		return nil, errors.Wrapf(err, "could not apply execution payload envelope for block %#x", blockRoot)
	}
	return st, nil
}

// loadBlocks loads the blocks between start slot and end slot by recursively fetching from end block root.
// The Blocks are returned in slot-descending order.
func (s *State) loadBlocks(ctx context.Context, startSlot, endSlot primitives.Slot, endBlockRoot [32]byte) ([]interfaces.ReadOnlySignedBeaconBlock, error) {
	// Nothing to load for invalid range.
	if startSlot > endSlot {
		return nil, fmt.Errorf("start slot %d > end slot %d", startSlot, endSlot)
	}
	query := filters.AncestryQuery{Earliest: startSlot, Descendent: filters.SlotRoot{Slot: endSlot, Root: endBlockRoot}}
	filter := filters.NewFilter().SetAncestryQuery(query)
	blocks, _, err := s.beaconDB.Blocks(ctx, filter)
	if err != nil {
		return nil, err
	}
	return blocks, nil
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
	blindedEnvelope *ethpb.SignedBlindedExecutionPayloadEnvelope,
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
	preTransitionState := state

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
		fields := logrus.Fields{
			"blockSlot":    signed.Block().Slot(),
			"parentRoot":   fmt.Sprintf("%#x", signed.Block().ParentRoot()),
			"blockVersion": signed.Block().Version(),
		}
		if preTransitionState != nil && !preTransitionState.IsNil() {
			fields["stateSlot"] = preTransitionState.Slot()
		}
		if preTransitionState != nil && !preTransitionState.IsNil() && preTransitionState.Version() >= version.Gloas {
			latestHash, hashErr := preTransitionState.LatestBlockHash()
			if hashErr == nil {
				fields["stateLatestBlockHash"] = fmt.Sprintf("%#x", latestHash)
			}
		}
		if signed.Block().Version() >= version.Gloas {
			signedBid, bidErr := signed.Block().Body().SignedExecutionPayloadBid()
			if bidErr == nil && signedBid != nil && signedBid.Message != nil && len(signedBid.Message.ParentBlockHash) == 32 {
				fields["bidParentBlockHash"] = fmt.Sprintf("%#x", [32]byte(signedBid.Message.ParentBlockHash))
			}
		}
		log.WithError(err).WithFields(fields).Debug("Failed to process block during stategen replay")
		return nil, errors.Wrap(err, "could not process block")
	}

	if state.Version() >= version.Gloas && blindedEnvelope != nil && blindedEnvelope.Message != nil {
		latestHeader := state.LatestBlockHeader()
		if len(latestHeader.StateRoot) == 0 || bytes.Equal(latestHeader.StateRoot, make([]byte, 32)) {
			previousStateRoot, err := state.HashTreeRoot(ctx)
			if err != nil {
				return nil, errors.Wrap(err, "could not compute state root")
			}
			latestHeader.StateRoot = previousStateRoot[:]
			if err := state.SetLatestBlockHeader(latestHeader); err != nil {
				return nil, errors.Wrap(err, "could not set latest block header")
			}
		}

		if err := gloas.ApplyExecutionPayloadStateMutations(
			ctx,
			state,
			blindedEnvelope.Message.ExecutionRequests,
			[32]byte(blindedEnvelope.Message.BlockHash),
		); err != nil {
			return nil, errors.Wrap(err, "could not apply execution payload state mutations")
		}
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
