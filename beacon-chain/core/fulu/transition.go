package fulu

import (
	"context"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/electra"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/pkg/errors"
)

func ProcessEpoch(ctx context.Context, state state.BeaconState) error {
	if err := electra.ProcessEpoch(ctx, state); err != nil {
		return errors.Wrap(err, "could not process epoch in fulu transition")
	}
	return ProcessProposerLookahead(ctx, state)
}

func ProcessProposerLookahead(ctx context.Context, state state.BeaconState) error {
	_, span := trace.StartSpan(ctx, "fulu.processProposerLookahead")
	defer span.End()

	if state == nil || state.IsNil() {
		return errors.New("nil state")
	}

	lookAhead, err := state.ProposerLookahead()
	if err != nil {
		return errors.Wrap(err, "could not get proposer lookahead")
	}
	lastEpochStart := len(lookAhead) - int(params.BeaconConfig().SlotsPerEpoch)
	copy(lookAhead[:lastEpochStart], lookAhead[params.BeaconConfig().SlotsPerEpoch:])
	lastEpoch := slots.ToEpoch(state.Slot()) + params.BeaconConfig().MinSeedLookahead + 1
	indices, err := helpers.ActiveValidatorIndices(ctx, state, lastEpoch)
	if err != nil {
		return err
	}
	lastEpochProposers, err := helpers.PrecomputeProposerIndices(state, indices, lastEpoch)
	if err != nil {
		return errors.Wrap(err, "could not precompute proposer indices")
	}
	copy(lookAhead[lastEpochStart:], lastEpochProposers)
	return state.SetProposerLookahead(lookAhead)
}
