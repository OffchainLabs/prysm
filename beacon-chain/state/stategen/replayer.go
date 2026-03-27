package stategen

import (
	"context"
	"fmt"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/gloas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db"
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

var ErrFutureSlotRequested = errors.New("cannot replay to future slots")
var ErrNoCanonicalBlockForSlot = errors.New("none of the blocks found in the db slot index are canonical")
var ErrNoBlocksBelowSlot = errors.New("no blocks found in db below slot")
var ErrReplayTargetSlotExceeded = errors.New("desired replay slot is less than state's slot")

type retrievalMethod int

const (
	forSlot retrievalMethod = iota
)

// HistoryAccessor describes the minimum set of database methods needed to support the ReplayerBuilder.
type HistoryAccessor interface {
	HighestRootsBelowSlot(ctx context.Context, slot primitives.Slot) (primitives.Slot, [][32]byte, error)
	GenesisBlockRoot(ctx context.Context) ([32]byte, error)
	Block(ctx context.Context, blockRoot [32]byte) (interfaces.ReadOnlySignedBeaconBlock, error)
	StateOrError(ctx context.Context, blockRoot [32]byte) (state.BeaconState, error)
}

// CanonicalChecker determines whether the given block root is canonical.
// In practice this should be satisfied by a type that uses the fork choice store.
type CanonicalChecker interface {
	IsCanonical(ctx context.Context, blockRoot [32]byte) (bool, error)
}

// CurrentSlotter provides the current Slot.
type CurrentSlotter interface {
	CurrentSlot() primitives.Slot
}

// Replayer encapsulates database query and replay logic. It can be constructed via a ReplayerBuilder.
type Replayer interface {
	// ReplayBlocks replays the blocks the Replayer knows about based on Builder params
	ReplayBlocks(ctx context.Context) (state.BeaconState, error)
	// ReplayToSlot invokes ReplayBlocks under the hood,
	// but then also runs process_slots to advance the state past the root or slot used in the builder.
	// For example, if you wanted the state to be at the target slot, but only integrating blocks up to
	// slot-1, you could request Builder.ReplayerForSlot(slot-1).ReplayToSlot(slot)
	ReplayToSlot(ctx context.Context, target primitives.Slot) (state.BeaconState, error)
}

var _ Replayer = &stateReplayer{}

// chainer is responsible for supplying the chain components necessary to rebuild a state,
// namely a starting BeaconState and all available blocks from the starting state up to and including the target slot
type chainer interface {
	chainForSlot(ctx context.Context, target primitives.Slot) (state.BeaconState, []interfaces.ReadOnlySignedBeaconBlock, error)
}

type executionPayloadEnvelopeProvider interface {
	executionPayloadEnvelope(ctx context.Context, blockRoot [32]byte) (*ethpb.SignedBlindedExecutionPayloadEnvelope, error)
}

type stateReplayer struct {
	target  primitives.Slot
	method  retrievalMethod
	chainer chainer
}

// ReplayBlocks applies all the blocks that were accumulated when building the Replayer.
// This method relies on the correctness of the code that constructed the Replayer data.
func (rs *stateReplayer) ReplayBlocks(ctx context.Context) (state.BeaconState, error) {
	ctx, span := trace.StartSpan(ctx, "stateGen.stateReplayer.ReplayBlocks")
	defer span.End()

	var s state.BeaconState
	var descendants []interfaces.ReadOnlySignedBeaconBlock
	var err error
	switch rs.method {
	case forSlot:
		s, descendants, err = rs.chainer.chainForSlot(ctx, rs.target)
		if err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("Replayer initialized using unknown state retrieval method")
	}

	start := time.Now()
	diff, err := rs.target.SafeSubSlot(s.Slot())
	if err != nil {
		msg := fmt.Sprintf("error subtracting state.slot %d from replay target slot %d", s.Slot(), rs.target)
		return nil, errors.Wrap(err, msg)
	}
	if diff == 0 {
		return s, nil
	}

	log.WithFields(logrus.Fields{
		"startSlot": s.Slot(),
		"endSlot":   rs.target,
		"diff":      diff,
	}).Debug("Replaying canonical blocks from most recent state")

	for i, b := range descendants {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		s, err = executeStateTransitionStateGen(ctx, s, b)
		if err != nil {
			return nil, errors.Wrap(err, "could not execute state transition")
		}

		// Apply the envelope for all blocks except the last one.
		// The caller is responsible for applying the envelope on the last block if needed.
		if i < len(descendants)-1 && b.Version() >= version.Gloas {
			if p, ok := rs.chainer.(executionPayloadEnvelopeProvider); ok {
				root, err := b.Block().HashTreeRoot()
				if err != nil {
					return nil, errors.Wrap(err, "could not compute block root for execution payload envelope lookup")
				}
				signedEnvelope, err := p.executionPayloadEnvelope(ctx, root)
				if err != nil && !errors.Is(err, db.ErrNotFound) {
					return nil, errors.Wrap(err, "could not retrieve execution payload envelope")
				}
				if signedEnvelope != nil && signedEnvelope.Message != nil {
					envelope, err := blocks.WrappedROBlindedExecutionPayloadEnvelope(signedEnvelope.Message)
					if err != nil {
						return nil, errors.Wrap(err, "could not wrap blinded execution payload envelope")
					}
					if err := gloas.ApplyBlindedExecutionPayloadEnvelopeForStateGen(ctx, s, b.Block().StateRoot(), envelope); err != nil {
						return nil, errors.Wrap(err, "could not apply execution payload envelope")
					}
				}
			}
		}
	}
	if rs.target > s.Slot() {
		s, err = ReplayProcessSlots(ctx, s, rs.target)
		if err != nil {
			return nil, err
		}
	}

	duration := time.Since(start)
	log.WithFields(logrus.Fields{
		"duration": duration,
	}).Debug("Finished calling process_blocks on all blocks in ReplayBlocks")
	replayBlocksSummary.Observe(float64(duration.Milliseconds()))
	return s, nil
}

// ReplayToSlot invokes ReplayBlocks under the hood,
// but then also runs process_slots to advance the state past the root or slot used in the builder.
// for example, if you wanted the state to be at the target slot, but only integrating blocks up to
// slot-1, you could request Builder.ReplayerForSlot(slot-1).ReplayToSlot(slot)
func (rs *stateReplayer) ReplayToSlot(ctx context.Context, replayTo primitives.Slot) (state.BeaconState, error) {
	ctx, span := trace.StartSpan(ctx, "stateGen.stateReplayer.ReplayToSlot")
	defer span.End()

	s, err := rs.ReplayBlocks(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to ReplayBlocks")
	}
	if replayTo < s.Slot() {
		return nil, errors.Wrapf(ErrReplayTargetSlotExceeded, "slot desired=%d, state.slot=%d", replayTo, s.Slot())
	}
	if replayTo == s.Slot() {
		return s, nil
	}

	start := time.Now()
	log.WithFields(logrus.Fields{
		"startSlot": s.Slot(),
		"endSlot":   replayTo,
		"diff":      replayTo - s.Slot(),
	}).Debug("Calling process_slots on remaining slots")

	// err will be handled after the bookend log
	s, err = ReplayProcessSlots(ctx, s, replayTo)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("ReplayToSlot failed to seek to slot %d after applying blocks", replayTo))
	}
	duration := time.Since(start)
	log.WithFields(logrus.Fields{
		"duration": duration,
	}).Debug("Time spent in process_slots")
	replayToSlotSummary.Observe(float64(duration.Milliseconds()))

	return s, nil
}

// ReplayerBuilder creates a Replayer that can be used to obtain a state at a specified slot or root
// (only ForSlot implemented so far).
// See documentation on Replayer for more on how to use this to obtain pre/post-block states
type ReplayerBuilder interface {
	// ReplayerForSlot creates a builder that will create a state that includes blocks up to and including the requested slot
	// The resulting Replayer will always yield a state with .Slot=target; if there are skipped blocks
	// between the highest canonical block in the db and the target, the replayer will fast-forward past the intervening
	// slots via process_slots.
	ReplayerForSlot(target primitives.Slot) Replayer
}
