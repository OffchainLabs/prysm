package gloas

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	"github.com/pkg/errors"
)

// ProcessBuilderDepositRequests applies each builder deposit request in order.
func ProcessBuilderDepositRequests(ctx context.Context, st state.BeaconState, requests []*enginev1.BuilderDepositRequest) error {
	for _, request := range requests {
		if err := processBuilderDepositRequest(st, request); err != nil {
			return errors.Wrap(err, "could not process builder deposit request")
		}
	}
	return nil
}

// processBuilderDepositRequest registers a new builder or tops up an existing one.
//
//	<spec fn="process_builder_deposit_request" fork="gloas" hash="todo">
//	def process_builder_deposit_request(state: BeaconState, request: BuilderDepositRequest) -> None:
//	    builder_pubkeys = [b.pubkey for b in state.builders]
//	    if request.pubkey not in builder_pubkeys:
//	        if is_valid_builder_deposit_signature(request):
//	            add_builder_to_registry(
//	                state,
//	                request.pubkey,
//	                uint8(request.withdrawal_credentials[0]),
//	                ExecutionAddress(request.withdrawal_credentials[12:]),
//	                request.amount,
//	                state.slot,
//	            )
//	    else:
//	        builder_index = builder_pubkeys.index(request.pubkey)
//	        state.builders[builder_index].balance += request.amount
//	</spec>
func processBuilderDepositRequest(st state.BeaconState, request *enginev1.BuilderDepositRequest) error {
	if request == nil {
		return errors.New("nil builder deposit request")
	}

	pubkey := bytesutil.ToBytes48(request.Pubkey)
	if idx, isBuilder := st.BuilderIndexByPubkey(pubkey); isBuilder {
		if err := st.IncreaseBuilderBalance(idx, request.Amount); err != nil {
			return err
		}
		builderDepositsProcessedTotal.Inc()
		return nil
	}

	valid, err := helpers.IsValidBuilderDepositSignature(request)
	if err != nil {
		return errors.Wrap(err, "could not verify builder deposit signature")
	}
	if !valid {
		return nil
	}

	if err := st.AddBuilderFromDeposit(pubkey, bytesutil.ToBytes32(request.WithdrawalCredentials), request.Amount); err != nil {
		return err
	}
	builderDepositsProcessedTotal.Inc()
	return nil
}
