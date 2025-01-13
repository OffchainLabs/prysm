package state_native

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/helpers"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	mathutil "github.com/prysmaticlabs/prysm/v5/math"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
)

const ETH1AddressOffset = 12

// NextWithdrawalIndex returns the index that will be assigned to the next withdrawal.
func (b *BeaconState) NextWithdrawalIndex() (uint64, error) {
	if b.version < version.Capella {
		return 0, errNotSupported("NextWithdrawalIndex", b.version)
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	return b.nextWithdrawalIndex, nil
}

// NextWithdrawalValidatorIndex returns the index of the validator which is
// next in line for a withdrawal.
func (b *BeaconState) NextWithdrawalValidatorIndex() (primitives.ValidatorIndex, error) {
	if b.version < version.Capella {
		return 0, errNotSupported("NextWithdrawalValidatorIndex", b.version)
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	return b.nextWithdrawalValidatorIndex, nil
}

// ExpectedWithdrawals returns the withdrawals that a proposer will need to pack in the next block
// applied to the current state. It is also used by validators to check that the execution payload carried
// the right number of withdrawals. Note: The number of partial withdrawals will be zero before EIP-7251.
//
// Spec definition:
//
//	def get_expected_withdrawals(state: BeaconState) -> Tuple[Sequence[Withdrawal], uint64]:
//		epoch = get_current_epoch(state)
//		withdrawal_index = state.next_withdrawal_index
//		validator_index = state.next_withdrawal_validator_index
//		withdrawals: List[Withdrawal] = []
//		processed_partial_withdrawals_count = 0
//
//		# [New in Electra:EIP7251] Consume pending partial withdrawals
//		for withdrawal in state.pending_partial_withdrawals:
//			if withdrawal.withdrawable_epoch > epoch or len(withdrawals) == MAX_PENDING_PARTIALS_PER_WITHDRAWALS_SWEEP:
//				break
//
//			validator = state.validators[withdrawal.index]
//			has_sufficient_effective_balance = validator.effective_balance >= MIN_ACTIVATION_BALANCE
//			has_excess_balance = state.balances[withdrawal.index] > MIN_ACTIVATION_BALANCE
//			if validator.exit_epoch == FAR_FUTURE_EPOCH and has_sufficient_effective_balance and has_excess_balance:
//				withdrawable_balance = min(state.balances[withdrawal.index] - MIN_ACTIVATION_BALANCE, withdrawal.amount)
//				withdrawals.append(Withdrawal(
//					index=withdrawal_index,
//					validator_index=withdrawal.index,
//					address=ExecutionAddress(validator.withdrawal_credentials[12:]),
//					amount=withdrawable_balance,
//				))
//				withdrawal_index += WithdrawalIndex(1)
//
//		processed_partial_withdrawals_count += 1
//
//		# Sweep for remaining.
//		bound = min(len(state.validators), MAX_VALIDATORS_PER_WITHDRAWALS_SWEEP)
//		for _ in range(bound):
//			validator = state.validators[validator_index]
//			# [Modified in Electra:EIP7251]
//			partially_withdrawn_balance = sum(withdrawal.amount for withdrawal in withdrawals if withdrawal.validator_index == validator_index)
//			balance = state.balances[validator_index] - partially_withdrawn_balance
//			if is_fully_withdrawable_validator(validator, balance, epoch):
//				withdrawals.append(Withdrawal(
//					index=withdrawal_index,
//					validator_index=validator_index,
//					address=ExecutionAddress(validator.withdrawal_credentials[12:]),
//					amount=balance,
//				))
//				withdrawal_index += WithdrawalIndex(1)
//			elif is_partially_withdrawable_validator(validator, balance):
//				withdrawals.append(Withdrawal(
//					index=withdrawal_index,
//					validator_index=validator_index,
//					address=ExecutionAddress(validator.withdrawal_credentials[12:]),
//					amount=balance - get_validator_max_effective_balance(validator),  # [Modified in Electra:EIP7251]
//				))
//				withdrawal_index += WithdrawalIndex(1)
//			if len(withdrawals) == MAX_WITHDRAWALS_PER_PAYLOAD:
//				break
//			validator_index = ValidatorIndex((validator_index + 1) % len(state.validators))
//		return withdrawals, processed_partial_withdrawals_count
func (b *BeaconState) ExpectedWithdrawals() ([]*enginev1.Withdrawal, uint64, error) {
	if b.version < version.Capella {
		return nil, 0, errNotSupported("ExpectedWithdrawals", b.version)
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	withdrawals := make([]*enginev1.Withdrawal, 0, params.BeaconConfig().MaxWithdrawalsPerPayload)
	validatorIndex := b.nextWithdrawalValidatorIndex
	withdrawalIndex := b.nextWithdrawalIndex
	epoch := slots.ToEpoch(b.slot)

	// Electra partial withdrawals functionality.
	var processedPartialWithdrawalsCount uint64
	if b.version >= version.Electra {
		for _, w := range b.pendingPartialWithdrawals {
			if w.WithdrawableEpoch > epoch || len(withdrawals) >= int(params.BeaconConfig().MaxPendingPartialsPerWithdrawalsSweep) {
				break
			}

			v, err := b.validatorAtIndexReadOnly(w.Index)
			if err != nil {
				return nil, 0, fmt.Errorf("failed to determine withdrawals at index %d: %w", w.Index, err)
			}
			vBal, err := b.balanceAtIndex(w.Index)
			if err != nil {
				return nil, 0, fmt.Errorf("could not retrieve balance at index %d: %w", w.Index, err)
			}
			hasSufficientEffectiveBalance := v.EffectiveBalance() >= params.BeaconConfig().MinActivationBalance
			hasExcessBalance := vBal > params.BeaconConfig().MinActivationBalance
			if v.ExitEpoch() == params.BeaconConfig().FarFutureEpoch && hasSufficientEffectiveBalance && hasExcessBalance {
				amount := min(vBal-params.BeaconConfig().MinActivationBalance, w.Amount)
				withdrawals = append(withdrawals, &enginev1.Withdrawal{
					Index:          withdrawalIndex,
					ValidatorIndex: w.Index,
					Address:        v.GetWithdrawalCredentials()[12:],
					Amount:         amount,
				})
				withdrawalIndex++
			}
			processedPartialWithdrawalsCount++
		}
	}

	validatorsLen := b.validatorsLen()
	bound := mathutil.Min(uint64(validatorsLen), params.BeaconConfig().MaxValidatorsPerWithdrawalsSweep)
	for i := uint64(0); i < bound; i++ {
		val, err := b.validatorAtIndexReadOnly(validatorIndex)
		if err != nil {
			return nil, 0, errors.Wrapf(err, "could not retrieve validator at index %d", validatorIndex)
		}
		balance, err := b.balanceAtIndex(validatorIndex)
		if err != nil {
			return nil, 0, errors.Wrapf(err, "could not retrieve balance at index %d", validatorIndex)
		}
		if b.version >= version.Electra {
			var partiallyWithdrawnBalance uint64
			for _, w := range withdrawals {
				if w.ValidatorIndex == validatorIndex {
					partiallyWithdrawnBalance += w.Amount
				}
			}
			balance = balance - partiallyWithdrawnBalance
		}
		if helpers.IsFullyWithdrawableValidator(val, balance, epoch, b.version) {
			withdrawals = append(withdrawals, &enginev1.Withdrawal{
				Index:          withdrawalIndex,
				ValidatorIndex: validatorIndex,
				Address:        bytesutil.SafeCopyBytes(val.GetWithdrawalCredentials()[ETH1AddressOffset:]),
				Amount:         balance,
			})
			withdrawalIndex++
		} else if helpers.IsPartiallyWithdrawableValidator(val, balance, epoch, b.version) {
			withdrawals = append(withdrawals, &enginev1.Withdrawal{
				Index:          withdrawalIndex,
				ValidatorIndex: validatorIndex,
				Address:        bytesutil.SafeCopyBytes(val.GetWithdrawalCredentials()[ETH1AddressOffset:]),
				Amount:         balance - helpers.ValidatorMaxEffectiveBalance(val),
			})
			withdrawalIndex++
		}
		if uint64(len(withdrawals)) == params.BeaconConfig().MaxWithdrawalsPerPayload {
			break
		}
		validatorIndex += 1
		if uint64(validatorIndex) == uint64(validatorsLen) {
			validatorIndex = 0
		}
	}

	return withdrawals, processedPartialWithdrawalsCount, nil
}

func (b *BeaconState) PendingPartialWithdrawals() ([]*ethpb.PendingPartialWithdrawal, error) {
	if b.version < version.Electra {
		return nil, errNotSupported("PendingPartialWithdrawals", b.version)
	}
	b.lock.RLock()
	defer b.lock.RUnlock()
	return b.pendingPartialWithdrawalsVal(), nil
}

func (b *BeaconState) pendingPartialWithdrawalsVal() []*ethpb.PendingPartialWithdrawal {
	return ethpb.CopySlice(b.pendingPartialWithdrawals)
}

func (b *BeaconState) NumPendingPartialWithdrawals() (uint64, error) {
	if b.version < version.Electra {
		return 0, errNotSupported("NumPendingPartialWithdrawals", b.version)
	}
	return uint64(len(b.pendingPartialWithdrawals)), nil
}
