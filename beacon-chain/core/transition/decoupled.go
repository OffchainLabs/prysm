package transition

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/blocks"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/decoupled"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/electra"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/gloas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	v "github.com/OffchainLabs/prysm/v7/beacon-chain/core/validators"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	"github.com/pkg/errors"
)

// Spec: process_operations from the old spec. Slashings and both vote streams route through the Decoupled package, the rest is the Gloas order.
func decoupledOperations(ctx context.Context, st state.BeaconState, block interfaces.ReadOnlyBeaconBlock) (state.BeaconState, error) {
	ctx, span := trace.StartSpan(ctx, "core.state.decoupledOperations")
	defer span.End()

	bb := block.Body()
	atts, err := bb.AttestationsDecoupled()
	if err != nil {
		return nil, err
	}
	slashings, err := bb.AttesterSlashingsDecoupled()
	if err != nil {
		return nil, err
	}
	availableAtts, err := bb.AvailableAttestations()
	if err != nil {
		return nil, err
	}

	var exitInfo *v.ExitInfo
	if len(bb.ProposerSlashings()) > 0 || len(slashings) > 0 || len(bb.VoluntaryExits()) > 0 {
		exitInfo = v.ExitInformation(st)
		if err := helpers.UpdateTotalActiveBalanceCache(st, exitInfo.TotalActiveBalance); err != nil {
			return nil, errors.Wrap(err, "could not update total active balance cache")
		}
	}
	st, err = decoupled.ProcessProposerSlashings(ctx, st, bb.ProposerSlashings(), exitInfo)
	if err != nil {
		return nil, errors.Wrap(ErrProcessProposerSlashingsFailed, err.Error())
	}
	st, err = decoupled.ProcessAttesterSlashings(ctx, st, slashings, exitInfo)
	if err != nil {
		return nil, errors.Wrap(ErrProcessAttesterSlashingsFailed, err.Error())
	}
	st, err = decoupled.ProcessAttestationsNoVerifySignature(ctx, st, atts)
	if err != nil {
		return nil, errors.Wrap(ErrProcessAttestationsFailed, err.Error())
	}
	if _, err := electra.ProcessDeposits(ctx, st, bb.Deposits()); err != nil {
		return nil, errors.Wrap(ErrProcessDepositsFailed, err.Error())
	}
	st, err = blocks.ProcessVoluntaryExits(ctx, st, bb.VoluntaryExits(), exitInfo)
	if err != nil {
		return nil, errors.Wrap(ErrProcessVoluntaryExitsFailed, err.Error())
	}
	st, err = blocks.ProcessBLSToExecutionChanges(st, block)
	if err != nil {
		return nil, errors.Wrap(ErrProcessBLSChangesFailed, err.Error())
	}
	if err := gloas.ProcessPayloadAttestations(ctx, st, bb); err != nil {
		return nil, errors.Wrap(ErrProcessPayloadAttestationsFailed, err.Error())
	}
	if err := decoupled.ProcessAvailableAttestations(ctx, st, availableAtts); err != nil {
		return nil, errors.Wrap(err, "could not process available attestations")
	}
	return st, nil
}
