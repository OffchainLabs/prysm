package electra

import (
	"bytes"
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
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
//	    source_effective_balance = min(state.balances[pending_consolidation.source_index], source_validator.effective_balance)
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
		if sourceValidator.Slashed {
			nextPendingConsolidation++
			continue
		}
		if sourceValidator.WithdrawableEpoch > nextEpoch {
			break
		}

		validatorBalance, err := st.BalanceAtIndex(pc.SourceIndex)
		if err != nil {
			return err
		}
		b := min(validatorBalance, sourceValidator.EffectiveBalance)

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
	withdrawalCreds := srcV.WithdrawalCredentials[:]
	if len(withdrawalCreds) != 32 || len(sourceAddress) != 20 || !bytes.HasSuffix(withdrawalCreds, sourceAddress) {
		return false
	}

	if !srcV.HasETH1WithdrawalCredentials() {
		return false
	}

	curEpoch := slots.ToEpoch(st.Slot())
	if !helpers.IsActiveValidatorUsingTrie(&srcV, curEpoch) {
		return false
	}

	if srcV.ExitEpoch != params.BeaconConfig().FarFutureEpoch {
		return false
	}
	return true
}
