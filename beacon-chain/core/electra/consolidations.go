package electra

import (
	"bytes"
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/helpers"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
	log "github.com/sirupsen/logrus"
)

// ProcessPendingConsolidations implements the spec definition below. This method makes mutating
// calls to the beacon state.
//
// Spec definition:
//
// def process_pending_consolidations(state: BeaconState) -> None:
//
//	next_epoch = Epoch(get_current_epoch(state) + 1)
//	next_pending_consolidation = 0
//	for pending_consolidation in state.pending_consolidations:
//	    source_validator = state.validators[pending_consolidation.source_index]
//	    if source_validator.slashed:
//	        next_pending_consolidation += 1
//	        continue
//	    if source_validator.withdrawable_epoch > next_epoch:
//	        break
//
//	    # Calculate the consolidated balance
//	    max_effective_balance = get_max_effective_balance(source_validator)
//	    source_effective_balance = min(state.balances[pending_consolidation.source_index], max_effective_balance)
//
//	    # Move active balance to target. Excess balance is withdrawable.
//	    decrease_balance(state, pending_consolidation.source_index, source_effective_balance)
//	    increase_balance(state, pending_consolidation.target_index, source_effective_balance)
//	    next_pending_consolidation += 1
//
//	state.pending_consolidations = state.pending_consolidations[next_pending_consolidation:]
func ProcessPendingConsolidations(ctx context.Context, st state.BeaconState) error {
	_, span := trace.StartSpan(ctx, "electra.ProcessPendingConsolidations")
	defer span.End()

	if st == nil || st.IsNil() {
		return errors.New("nil state")
	}

	nextEpoch := slots.ToEpoch(st.Slot()) + 1

	pendingConsolidations, err := st.PendingConsolidations()
	if err != nil {
		return err
	}
	var nextPendingConsolidation uint64
	for _, pc := range pendingConsolidations {
		sourceValidator, err := st.ValidatorAtIndexReadOnly(pc.SourceIndex)
		if err != nil {
			return err
		}
		if sourceValidator.Slashed() {
			nextPendingConsolidation++
			continue
		}
		if sourceValidator.WithdrawableEpoch() > nextEpoch {
			break
		}

		validatorBalance, err := st.BalanceAtIndex(pc.SourceIndex)
		if err != nil {
			return err
		}
		b := min(validatorBalance, helpers.ValidatorMaxEffectiveBalance(sourceValidator))

		if err := helpers.DecreaseBalance(st, pc.SourceIndex, b); err != nil {
			return err
		}
		if err := helpers.IncreaseBalance(st, pc.TargetIndex, b); err != nil {
			return err
		}
		nextPendingConsolidation++
	}

	if nextPendingConsolidation > 0 {
		return st.SetPendingConsolidations(pendingConsolidations[nextPendingConsolidation:])
	}

	return nil
}

