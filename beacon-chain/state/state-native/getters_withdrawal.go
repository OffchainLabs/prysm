package state_native

import (
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	mathutil "github.com/OffchainLabs/prysm/v7/math"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
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
//			total_withdrawn = sum(w.amount for w in withdrawals if w.validator_index == withdrawal.validator_index)
//			balance = state.balances[withdrawal.validator_index] - total_withdrawn
//			has_excess_balance = balance > MIN_ACTIVATION_BALANCE
//			if validator.exit_epoch == FAR_FUTURE_EPOCH and has_sufficient_effective_balance and has_excess_balance:
//				withdrawable_balance = min(balance - MIN_ACTIVATION_BALANCE, withdrawal.amount)
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
//					amount=balance - get_max_effective_balance(validator),  # [Modified in Electra:EIP7251]
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
	withdrawalIndex := b.nextWithdrawalIndex

	withdrawalIndex, processedPartialWithdrawalsCount, err := b.appendPendingPartialWithdrawals(withdrawalIndex, &withdrawals)
	if err != nil {
		return nil, 0, err
	}

	err = b.appendValidatorsSweepWithdrawals(withdrawalIndex, &withdrawals)
	if err != nil {
		return nil, 0, err
	}

	return withdrawals, processedPartialWithdrawalsCount, nil
}

func (b *BeaconState) appendPendingPartialWithdrawals(withdrawalIndex uint64, withdrawals *[]*enginev1.Withdrawal) (uint64, uint64, error) {
	if b.version < version.Electra {
		return withdrawalIndex, 0, nil
	}

	cfg := params.BeaconConfig()
	withdrawalsLimit := min(
		uint64(len(*withdrawals))+cfg.MaxPendingPartialsPerWithdrawalsSweep,
		cfg.MaxWithdrawalsPerPayload-1,
	)
	if uint64(len(*withdrawals)) > withdrawalsLimit {
		return withdrawalIndex, 0, fmt.Errorf("prior withdrawals length %d exceeds limit %d", len(*withdrawals), withdrawalsLimit)
	}

	ws := *withdrawals
	epoch := slots.ToEpoch(b.slot)
	var processedPartialWithdrawalsCount uint64
	for _, w := range b.pendingPartialWithdrawals {
		isWithdrawable := w.WithdrawableEpoch <= epoch
		hasReachedLimit := uint64(len(ws)) >= withdrawalsLimit
		if !isWithdrawable || hasReachedLimit {
			break
		}

		v, err := b.validatorAtIndexReadOnly(w.Index)
		if err != nil {
			return withdrawalIndex, 0, fmt.Errorf("failed to determine withdrawals at index %d: %w", w.Index, err)
		}
		vBal, err := b.balanceAtIndex(w.Index)
		if err != nil {
			return withdrawalIndex, 0, fmt.Errorf("could not retrieve balance at index %d: %w", w.Index, err)
		}
		hasSufficientEffectiveBalance := v.EffectiveBalance() >= cfg.MinActivationBalance
		var totalWithdrawn uint64
		for _, wi := range ws {
			if wi.ValidatorIndex == w.Index {
				totalWithdrawn += wi.Amount
			}
		}
		balance, err := mathutil.Sub64(vBal, totalWithdrawn)
		if err != nil {
			return withdrawalIndex, 0, errors.Wrapf(err, "failed to subtract balance %d with total withdrawn %d", vBal, totalWithdrawn)
		}
		hasExcessBalance := balance > cfg.MinActivationBalance
		if v.ExitEpoch() == cfg.FarFutureEpoch && hasSufficientEffectiveBalance && hasExcessBalance {
			amount := min(balance-cfg.MinActivationBalance, w.Amount)
			ws = append(ws, &enginev1.Withdrawal{
				Index:          withdrawalIndex,
				ValidatorIndex: w.Index,
				Address:        v.GetWithdrawalCredentials()[12:],
				Amount:         amount,
			})
			withdrawalIndex++
		}
		processedPartialWithdrawalsCount++
	}

	*withdrawals = ws
	return withdrawalIndex, processedPartialWithdrawalsCount, nil
}

func (b *BeaconState) appendValidatorsSweepWithdrawals(withdrawalIndex uint64, withdrawals *[]*enginev1.Withdrawal) error {
	ws := *withdrawals
	validatorIndex := b.nextWithdrawalValidatorIndex
	validatorsLen := b.validatorsLen()
	epoch := slots.ToEpoch(b.slot)
	bound := min(uint64(validatorsLen), params.BeaconConfig().MaxValidatorsPerWithdrawalsSweep)
	for range bound {
		val, err := b.validatorAtIndexReadOnly(validatorIndex)
		if err != nil {
			return errors.Wrapf(err, "could not retrieve validator at index %d", validatorIndex)
		}
		balance, err := b.balanceAtIndex(validatorIndex)
		if err != nil {
			return errors.Wrapf(err, "could not retrieve balance at index %d", validatorIndex)
		}
		if b.version >= version.Electra {
			var partiallyWithdrawnBalance uint64
			for _, w := range ws {
				if w.ValidatorIndex == validatorIndex {
					partiallyWithdrawnBalance += w.Amount
				}
			}
			balance, err = mathutil.Sub64(balance, partiallyWithdrawnBalance)
			if err != nil {
				return errors.Wrapf(err, "could not subtract balance %d with partial withdrawn balance %d", balance, partiallyWithdrawnBalance)
			}
		}
		if helpers.IsFullyWithdrawableValidator(val, balance, epoch, b.version) {
			ws = append(ws, &enginev1.Withdrawal{
				Index:          withdrawalIndex,
				ValidatorIndex: validatorIndex,
				Address:        bytesutil.SafeCopyBytes(val.GetWithdrawalCredentials()[ETH1AddressOffset:]),
				Amount:         balance,
			})
			withdrawalIndex++
		} else if helpers.IsPartiallyWithdrawableValidator(val, balance, epoch, b.version) {
			ws = append(ws, &enginev1.Withdrawal{
				Index:          withdrawalIndex,
				ValidatorIndex: validatorIndex,
				Address:        bytesutil.SafeCopyBytes(val.GetWithdrawalCredentials()[ETH1AddressOffset:]),
				Amount:         balance - helpers.ValidatorMaxEffectiveBalance(val),
			})
			withdrawalIndex++
		}
		if uint64(len(ws)) == params.BeaconConfig().MaxWithdrawalsPerPayload {
			break
		}
		validatorIndex += 1
		if uint64(validatorIndex) == uint64(validatorsLen) {
			validatorIndex = 0
		}
	}

	*withdrawals = ws
	return nil
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
