package gloas

import (
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/pkg/errors"
)

// ProcessWithdrawals applies withdrawals to the state for Gloas.
//
// Spec v1.7.0-alpha.1 (pseudocode):
//
// def process_withdrawals(
//
//	state: BeaconState,
//	# [Modified in Gloas:EIP7732]
//	# Removed `payload`
//
// ) -> None:
//
//	# [New in Gloas:EIP7732]
//	# Return early if the parent block is empty
//	if not is_parent_block_full(state):
//	    return
//
//	# Get expected withdrawals
//	expected = get_expected_withdrawals(state)
//
//	# Apply expected withdrawals
//	apply_withdrawals(state, expected.withdrawals)
//
//	# Update withdrawals fields in the state
//	update_next_withdrawal_index(state, expected.withdrawals)
//	# [New in Gloas:EIP7732]
//	update_payload_expected_withdrawals(state, expected.withdrawals)
//	# [New in Gloas:EIP7732]
//	update_builder_pending_withdrawals(state, expected.processed_builder_withdrawals_count)
//	update_pending_partial_withdrawals(state, expected.processed_partial_withdrawals_count)
//	# [New in Gloas:EIP7732]
//	update_next_withdrawal_builder_index(state, expected.processed_builders_sweep_count)
//	update_next_withdrawal_validator_index(state, expected.withdrawals)
func ProcessWithdrawals(st state.BeaconState) error {
	full, err := st.IsParentBlockFull()
	if err != nil {
		return errors.Wrap(err, "could not get parent block full status")
	}
	if !full {
		return nil
	}

	expected, err := st.ExpectedWithdrawalsGloas()
	if err != nil {
		return errors.Wrap(err, "could not get expected withdrawals")
	}

	if err := st.DecreaseWithdrawalBalances(expected.Withdrawals); err != nil {
		return errors.Wrap(err, "could not decrease withdrawal balances")
	}

	if len(expected.Withdrawals) > 0 {
		if err := st.SetNextWithdrawalIndex(expected.Withdrawals[len(expected.Withdrawals)-1].Index + 1); err != nil {
			return errors.Wrap(err, "could not set next withdrawal index")
		}
	}

	err = st.SetPayloadExpectedWithdrawals(expected.Withdrawals)
	if err != nil {
		return errors.Wrap(err, "could not set payload expected withdrawals")
	}

	err = st.DequeueBuilderPendingWithdrawals(expected.ProcessedBuilderWithdrawalsCount)
	if err != nil {
		return errors.Wrap(err, "unable to dequeue builder pending withdrawals from state")
	}

	if err := st.DequeuePendingPartialWithdrawals(expected.ProcessedPartialWithdrawalsCount); err != nil {
		return errors.Wrap(err, "unable to dequeue partial withdrawals from state")
	}

	err = st.SetNextWithdrawalBuilderIndex(expected.NextWithdrawalBuilderIndex)
	if err != nil {
		return errors.Wrap(err, "could not set next withdrawal builder index")
	}

	var nextValidatorIndex primitives.ValidatorIndex
	if uint64(len(expected.Withdrawals)) < params.BeaconConfig().MaxWithdrawalsPerPayload {
		nextValidatorIndex, err = st.NextWithdrawalValidatorIndex()
		if err != nil {
			return errors.Wrap(err, "could not get next withdrawal validator index")
		}
		nextValidatorIndex += primitives.ValidatorIndex(params.BeaconConfig().MaxValidatorsPerWithdrawalsSweep)
		nextValidatorIndex = nextValidatorIndex % primitives.ValidatorIndex(st.NumValidators())
	} else {
		nextValidatorIndex = expected.Withdrawals[len(expected.Withdrawals)-1].ValidatorIndex + 1
		if nextValidatorIndex == primitives.ValidatorIndex(st.NumValidators()) {
			nextValidatorIndex = 0
		}
	}
	if err := st.SetNextWithdrawalValidatorIndex(nextValidatorIndex); err != nil {
		return errors.Wrap(err, "could not set next withdrawal validator index")
	}

	return nil
}
