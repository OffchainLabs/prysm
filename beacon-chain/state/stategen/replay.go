package stategen

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/gloas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
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

	// For Gloas states: if the first replay block's bid parentBlockHash doesn't
	// match the ancestor state's latestBlockHash, the ancestor block's execution
	// payload was delivered but the state is post-CL (EL not yet applied). Apply
	// the ancestor's envelope to bring the state to post-EL before replaying.
	if len(signed) > 0 && state.Version() >= version.Gloas && signed[0].Block().Version() >= version.Gloas {
		if err := s.maybeApplyAncestorEnvelope(ctx, state, signed[0]); err != nil {
			return nil, errors.Wrap(err, "could not apply ancestor execution payload envelope")
		}
	}

	for i, blk := range signed {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		stateSlot := state.Slot()
		if stateSlot >= targetSlot {
			break
		}
		slot := blk.Block().Slot()
		if stateSlot >= slot {
			continue
		}

		var envelope *ethpb.SignedBlindedExecutionPayloadEnvelope
		if i < len(signed)-1 && blk.Block().Version() >= version.Gloas {
			bid, err := blk.Block().Body().SignedExecutionPayloadBid()
			if err != nil {
				return nil, errors.Wrapf(err, "could not get execution payload bid for block at slot %d", blk.Block().Slot())
			}
			if bid == nil || bid.Message == nil {
				return nil, fmt.Errorf("missing execution payload bid for block at slot %d", blk.Block().Slot())
			}
			child := signed[i+1].Block()
			childBid, err := child.Body().SignedExecutionPayloadBid()
			if err != nil {
				return nil, errors.Wrapf(err, "could not get execution payload bid for block at slot %d", child.Slot())
			}
			if childBid == nil || childBid.Message == nil {
				return nil, fmt.Errorf("missing execution payload bid for block at slot %d", child.Slot())
			}
			if bytes.Equal(childBid.Message.ParentBlockHash, bid.Message.BlockHash) {
				root := child.ParentRoot()
				envelope, err = s.beaconDB.ExecutionPayloadEnvelope(ctx, root)
				if err != nil {
					return nil, errors.Wrapf(err, "could not retrieve execution payload envelope for block with root %#x at slot %d", root, slot)
				}
			}
		}
		state, err = executeStateTransitionStateGen(ctx, state, blk)
		if err != nil {
			return nil, errors.Wrapf(err, "could not execute state transition for block at slot %d", slot)
		}
		if envelope != nil && envelope.Message != nil {
			wrappedEnvelope, err := blocks.WrappedROBlindedExecutionPayloadEnvelope(envelope.Message)
			if err != nil {
				return nil, errors.Wrapf(err, "could not wrap blinded execution payload envelope for block at slot %d", slot)
			}
			if err := gloas.ApplyBlindedExecutionPayloadEnvelopeForStateGen(ctx, state, blk.Block().StateRoot(), wrappedEnvelope); err != nil {
				return nil, errors.Wrapf(err, "could not apply execution payload envelope for block at slot %d", slot)
			}
		}
	}

	duration := time.Since(start)
	rLog.WithFields(logrus.Fields{
		"duration": duration,
	}).Debug("Replayed state")

	replayBlocksSummary.Observe(float64(duration.Milliseconds()))

	return state, nil
}

// maybeApplyAncestorEnvelope checks whether the ancestor state needs its
// execution payload envelope applied before block replay can proceed. This is
// needed when the ancestor state is post-CL (latestBlockHash not yet updated
// with the delivered payload). The check compares the first replay block's bid
// parentBlockHash with the state's latestBlockHash: a mismatch means the
// ancestor's payload was delivered but the state doesn't reflect it.
func (s *State) maybeApplyAncestorEnvelope(
	ctx context.Context,
	st state.BeaconState,
	firstBlock interfaces.ReadOnlySignedBeaconBlock,
) error {
	firstBid, err := firstBlock.Block().Body().SignedExecutionPayloadBid()
	if err != nil || firstBid == nil || firstBid.Message == nil {
		return nil
	}
	latestHash, err := st.LatestBlockHash()
	if err != nil {
		return err
	}
	if bytes.Equal(firstBid.Message.ParentBlockHash, latestHash[:]) {
		return nil
	}
	// The first block expects a different latestBlockHash than what the state
	// has: the ancestor's execution payload was delivered but the state is
	// post-CL. Apply the ancestor's envelope.
	ancestorRoot := firstBlock.Block().ParentRoot()
	envelope, err := s.beaconDB.ExecutionPayloadEnvelope(ctx, ancestorRoot)
	if err != nil {
		return errors.Wrapf(err, "could not retrieve execution payload envelope for ancestor root %#x", ancestorRoot)
	}
	if envelope == nil || envelope.Message == nil {
		return errors.Errorf("Received nil execution payload envelope for ancestor root %#x", ancestorRoot)
	}
	ancestorBlock, err := s.beaconDB.Block(ctx, ancestorRoot)
	if err != nil {
		return errors.Wrapf(err, "could not retrieve ancestor block for root %#x", ancestorRoot)
	}
	wrappedEnvelope, err := blocks.WrappedROBlindedExecutionPayloadEnvelope(envelope.Message)
	if err != nil {
		return errors.Wrap(err, "could not wrap ancestor blinded execution payload envelope")
	}
	return gloas.ApplyBlindedExecutionPayloadEnvelopeForStateGen(ctx, st, ancestorBlock.Block().StateRoot(), wrappedEnvelope)
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
	st state.BeaconState,
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
	st, err = ReplayProcessSlots(ctx, st, signed.Block().Slot())
	if err != nil {
		return nil, errors.Wrap(err, "could not process slot")
	}

	// Execute per block transition.
	// Given this is for state gen, a node only cares about the post state without proposer
	// and randao signature verifications.
	st, err = transition.ProcessBlockForStateRoot(ctx, st, signed)
	if err == nil {
		return st, nil
	}
	fields := logrus.Fields{
		"blockSlot":    signed.Block().Slot(),
		"parentRoot":   fmt.Sprintf("%#x", signed.Block().ParentRoot()),
		"blockVersion": signed.Block().Version(),
	}
	if st != nil && !st.IsNil() {
		fields["stateSlot"] = st.Slot()
		if st.Version() >= version.Gloas {
			latestHash, hashErr := st.LatestBlockHash()
			if hashErr == nil {
				fields["stateLatestBlockHash"] = fmt.Sprintf("%#x", latestHash)
			}
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
