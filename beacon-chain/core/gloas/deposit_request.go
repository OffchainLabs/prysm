package gloas

import (
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

func processDepositRequests(ctx context.Context, beaconState state.BeaconState, requests []*enginev1.DepositRequest) error {
	if len(requests) == 0 {
		return nil
	}

	for _, receipt := range requests {
		if err := processDepositRequest(beaconState, receipt); err != nil {
			return errors.Wrap(err, "could not apply deposit request")
		}
	}
	return nil
}

// processDepositRequest processes the specific deposit request
//
//	<spec fn="process_deposit_request" fork="gloas" hash="0e8b94ab">
//	def process_deposit_request(state: BeaconState, deposit_request: DepositRequest) -> None:
//	    # [New in Gloas:EIP7732]
//	    builder_pubkeys = [b.pubkey for b in state.builders]
//	    validator_pubkeys = [v.pubkey for v in state.validators]
//
//	    # [New in Gloas:EIP7732]
//	    # Regardless of the withdrawal credentials prefix, if a builder/validator
//	    # already exists with this pubkey, apply the deposit to their balance
//	    is_builder = deposit_request.pubkey in builder_pubkeys
//	    is_validator = deposit_request.pubkey in validator_pubkeys
//	    if is_builder or (
//	        is_builder_withdrawal_credential(deposit_request.withdrawal_credentials)
//	        and not is_validator
//	        and not is_pending_validator(state, deposit_request.pubkey)
//	    ):
//	        # Apply builder deposits immediately
//	        apply_deposit_for_builder(
//	            state,
//	            deposit_request.pubkey,
//	            deposit_request.withdrawal_credentials,
//	            deposit_request.amount,
//	            deposit_request.signature,
//	            state.slot,
//	        )
//	        return
//
//	    # Add validator deposits to the queue
//	    state.pending_deposits.append(
//	        PendingDeposit(
//	            pubkey=deposit_request.pubkey,
//	            withdrawal_credentials=deposit_request.withdrawal_credentials,
//	            amount=deposit_request.amount,
//	            signature=deposit_request.signature,
//	            slot=state.slot,
//	        )
//	    )
//	</spec>
func processDepositRequest(beaconState state.BeaconState, request *enginev1.DepositRequest) error {
	if request == nil {
		return errors.New("nil deposit request")
	}

	applied, err := applyBuilderDepositRequest(beaconState, request)
	if err != nil {
		return errors.Wrap(err, "could not apply builder deposit")
	}
	if applied {
		builderDepositsProcessedTotal.Inc()
		return nil
	}

	if err := beaconState.AppendPendingDeposit(&ethpb.PendingDeposit{
		PublicKey:             request.Pubkey,
		WithdrawalCredentials: request.WithdrawalCredentials,
		Amount:                request.Amount,
		Signature:             request.Signature,
		Slot:                  beaconState.Slot(),
	}); err != nil {
		return errors.Wrap(err, "could not append deposit request")
	}
	return nil
}

// <spec fn="apply_deposit_for_builder" fork="gloas" hash="e4bc98c7">
// def apply_deposit_for_builder(
//
//	state: BeaconState,
//	pubkey: BLSPubkey,
//	withdrawal_credentials: Bytes32,
//	amount: uint64,
//	signature: BLSSignature,
//	slot: Slot,
//
// ) -> None:
//
//	builder_pubkeys = [b.pubkey for b in state.builders]
//	if pubkey not in builder_pubkeys:
//	    # Verify the deposit signature (proof of possession) which is not checked by the deposit contract
//	    if is_valid_deposit_signature(pubkey, withdrawal_credentials, amount, signature):
//	        add_builder_to_registry(state, pubkey, withdrawal_credentials, amount, slot)
//	else:
//	    # Increase balance by deposit amount
//	    builder_index = builder_pubkeys.index(pubkey)
//	    state.builders[builder_index].balance += amount
//
// </spec>
func applyBuilderDepositRequest(beaconState state.BeaconState, request *enginev1.DepositRequest) (bool, error) {
	if beaconState.Version() < version.Gloas {
		return false, nil
	}

	pubkey := bytesutil.ToBytes48(request.Pubkey)
	idx, isBuilder := beaconState.BuilderIndexByPubkey(pubkey)
	if isBuilder {
		if err := beaconState.IncreaseBuilderBalance(idx, request.Amount); err != nil {
			return false, err
		}
		return true, nil
	}

	isBuilderPrefix := helpers.IsBuilderWithdrawalCredential(request.WithdrawalCredentials)
	_, isValidator := beaconState.ValidatorIndexByPubkey(pubkey)
	if !isBuilderPrefix || isValidator {
		return false, nil
	}

	isPending, err := beaconState.IsPendingValidator(request.Pubkey)
	if err != nil {
		return false, err
	}
	if isPending {
		return false, nil
	}

	if err := applyDepositForNewBuilder(
		beaconState,
		request.Pubkey,
		request.WithdrawalCredentials,
		request.Amount,
		request.Signature,
	); err != nil {
		return false, err
	}
	return true, nil
}

func applyDepositForNewBuilder(
	beaconState state.BeaconState,
	pubkey []byte,
	withdrawalCredentials []byte,
	amount uint64,
	signature []byte,
) error {
	pubkeyBytes := bytesutil.ToBytes48(pubkey)
	valid, err := helpers.IsValidDepositSignature(&ethpb.Deposit_Data{
		PublicKey:             pubkey,
		WithdrawalCredentials: withdrawalCredentials,
		Amount:                amount,
		Signature:             signature,
	})
	if err != nil {
		return errors.Wrap(err, "could not verify deposit signature")
	}
	if !valid {
		log.WithFields(logrus.Fields{
			"pubkey": fmt.Sprintf("%x", pubkey),
		}).Warn("ignoring builder deposit: invalid signature")
		return nil
	}

	withdrawalCredBytes := bytesutil.ToBytes32(withdrawalCredentials)
	return beaconState.AddBuilderFromDeposit(pubkeyBytes, withdrawalCredBytes, amount)
}
