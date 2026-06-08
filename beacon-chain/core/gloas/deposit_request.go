package gloas

import (
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// depositSigValidity builds a per-request "signature valid" slice from a list of
// invalid indices. A nil/empty invalidIdx means every request is valid.
func depositSigValidity(n int, invalidIdx []int) ([]bool, error) {
	valid := make([]bool, n)
	for i := range valid {
		valid[i] = true
	}
	for _, idx := range invalidIdx {
		if idx < 0 || idx >= n {
			return nil, errors.Errorf("deposit signature index %d out of range [0,%d)", idx, n)
		}
		valid[idx] = false
	}
	return valid, nil
}

// prefetchedDepositSigs returns the cached per-request validity slice, or nil
// on cache miss.
func prefetchedDepositSigs(rqs *enginev1.ExecutionRequests) []bool {
	if rqs == nil || len(rqs.Deposits) == 0 {
		return nil
	}
	root, err := rqs.HashTreeRoot()
	if err != nil {
		return nil
	}
	invalidIdx, ok := cache.DepositSig.Get(root)
	if !ok {
		return nil
	}
	valid, err := depositSigValidity(len(rqs.Deposits), invalidIdx)
	if err != nil {
		return nil
	}
	return valid
}

func ProcessDepositRequests(ctx context.Context, beaconState state.BeaconState, requests []*enginev1.DepositRequest, prefetched []bool) error {
	if len(requests) == 0 {
		return nil
	}

	// On a prefetch cache miss the validity slice is nil; verify all signatures
	// now so the per-request handling only ever sees a plain bool.
	if prefetched == nil {
		invalidIdx, err := helpers.BatchVerifyDepositRequestSignatures(ctx, requests)
		if err != nil {
			return errors.Wrap(err, "could not verify deposit signatures")
		}
		if prefetched, err = depositSigValidity(len(requests), invalidIdx); err != nil {
			return err
		}
	}

	for i, request := range requests {
		if err := processDepositRequest(beaconState, request, prefetched[i]); err != nil {
			return errors.Wrap(err, "could not apply deposit request")
		}
	}
	return nil
}

// processDepositRequest processes the specific deposit request
//
//	<spec fn="process_deposit_request" fork="gloas" hash="a6fff32f">
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
//	        and not is_pending_validator(state.pending_deposits, deposit_request.pubkey)
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
func processDepositRequest(beaconState state.BeaconState, request *enginev1.DepositRequest, sigValid bool) error {
	if request == nil {
		return errors.New("nil deposit request")
	}

	applied, err := applyBuilderDepositRequest(beaconState, request, sigValid)
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
func applyBuilderDepositRequest(beaconState state.BeaconState, request *enginev1.DepositRequest, sigValid bool) (bool, error) {
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
		sigValid,
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
	sigValid bool,
) error {
	if !sigValid {
		log.WithFields(logrus.Fields{
			"pubkey": fmt.Sprintf("%x", pubkey),
		}).Warn("ignoring builder deposit: invalid signature")
		return nil
	}

	pubkeyBytes := bytesutil.ToBytes48(pubkey)
	withdrawalCredBytes := bytesutil.ToBytes32(withdrawalCredentials)
	return beaconState.AddBuilderFromDeposit(pubkeyBytes, withdrawalCredBytes, amount)
}
