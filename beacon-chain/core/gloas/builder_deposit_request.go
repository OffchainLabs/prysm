package gloas

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

type sigStatus int

const (
	sigUnverified sigStatus = iota
	sigKnownValid
	sigKnownInvalid
)

// ProcessBuilderDepositRequests applies each builder deposit request in order.
func ProcessBuilderDepositRequests(ctx context.Context, st state.BeaconState, requests []*enginev1.BuilderDepositRequest) error {
	// Topups to an existing builder skip signature verification per spec, so only batch-verify
	// deposits that would register a new builder.
	newDeposits := make([]*enginev1.BuilderDepositRequest, 0, len(requests))
	newIdx := make([]int, 0, len(requests))
	for i, request := range requests {
		if request == nil || !helpers.IsBuilderWithdrawalCredential(request.WithdrawalCredentials) {
			continue
		}
		if _, isBuilder := st.BuilderIndexByPubkey(bytesutil.ToBytes48(request.Pubkey)); !isBuilder {
			newDeposits = append(newDeposits, request)
			newIdx = append(newIdx, i)
		}
	}
	invalid, err := helpers.BatchVerifyBuilderDepositRequestSignatures(ctx, newDeposits)
	if err != nil {
		return err
	}
	statuses := make([]sigStatus, len(requests))
	for _, i := range newIdx {
		statuses[i] = sigKnownValid
	}
	for _, j := range invalid {
		statuses[newIdx[j]] = sigKnownInvalid
	}
	for i, request := range requests {
		if err := processBuilderDepositRequest(ctx, st, request, statuses[i]); err != nil {
			return errors.Wrap(err, "could not process builder deposit request")
		}
	}
	return nil
}

// processBuilderDepositRequest registers a new builder or tops up an existing one.
//
//	<spec fn="process_builder_deposit_request" fork="gloas" hash="c3ba5adf">
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
//	        builder_index = BuilderIndex(builder_pubkeys.index(request.pubkey))
//	        builder = state.builders[builder_index]
//
//	        # Increase balance by deposit amount
//	        builder.balance += request.amount
//
//	        # If exited, reset the withdrawable epoch
//	        if builder.withdrawable_epoch != FAR_FUTURE_EPOCH:
//	            epoch = get_current_epoch(state)
//	            builder.withdrawable_epoch = epoch + MIN_BUILDER_WITHDRAWABILITY_DELAY
//	</spec>
func processBuilderDepositRequest(ctx context.Context, st state.BeaconState, request *enginev1.BuilderDepositRequest, status sigStatus) error {
	if request == nil {
		return errors.New("nil builder deposit request")
	}

	// Deposits without BUILDER_WITHDRAWAL_PREFIX are dropped and the funds are lost.
	if !helpers.IsBuilderWithdrawalCredential(request.WithdrawalCredentials) {
		return nil
	}

	pubkey := bytesutil.ToBytes48(request.Pubkey)
	if idx, isBuilder := st.BuilderIndexByPubkey(pubkey); isBuilder {
		builder, err := st.Builder(idx)
		if err != nil {
			return err
		}
		if builder == nil {
			return errors.Errorf("builder at index %d is nil", idx)
		}
		// Reset only when exited and fully swept, checked before the top-up is credited.
		if builder.WithdrawableEpoch != params.BeaconConfig().FarFutureEpoch && builder.Balance == 0 {
			builder.WithdrawableEpoch = slots.ToEpoch(st.Slot()) + params.BeaconConfig().MinBuilderWithdrawabilityDelay
			if err := st.UpdateBuilderAtIndex(idx, builder); err != nil {
				return err
			}
		}
		if err := st.IncreaseBuilderBalance(idx, request.Amount); err != nil {
			return err
		}
		builderDepositsProcessedTotal.Inc()
		return nil
	}

	// An in-batch index reuse can evict a pubkey that was a top-up at classification time, so
	// an unverified request reaching the registration path must have its signature checked here.
	if status == sigUnverified {
		valid, err := helpers.VerifyBuilderDepositRequestSignature(ctx, request)
		if err != nil {
			return err
		}
		if valid {
			status = sigKnownValid
		} else {
			status = sigKnownInvalid
		}
	}

	if status != sigKnownValid {
		return nil
	}

	if err := st.AddBuilderFromDeposit(pubkey, bytesutil.ToBytes32(request.WithdrawalCredentials), request.Amount); err != nil {
		return err
	}
	builderDepositsProcessedTotal.Inc()
	return nil
}
