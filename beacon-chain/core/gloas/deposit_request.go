package gloas

import (
	"context"
	"fmt"
	gotime "time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

func processDepositRequests(ctx context.Context, beaconState state.BeaconState, requests []*enginev1.DepositRequest, prefetched []bool) error {
	if len(requests) == 0 {
		return nil
	}

	if beaconState.Version() < version.Gloas {
		return processDepositRequestsPerRequest(beaconState, requests)
	}

	// dup pubkeys break batching: a later request's classification can depend on the earlier one succeeding
	seen := make(map[[fieldparams.BLSPubkeyLength]byte]struct{}, len(requests))
	for _, req := range requests {
		if req == nil {
			return errors.New("could not apply deposit request: nil deposit request")
		}
		pk := bytesutil.ToBytes48(req.Pubkey)
		if _, dup := seen[pk]; dup {
			return processDepositRequestsPerRequest(beaconState, requests)
		}
		seen[pk] = struct{}{}
	}

	slot := beaconState.Slot()
	newBuilders := make([]*enginev1.DepositRequest, 0, len(requests))
	newBuilderIndices := make([]int, 0, len(requests))
	var (
		tBuilderLookup, tValLookup, tPendingCheck, tAppend gotime.Duration
		nAppends, nBuilderBumps                            int
	)
	for i, req := range requests {
		pubkey := bytesutil.ToBytes48(req.Pubkey)

		t1 := gotime.Now()
		idx, isExistingBuilder := beaconState.BuilderIndexByPubkey(pubkey)
		tBuilderLookup += gotime.Since(t1)
		if isExistingBuilder {
			if err := beaconState.IncreaseBuilderBalance(idx, req.Amount); err != nil {
				return errors.Wrap(err, "could not apply builder deposit")
			}
			builderDepositsProcessedTotal.Inc()
			nBuilderBumps++
			continue
		}

		if helpers.IsBuilderWithdrawalCredential(req.WithdrawalCredentials) {
			t2 := gotime.Now()
			_, isValidator := beaconState.ValidatorIndexByPubkey(pubkey)
			tValLookup += gotime.Since(t2)
			if !isValidator {
				t3 := gotime.Now()
				isPending, err := beaconState.IsPendingValidator(req.Pubkey)
				tPendingCheck += gotime.Since(t3)
				if err != nil {
					return errors.Wrap(err, "could not check pending validator")
				}
				if !isPending {
					newBuilders = append(newBuilders, req)
					newBuilderIndices = append(newBuilderIndices, i)
					continue
				}
			}
		}

		t4 := gotime.Now()
		if err := beaconState.AppendPendingDeposit(&ethpb.PendingDeposit{
			PublicKey:             req.Pubkey,
			WithdrawalCredentials: req.WithdrawalCredentials,
			Amount:                req.Amount,
			Signature:             req.Signature,
			Slot:                  slot,
		}); err != nil {
			return errors.Wrap(err, "could not append deposit request")
		}
		tAppend += gotime.Since(t4)
		nAppends++
	}

	tReg := gotime.Now()
	err := registerNewBuilders(ctx, beaconState, newBuilders, newBuilderIndices, prefetched)
	tRegDur := gotime.Since(tReg)

	log.WithFields(logrus.Fields{
		"numRequests":      len(requests),
		"nBuilderBumps":    nBuilderBumps,
		"nAppends":         nAppends,
		"nNewBuilders":     len(newBuilders),
		"builderLookup":    tBuilderLookup,
		"validatorLookup":  tValLookup,
		"pendingCheck":     tPendingCheck,
		"appendTotal":      tAppend,
		"registerBuilders": tRegDur,
	}).Info("ProcessDepositRequests timings")

	return err
}

func processDepositRequestsPerRequest(beaconState state.BeaconState, requests []*enginev1.DepositRequest) error {
	for _, receipt := range requests {
		if err := processDepositRequest(beaconState, receipt); err != nil {
			return errors.Wrap(err, "could not apply deposit request")
		}
	}
	return nil
}

func registerNewBuilders(ctx context.Context, beaconState state.BeaconState, candidates []*enginev1.DepositRequest, indices []int, prefetched []bool) error {
	if len(candidates) == 0 {
		return nil
	}

	var valid []bool
	if prefetched != nil {
		valid = make([]bool, len(indices))
		for i, idx := range indices {
			valid[i] = prefetched[idx]
		}
	} else {
		var err error
		valid, err = helpers.BatchVerifyDepositRequestSignatures(ctx, candidates)
		if err != nil {
			return errors.Wrap(err, "could not verify builder deposits")
		}
	}

	deposits := make([]state.BuilderDeposit, 0, len(candidates))
	for i, c := range candidates {
		builderDepositsProcessedTotal.Inc()
		if !valid[i] {
			log.WithFields(logrus.Fields{
				"pubkey": fmt.Sprintf("%x", c.Pubkey),
			}).Warn("ignoring builder deposit: invalid signature")
			continue
		}
		deposits = append(deposits, state.BuilderDeposit{
			Pubkey:                bytesutil.ToBytes48(c.Pubkey),
			WithdrawalCredentials: bytesutil.ToBytes32(c.WithdrawalCredentials),
			Amount:                c.Amount,
		})
	}
	if err := beaconState.AddBuildersFromDeposits(deposits); err != nil {
		return errors.Wrap(err, "could not apply builder deposits")
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
