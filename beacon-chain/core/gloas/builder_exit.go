package gloas

import (
	"bytes"
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

// InitiateBuilderExit initiates the exit of a builder by setting its withdrawable epoch.
//
//	<spec fn="initiate_builder_exit" fork="gloas" hash="f71d22b9">
//	def initiate_builder_exit(state: BeaconState, builder_index: BuilderIndex) -> None:
//	    """
//	    Initiate the exit of the builder with index ``index``.
//	    """
//	    # Set builder exit epoch
//	    builder = state.builders[builder_index]
//	    builder.withdrawable_epoch = get_current_epoch(state) + MIN_BUILDER_WITHDRAWABILITY_DELAY
//	</spec>
func InitiateBuilderExit(s state.BeaconState, builderIndex primitives.BuilderIndex) error {
	builder, err := s.Builder(builderIndex)
	if err != nil {
		return err
	}
	// Return if builder already initiated exit.
	if builder.WithdrawableEpoch != params.BeaconConfig().FarFutureEpoch {
		return nil
	}
	currentEpoch := slots.ToEpoch(s.Slot())
	builder.WithdrawableEpoch = currentEpoch + params.BeaconConfig().MinBuilderWithdrawabilityDelay
	if err := s.UpdateBuilderAtIndex(builderIndex, builder); err != nil {
		return err
	}
	builderExitsProcessedTotal.Inc()
	return nil
}

// ProcessBuilderExitRequests applies each builder exit request in order.
func ProcessBuilderExitRequests(ctx context.Context, s state.BeaconState, requests []*enginev1.BuilderExitRequest) error {
	for _, request := range requests {
		if err := processBuilderExitRequest(s, request); err != nil {
			return errors.Wrap(err, "could not process builder exit request")
		}
	}
	return nil
}

// processBuilderExitRequest initiates a builder exit when the request originates
// from the builder's execution address and the builder has no pending withdrawals.
//
//	<spec fn="process_builder_exit_request" fork="gloas" hash="144e9faf">
//	def process_builder_exit_request(state: BeaconState, request: BuilderExitRequest) -> None:
//	    builder_pubkeys = [b.pubkey for b in state.builders]
//	    if request.pubkey not in builder_pubkeys:
//	        return
//
//	    builder_index = BuilderIndex(builder_pubkeys.index(request.pubkey))
//	    builder = state.builders[builder_index]
//
//	    if not is_active_builder(state, builder_index):
//	        return
//	    if builder.execution_address != request.source_address:
//	        return
//	    if get_pending_balance_to_withdraw_for_builder(state, builder_index) != 0:
//	        return
//
//	    initiate_builder_exit(state, builder_index)
//	</spec>
func processBuilderExitRequest(s state.BeaconState, request *enginev1.BuilderExitRequest) error {
	if request == nil {
		return errors.New("nil builder exit request")
	}

	idx, isBuilder := s.BuilderIndexByPubkey(bytesutil.ToBytes48(request.Pubkey))
	if !isBuilder {
		return nil
	}

	active, err := s.IsActiveBuilder(idx)
	if err != nil {
		return errors.Wrap(err, "could not check if builder is active")
	}
	if !active {
		return nil
	}

	builder, err := s.Builder(idx)
	if err != nil {
		return err
	}
	if !bytes.Equal(builder.ExecutionAddress, request.SourceAddress) {
		return nil
	}

	pendingBalance, err := s.BuilderPendingBalanceToWithdraw(idx)
	if err != nil {
		return errors.Wrap(err, "could not get builder pending balance to withdraw")
	}
	if pendingBalance != 0 {
		return nil
	}

	if err := InitiateBuilderExit(s, idx); err != nil {
		return err
	}
	builderExitsProcessedTotal.Inc()
	return nil
}
