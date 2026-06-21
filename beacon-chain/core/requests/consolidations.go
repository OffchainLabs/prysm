package requests

import (
	"bytes"
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/electra"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/pkg/errors"
)

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
//	    # Verify that target has compounding withdrawal credentials
//	    if not has_compounding_withdrawal_credential(target_validator):
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
//	    # Verify the source has been active long enough
//	    if current_epoch < source_validator.activation_epoch + SHARD_COMMITTEE_PERIOD:
//	        return
//
//	    # Verify the source has no pending withdrawals in the queue
//	    if get_pending_balance_to_withdraw(state, source_index) > 0:
//	        return
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
func ProcessConsolidationRequests(ctx context.Context, st state.BeaconState, reqs []*enginev1.ConsolidationRequest) error {
	ctx, span := trace.StartSpan(ctx, "requests.ProcessConsolidationRequests")
	defer span.End()

	if len(reqs) == 0 || st == nil {
		return nil
	}
	curEpoch := slots.ToEpoch(st.Slot())
	ffe := params.BeaconConfig().FarFutureEpoch
	minValWithdrawDelay := params.BeaconConfig().MinValidatorWithdrawabilityDelay
	pcLimit := params.BeaconConfig().PendingConsolidationsLimit

	for _, cr := range reqs {
		if cr == nil {
			return errors.New("nil consolidation request")
		}
		if ctx.Err() != nil {
			return fmt.Errorf("cannot process consolidation requests: %w", ctx.Err())
		}

		if electra.IsValidSwitchToCompoundingRequest(st, cr) {
			srcIdx, ok := st.ValidatorIndexByPubkey(bytesutil.ToBytes48(cr.SourcePubkey))
			if !ok {
				log.Error("Failed to find source validator index")
				continue
			}
			if err := electra.SwitchToCompoundingValidator(st, srcIdx); err != nil {
				log.WithError(err).Error("Failed to switch to compounding validator")
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
			continue
		}

		activeBal, err := helpers.TotalActiveBalance(ctx, st)
		if err != nil {
			return err
		}
		churnLimit := helpers.ConsolidationChurnLimitForVersion(st.Version(), primitives.Gwei(activeBal))
		if churnLimit <= primitives.Gwei(params.BeaconConfig().MinActivationBalance) {
			continue
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

		roSrcV, err := state_native.NewValidator(srcV)
		if err != nil {
			return err
		}

		tgtV, err := st.ValidatorAtIndexReadOnly(tgtIdx)
		if err != nil {
			return fmt.Errorf("failed to fetch target validator: %w", err) // This should never happen.
		}

		// Verify source withdrawal credentials.
		if !roSrcV.HasExecutionWithdrawalCredentials() {
			continue
		}
		// Confirm source_validator.withdrawal_credentials[12:] == consolidation_request.source_address.
		if len(srcV.WithdrawalCredentials) != 32 || len(cr.SourceAddress) != 20 || !bytes.HasSuffix(srcV.WithdrawalCredentials, cr.SourceAddress) {
			continue
		}

		// Target validator must have their withdrawal credentials set appropriately.
		if !tgtV.HasCompoundingWithdrawalCredentials() {
			continue
		}

		// Both validators must be active.
		if !helpers.IsActiveValidator(srcV, curEpoch) || !helpers.IsActiveValidatorUsingTrie(tgtV, curEpoch) {
			continue
		}
		// Neither validator is exiting.
		if srcV.ExitEpoch != ffe || tgtV.ExitEpoch() != ffe {
			continue
		}

		e, overflow := math.SafeAdd(uint64(srcV.ActivationEpoch), uint64(params.BeaconConfig().ShardCommitteePeriod))
		if overflow {
			log.Error("Overflow when adding activation epoch and shard committee period")
			continue
		}
		if uint64(curEpoch) < e {
			continue
		}

		hasBal, err := st.HasPendingBalanceToWithdraw(srcIdx)
		if err != nil {
			log.WithError(err).Error("Failed to fetch pending balance to withdraw")
			continue
		}
		if hasBal {
			continue
		}

		exitEpoch, err := electra.ComputeConsolidationEpochAndUpdateChurn(ctx, st, primitives.Gwei(srcV.EffectiveBalance))
		if err != nil {
			log.WithError(err).Error("Failed to compute consolidation epoch")
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
	}

	return nil
}