// ProcessConsolidationRequests implements the spec definition below. This method makes mutating
// calls to the beacon state.
//
//	def process_consolidation_request(
//	    state: BeaconState,
//	    consolidation_request: ConsolidationRequest
//	) -> None:
//	    if is_valid_switch_to_compounding_request(state, consolidation_request):
//	        validator_pubkeys = [v.pubkey for v in state.validators]
//	        request_source_pubkey = consolidation_request.source_pubkey
//	        source_index = ValidatorIndex(validator_pubkeys.index(request_source_pubkey))
//	        switch_to_compounding_validator(state, source_index)
//	        return
//
//	    # Verify that source != target, so a consolidation cannot be used as an exit.
//	    if consolidation_request.source_pubkey == consolidation_request.target_pubkey:
//	        return
//	    # If the pending consolidations queue is full, consolidation requests are ignored
//	    if len(state.pending_consolidations) == PENDING_CONSOLIDATIONS_LIMIT:
//	        return
//	    # If there is too little available consolidation churn limit, consolidation requests are ignored
//	    if get_consolidation_churn_limit(state) <= MIN_ACTIVATION_BALANCE:
//	        return
//
//	    validator_pubkeys = [v.pubkey for v in state.validators]
//	    # Verify pubkeys exists
//	    request_source_pubkey = consolidation_request.source_pubkey
//	    request_target_pubkey = consolidation_request.target_pubkey
//	    if request_source_pubkey not in validator_pubkeys:
//	        return
//	    if request_target_pubkey not in validator_pubkeys:
//	        return
//	    source_index = ValidatorIndex(validator_pubkeys.index(request_source_pubkey))
//	    target_index = ValidatorIndex(validator_pubkeys.index(request_target_pubkey))
//	    source_validator = state.validators[source_index]
//	    target_validator = state.validators[target_index]
//
//	    # Verify source withdrawal credentials
//	    has_correct_credential = has_execution_withdrawal_credential(source_validator)
//	    is_correct_source_address = (
//	        source_validator.withdrawal_credentials[12:] == consolidation_request.source_address
//	    )
//	    if not (has_correct_credential and is_correct_source_address):
//	        return
//
//	    # Verify that target has execution withdrawal credentials
//	    if not has_execution_withdrawal_credential(target_validator):
//	        return
//
//	    # Verify the source and the target are active
//	    current_epoch = get_current_epoch(state)
//	    if not is_active_validator(source_validator, current_epoch):
//	        return
//	    if not is_active_validator(target_validator, current_epoch):
//	        return
//	    # Verify exits for source and target have not been initiated
//	    if source_validator.exit_epoch != FAR_FUTURE_EPOCH:
//	        return
//	    if target_validator.exit_epoch != FAR_FUTURE_EPOCH:
//	        return
//
//	    # Initiate source validator exit and append pending consolidation
//	    source_validator.exit_epoch = compute_consolidation_epoch_and_update_churn(
//	        state, source_validator.effective_balance
//	    )
//	    source_validator.withdrawable_epoch = Epoch(
//	        source_validator.exit_epoch + MIN_VALIDATOR_WITHDRAWABILITY_DELAY
//	    )
//	    state.pending_consolidations.append(PendingConsolidation(
//	        source_index=source_index,
//	        target_index=target_index
//	    ))
//
//	    # Churn any target excess active balance of target and raise its max
//	    if has_eth1_withdrawal_credential(target_validator):
//	        switch_to_compounding_validator(state, target_index)
func ProcessConsolidationRequests(ctx context.Context, st state.BeaconState, reqs []*enginev1.ConsolidationRequest) error {
	if len(reqs) == 0 || st == nil {
		return nil
	}
	curEpoch := slots.ToEpoch(st.Slot())
	ffe := params.BeaconConfig().FarFutureEpoch
	minValWithdrawDelay := params.BeaconConfig().MinValidatorWithdrawabilityDelay
	pcLimit := params.BeaconConfig().PendingConsolidationsLimit

	for _, cr := range reqs {
		if ctx.Err() != nil {
			return fmt.Errorf("cannot process consolidation requests: %w", ctx.Err())
		}
		if IsValidSwitchToCompoundingRequest(st, cr) {
			srcIdx, ok := st.ValidatorIndexByPubkey(bytesutil.ToBytes48(cr.SourcePubkey))
			if !ok {
				log.Error("failed to find source validator index")
				continue
			}
			if err := SwitchToCompoundingValidator(st, srcIdx); err != nil {
				log.WithError(err).Error("failed to switch to compounding validator")
			}
			continue
		}
		sourcePubkey := bytesutil.ToBytes48(cr.SourcePubkey)
		targetPubkey := bytesutil.ToBytes48(cr.TargetPubkey)
		if sourcePubkey == targetPubkey {
			continue
		}

		if npc, err := st.NumPendingConsolidations(); err != nil {
			return fmt.Errorf("failed to fetch number of pending consolidations: %w", err) // This should never happen.
		} else if npc >= pcLimit {
			return nil
		}

		activeBal, err := helpers.TotalActiveBalance(st)
		if err != nil {
			return err
		}
		churnLimit := helpers.ConsolidationChurnLimit(primitives.Gwei(activeBal))
		if churnLimit <= primitives.Gwei(params.BeaconConfig().MinActivationBalance) {
			return nil
		}

		srcIdx, ok := st.ValidatorIndexByPubkey(sourcePubkey)
		if !ok {
			continue
		}
		tgtIdx, ok := st.ValidatorIndexByPubkey(targetPubkey)
		if !ok {
			continue
		}

		srcV, err := st.ValidatorAtIndex(srcIdx)
		if err != nil {
			return fmt.Errorf("failed to fetch source validator: %w", err) // This should never happen.
		}

		tgtV, err := st.ValidatorAtIndexReadOnly(tgtIdx)
		if err != nil {
			return fmt.Errorf("failed to fetch target validator: %w", err) // This should never happen.
		}

		// Verify source withdrawal credentials
		if !helpers.HasExecutionWithdrawalCredentials(srcV) {
			continue
		}
		// Confirm source_validator.withdrawal_credentials[12:] == consolidation_request.source_address
		if len(srcV.WithdrawalCredentials) != 32 || len(cr.SourceAddress) != 20 || !bytes.HasSuffix(srcV.WithdrawalCredentials, cr.SourceAddress) {
			continue
		}

		// Target validator must have their withdrawal credentials set appropriately.
		if !helpers.HasExecutionWithdrawalCredentials(tgtV) {
			continue
		}

		// Both validators must be active.
		if !helpers.IsActiveValidator(srcV, curEpoch) || !helpers.IsActiveValidatorUsingTrie(tgtV, curEpoch) {
			continue
		}
		// Neither validator are exiting.
		if srcV.ExitEpoch != ffe || tgtV.ExitEpoch() != ffe {
			continue
		}

		// Initiate the exit of the source validator.
		exitEpoch, err := ComputeConsolidationEpochAndUpdateChurn(ctx, st, primitives.Gwei(srcV.EffectiveBalance))
		if err != nil {
			log.WithError(err).Error("failed to compute consolidation epoch")
			continue
		}
		srcV.ExitEpoch = exitEpoch
		srcV.WithdrawableEpoch = exitEpoch + minValWithdrawDelay
		if err := st.UpdateValidatorAtIndex(srcIdx, srcV); err != nil {
			return fmt.Errorf("failed to update validator: %w", err) // This should never happen.
		}

		if err := st.AppendPendingConsolidation(&eth.PendingConsolidation{SourceIndex: srcIdx, TargetIndex: tgtIdx}); err != nil {
			return fmt.Errorf("failed to append pending consolidation: %w", err) // This should never happen.
		}

		if helpers.HasETH1WithdrawalCredential(tgtV) {
			if err := SwitchToCompoundingValidator(st, tgtIdx); err != nil {
				log.WithError(err).Error("failed to switch to compounding validator")
				continue
			}
		}
	}

	return nil
}

