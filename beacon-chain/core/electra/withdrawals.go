package electra

import (
	"bytes"
	"context"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/helpers"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/validators"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
	log "github.com/sirupsen/logrus"
)

// ProcessWithdrawalRequests processes the validator withdrawals from the provided execution payload
// into the beacon state triggered by the execution layer.
//
// Spec pseudocode definition:
//
// def process_withdrawal_request(
//
//	state: BeaconState,
//	withdrawal_request: WithdrawalRequest
//
// ) -> None:
//
//	amount = withdrawal_request.amount
//	is_full_exit_request = amount == FULL_EXIT_REQUEST_AMOUNT
//
//	# If partial withdrawal queue is full, only full exits are processed
//	if len(state.pending_partial_withdrawals) == PENDING_PARTIAL_WITHDRAWALS_LIMIT and not is_full_exit_request:
//	    return
//
//	validator_pubkeys = [v.pubkey for v in state.validators]
//	# Verify pubkey exists
//	request_pubkey = withdrawal_request.validator_pubkey
//	if request_pubkey not in validator_pubkeys:
//	    return
//	index = ValidatorIndex(validator_pubkeys.index(request_pubkey))
//	validator = state.validators[index]
//
//	# Verify withdrawal credentials
//	has_correct_credential = has_execution_withdrawal_credential(validator)
//	is_correct_source_address = (
//	    validator.withdrawal_credentials[12:] == withdrawal_request.source_address
//	)
//	if not (has_correct_credential and is_correct_source_address):
//	    return
//	# Verify the validator is active
//	if not is_active_validator(validator, get_current_epoch(state)):
//	    return
//	# Verify exit has not been initiated
//	if validator.exit_epoch != FAR_FUTURE_EPOCH:
//	    return
//	# Verify the validator has been active long enough
//	if get_current_epoch(state) < validator.activation_epoch + SHARD_COMMITTEE_PERIOD:
//	    return
//
//	pending_balance_to_withdraw = get_pending_balance_to_withdraw(state, index)
//
//	if is_full_exit_request:
//	    # Only exit validator if it has no pending withdrawals in the queue
//	    if pending_balance_to_withdraw == 0:
//	        initiate_validator_exit(state, index)
//	    return
//
//	has_sufficient_effective_balance = validator.effective_balance >= MIN_ACTIVATION_BALANCE
//	has_excess_balance = state.balances[index] > MIN_ACTIVATION_BALANCE + pending_balance_to_withdraw
//
//	# Only allow partial withdrawals with compounding withdrawal credentials
//	if has_compounding_withdrawal_credential(validator) and has_sufficient_effective_balance and has_excess_balance:
//	    to_withdraw = min(
//	        state.balances[index] - MIN_ACTIVATION_BALANCE - pending_balance_to_withdraw,
//	        amount
//	    )
//	    exit_queue_epoch = compute_exit_epoch_and_update_churn(state, to_withdraw)
//	    withdrawable_epoch = Epoch(exit_queue_epoch + MIN_VALIDATOR_WITHDRAWABILITY_DELAY)
//	    state.pending_partial_withdrawals.append(PendingPartialWithdrawal(
//	        index=index,
//	        amount=to_withdraw,
//	        withdrawable_epoch=withdrawable_epoch,
//	    ))
func ProcessWithdrawalRequests(ctx context.Context, st state.BeaconState, wrs []*enginev1.WithdrawalRequest) (state.BeaconState, error) {
	ctx, span := trace.StartSpan(ctx, "electra.ProcessWithdrawalRequests")
	defer span.End()
	currentEpoch := slots.ToEpoch(st.Slot())
	for _, wr := range wrs {
		if wr == nil {
			return nil, errors.New("nil execution layer withdrawal request")
		}
		amount := wr.Amount
		isFullExitRequest := amount == params.BeaconConfig().FullExitRequestAmount
		// If partial withdrawal queue is full, only full exits are processed
		if n, err := st.NumPendingPartialWithdrawals(); err != nil {
			return nil, err
		} else if n == params.BeaconConfig().PendingPartialWithdrawalsLimit && !isFullExitRequest {
			// if the PendingPartialWithdrawalsLimit is met, the user would have paid for a partial withdrawal that's not included
			log.Debugln("Skipping execution layer withdrawal request, PendingPartialWithdrawalsLimit reached")
			continue
		}

		vIdx, exists := st.ValidatorIndexByPubkey(bytesutil.ToBytes48(wr.ValidatorPubkey))
		if !exists {
			log.Debugf("Skipping execution layer withdrawal request, validator index for %s not found\n", hexutil.Encode(wr.ValidatorPubkey))
			continue
		}
		validator, err := st.ValidatorAtIndexReadOnly(vIdx)
		if err != nil {
			return nil, err
		}
		// Verify withdrawal credentials
		hasCorrectCredential := helpers.HasExecutionWithdrawalCredentials(validator)
		wc := validator.GetWithdrawalCredentials()
		isCorrectSourceAddress := bytes.Equal(wc[12:], wr.SourceAddress)
		if !hasCorrectCredential || !isCorrectSourceAddress {
			log.Debugln("Skipping execution layer withdrawal request, wrong withdrawal credentials")
			continue
		}

		// Verify the validator is active.
		if !helpers.IsActiveValidatorUsingTrie(validator, currentEpoch) {
			log.Debugln("Skipping execution layer withdrawal request, validator not active")
			continue
		}
		// Verify the validator has not yet submitted an exit.
		if validator.ExitEpoch() != params.BeaconConfig().FarFutureEpoch {
			log.Debugln("Skipping execution layer withdrawal request, validator has submitted an exit already")
			continue
		}
		// Verify the validator has been active long enough.
		if currentEpoch < validator.ActivationEpoch().AddEpoch(params.BeaconConfig().ShardCommitteePeriod) {
			log.Debugln("Skipping execution layer withdrawal request, validator has not been active long enough")
			continue
		}

		pendingBalanceToWithdraw, err := st.PendingBalanceToWithdraw(vIdx)
		if err != nil {
			return nil, err
		}
		if isFullExitRequest {
			// Only exit validator if it has no pending withdrawals in the queue
			if pendingBalanceToWithdraw == 0 {
				maxExitEpoch, churn := validators.MaxExitEpochAndChurn(st)
				var err error
				st, _, err = validators.InitiateValidatorExit(ctx, st, vIdx, maxExitEpoch, churn)
				if err != nil {
					return nil, err
				}
			}
			continue
		}

		hasSufficientEffectiveBalance := validator.EffectiveBalance() >= params.BeaconConfig().MinActivationBalance
		vBal, err := st.BalanceAtIndex(vIdx)
		if err != nil {
			return nil, err
		}
		hasExcessBalance := vBal > params.BeaconConfig().MinActivationBalance+pendingBalanceToWithdraw

		// Only allow partial withdrawals with compounding withdrawal credentials
		if helpers.HasCompoundingWithdrawalCredential(validator) && hasSufficientEffectiveBalance && hasExcessBalance {
			// Spec definition:
			//  to_withdraw = min(
			//	  state.balances[index] - MIN_ACTIVATION_BALANCE - pending_balance_to_withdraw,
			//	  amount
			//  )

			// note: you can safely subtract these values because haxExcessBalance is checked
			toWithdraw := min(vBal-params.BeaconConfig().MinActivationBalance-pendingBalanceToWithdraw, amount)
			exitQueueEpoch, err := st.ExitEpochAndUpdateChurn(primitives.Gwei(toWithdraw))
			if err != nil {
				return nil, err
			}
			// safe add the uint64 to avoid overflow
			withdrawableEpoch, err := exitQueueEpoch.SafeAddEpoch(params.BeaconConfig().MinValidatorWithdrawabilityDelay)
			if err != nil {
				return nil, errors.Wrap(err, "failed to add withdrawability delay to exit queue epoch")
			}
			if err := st.AppendPendingPartialWithdrawal(&ethpb.PendingPartialWithdrawal{
				Index:             vIdx,
				Amount:            toWithdraw,
				WithdrawableEpoch: withdrawableEpoch,
			}); err != nil {
				return nil, err
			}
		}
	}
	return st, nil
}
