package gloas

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

// ProcessProposerLookahead advances the cached proposer lookahead by one epoch
// using EIP-8045 semantics: slashed validators are excluded from the candidate
// pool used to derive the new last-epoch proposer indices.
func ProcessProposerLookahead(ctx context.Context, state state.BeaconState) error {
	_, span := trace.StartSpan(ctx, "gloas.processProposerLookahead")
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
	indices, err := helpers.ActiveNonSlashedValidatorIndices(ctx, state, lastEpoch)
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
