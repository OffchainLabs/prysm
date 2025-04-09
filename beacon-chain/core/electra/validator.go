package electra

import (
	"errors"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/crypto/bls/common"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
)

// SwitchToCompoundingValidator
//
// Spec definition:
//
// def switch_to_compounding_validator(state: BeaconState, index: ValidatorIndex) -> None:
//
//	validator = state.validators[index]
//	validator.withdrawal_credentials = COMPOUNDING_WITHDRAWAL_PREFIX + validator.withdrawal_credentials[1:]
//	queue_excess_active_balance(state, index)
func SwitchToCompoundingValidator(s state.BeaconState, idx primitives.ValidatorIndex) error {
	v, err := s.ValidatorAtIndex(idx)
	if err != nil {
		return err
	}
	if len(v.WithdrawalCredentials) == 0 {
		return errors.New("validator has no withdrawal credentials")
	}

	v.WithdrawalCredentials[0] = params.BeaconConfig().CompoundingWithdrawalPrefixByte
	if err := s.UpdateValidatorAtIndex(idx, v); err != nil {
		return err
	}
	return QueueExcessActiveBalance(s, idx)
}

// QueueExcessActiveBalance queues validators with balances above the min activation balance and adds to pending deposit.
//
// Spec definition:
//
// def queue_excess_active_balance(state: BeaconState, index: ValidatorIndex) -> None:
//
//	balance = state.balances[index]
//	if balance > MIN_ACTIVATION_BALANCE:
//		excess_balance = balance - MIN_ACTIVATION_BALANCE
//		state.balances[index] = MIN_ACTIVATION_BALANCE
//		validator = state.validators[index]
//	 state.pending_deposits.append(PendingDeposit(
//	   pubkey=validator.pubkey,
//	   withdrawal_credentials=validator.withdrawal_credentials,
//	   amount=excess_balance,
//	   signature=bls.G2_POINT_AT_INFINITY,
//	   slot=GENESIS_SLOT,
//	 ))
func QueueExcessActiveBalance(s state.BeaconState, idx primitives.ValidatorIndex) error {
	bal, err := s.BalanceAtIndex(idx)
	if err != nil {
		return err
	}

	if bal > params.BeaconConfig().MinActivationBalance {
		if err := s.UpdateBalancesAtIndex(idx, params.BeaconConfig().MinActivationBalance); err != nil {
			return err
		}
		excessBalance := bal - params.BeaconConfig().MinActivationBalance
		val, err := s.ValidatorAtIndexReadOnly(idx)
		if err != nil {
			return err
		}
		pk := val.PublicKey()
		return s.AppendPendingDeposit(&ethpb.PendingDeposit{
			PublicKey:             pk[:],
			WithdrawalCredentials: val.GetWithdrawalCredentials(),
			Amount:                excessBalance,
			Signature:             common.InfiniteSignature[:],
			Slot:                  params.BeaconConfig().GenesisSlot,
		})
	}
	return nil
}

// QueueEntireBalanceAndResetValidator queues the entire balance and resets the validator. This is used in electra fork logic.
//
// Spec definition:
//
// def queue_entire_balance_and_reset_validator(state: BeaconState, index: ValidatorIndex) -> None:
//
//		balance = state.balances[index]
//		state.balances[index] = 0
//		validator = state.validators[index]
//		validator.effective_balance = 0
//		validator.activation_eligibility_epoch = FAR_FUTURE_EPOCH
//		state.pending_deposits.append(PendingDeposit(
//	  		pubkey=validator.pubkey,
//	  		withdrawal_credentials=validator.withdrawal_credentials,
//	  		amount=balance,
//	  		signature=bls.G2_POINT_AT_INFINITY,
//	  		slot=GENESIS_SLOT,
//
// ))
//
//nolint:dupword
func QueueEntireBalanceAndResetValidator(s state.BeaconState, idx primitives.ValidatorIndex) error {
	bal, err := s.BalanceAtIndex(idx)
	if err != nil {
		return err
	}

	if err := s.UpdateBalancesAtIndex(idx, 0); err != nil {
		return err
	}

	v, err := s.ValidatorAtIndex(idx)
	if err != nil {
		return err
	}

	v.EffectiveBalance = 0
	v.ActivationEligibilityEpoch = params.BeaconConfig().FarFutureEpoch
	if err := s.UpdateValidatorAtIndex(idx, v); err != nil {
		return err
	}

	return s.AppendPendingDeposit(&ethpb.PendingDeposit{
		PublicKey:             v.PublicKey,
		WithdrawalCredentials: v.WithdrawalCredentials,
		Amount:                bal,
		Signature:             common.InfiniteSignature[:],
		Slot:                  params.BeaconConfig().GenesisSlot,
	})
}