// IsValidSwitchToCompoundingRequest returns true if the given consolidation request is valid for switching to compounding.
//
// Spec code:
//
// def is_valid_switch_to_compounding_request(
//
//	state: BeaconState,
//	consolidation_request: ConsolidationRequest
//
// ) -> bool:
//
//	# Switch to compounding requires source and target be equal
//	if consolidation_request.source_pubkey != consolidation_request.target_pubkey:
//	    return False
//
//	# Verify pubkey exists
//	source_pubkey = consolidation_request.source_pubkey
//	validator_pubkeys = [v.pubkey for v in state.validators]
//	if source_pubkey not in validator_pubkeys:
//	    return False
//
//	source_validator = state.validators[ValidatorIndex(validator_pubkeys.index(source_pubkey))]
//
//	# Verify request has been authorized
//	if source_validator.withdrawal_credentials[12:] != consolidation_request.source_address:
//	    return False
//
//	# Verify source withdrawal credentials
//	if not has_eth1_withdrawal_credential(source_validator):
//	    return False
//
//	# Verify the source is active
//	current_epoch = get_current_epoch(state)
//	if not is_active_validator(source_validator, current_epoch):
//	    return False
//
//	# Verify exit for source has not been initiated
//	if source_validator.exit_epoch != FAR_FUTURE_EPOCH:
//	    return False
//
//	return True
func IsValidSwitchToCompoundingRequest(st state.BeaconState, req *enginev1.ConsolidationRequest) bool {
	if req.SourcePubkey == nil || req.TargetPubkey == nil {
		return false
	}

	if !bytes.Equal(req.SourcePubkey, req.TargetPubkey) {
		return false
	}

	srcIdx, ok := st.ValidatorIndexByPubkey(bytesutil.ToBytes48(req.SourcePubkey))
	if !ok {
		return false
	}
	// As per the consensus specification, this error is not considered an assertion.
	// Therefore, if the source_pubkey is not found in validator_pubkeys, we simply return false.
	srcV, err := st.ValidatorAtIndexReadOnly(srcIdx)
	if err != nil {
		return false
	}
	sourceAddress := req.SourceAddress
	withdrawalCreds := srcV.GetWithdrawalCredentials()
	if len(withdrawalCreds) != 32 || len(sourceAddress) != 20 || !bytes.HasSuffix(withdrawalCreds, sourceAddress) {
		return false
	}

	if !helpers.HasETH1WithdrawalCredential(srcV) {
		return false
	}

	curEpoch := slots.ToEpoch(st.Slot())
	if !helpers.IsActiveValidatorUsingTrie(srcV, curEpoch) {
		return false
	}

	if srcV.ExitEpoch() != params.BeaconConfig().FarFutureEpoch {
		return false
	}
	return true
}
