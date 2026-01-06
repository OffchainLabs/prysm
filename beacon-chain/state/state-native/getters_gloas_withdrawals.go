package state_native

import (
	"fmt"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

// ExpectedWithdrawalsGloas returns the withdrawals that a proposer will need to pack in the next block
// applied to the current state. It is also used by validators to check that the execution payload carried
// the right number of withdrawals.
//
// Spec v1.7.0-alpha.1:
//
//	def get_expected_withdrawals(state: BeaconState) -> ExpectedWithdrawals:
//	    withdrawal_index = state.next_withdrawal_index
//	    withdrawals: List[Withdrawal] = []
//
//	    # [New in Gloas:EIP7732]
//	    # Get builder withdrawals
//	    builder_withdrawals, withdrawal_index, processed_builder_withdrawals_count = (
//	        get_builder_withdrawals(state, withdrawal_index, withdrawals)
//	    )
//	    withdrawals.extend(builder_withdrawals)
//
//	    # Get partial withdrawals
//	    partial_withdrawals, withdrawal_index, processed_partial_withdrawals_count = (
//	        get_pending_partial_withdrawals(state, withdrawal_index, withdrawals)
//	    )
//	    withdrawals.extend(partial_withdrawals)
//
//	    # [New in Gloas:EIP7732]
//	    # Get builders sweep withdrawals
//	    builders_sweep_withdrawals, withdrawal_index, processed_builders_sweep_count = (
//	        get_builders_sweep_withdrawals(state, withdrawal_index, withdrawals)
//	    )
//	    withdrawals.extend(builders_sweep_withdrawals)
//
//	    # Get validators sweep withdrawals
//	    validators_sweep_withdrawals, withdrawal_index, processed_validators_sweep_count = (
//	        get_validators_sweep_withdrawals(state, withdrawal_index, withdrawals)
//	    )
//	    withdrawals.extend(validators_sweep_withdrawals)
//
//	    return ExpectedWithdrawals(
//	        withdrawals,
//	        # [New in Gloas:EIP7732]
//	        processed_builder_withdrawals_count,
//	        processed_partial_withdrawals_count,
//	        # [New in Gloas:EIP7732]
//	        processed_builders_sweep_count,
//	        processed_validators_sweep_count,
//	    )
func (b *BeaconState) ExpectedWithdrawalsGloas() ([]*enginev1.Withdrawal, uint64, uint64, primitives.BuilderIndex, error) {
	if b.version < version.Gloas {
		return nil, 0, 0, 0, errNotSupported("ExpectedWithdrawalsGloas", b.version)
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	cfg := params.BeaconConfig()
	withdrawals := make([]*enginev1.Withdrawal, 0, cfg.MaxWithdrawalsPerPayload)
	withdrawalIndex := b.nextWithdrawalIndex

	withdrawalIndex, processedBuilderWithdrawalsCount, err := b.appendBuilderWithdrawalsGloas(withdrawalIndex, &withdrawals)
	if err != nil {
		return nil, 0, 0, 0, err
	}

	withdrawalIndex, processedPartialWithdrawalsCount, err := b.appendPendingPartialWithdrawals(withdrawalIndex, &withdrawals)
	if err != nil {
		return nil, 0, 0, 0, err
	}

	withdrawalIndex, processedBuildersSweepCount, err := b.appendBuildersSweepWithdrawalsGloas(withdrawalIndex, &withdrawals)
	if err != nil {
		return nil, 0, 0, 0, err
	}
	nextBuilderIndex := b.nextWithdrawalBuilderIndex
	if buildersLen := len(b.builders); buildersLen > 0 {
		nextBuilderIndex = primitives.BuilderIndex((uint64(nextBuilderIndex) + processedBuildersSweepCount) % uint64(buildersLen))
	}

	err = b.appendValidatorsSweepWithdrawals(withdrawalIndex, &withdrawals)
	if err != nil {
		return nil, 0, 0, 0, err
	}

	return withdrawals, processedBuilderWithdrawalsCount, processedPartialWithdrawalsCount, nextBuilderIndex, nil
}

func (b *BeaconState) appendBuilderWithdrawalsGloas(withdrawalIndex uint64, withdrawals *[]*enginev1.Withdrawal) (uint64, uint64, error) {
	cfg := params.BeaconConfig()
	withdrawalsLimit := cfg.MaxWithdrawalsPerPayload - 1
	ws := *withdrawals
	var processedCount uint64
	for _, w := range b.builderPendingWithdrawals {
		if uint64(len(ws)) >= withdrawalsLimit {
			break
		}

		ws = append(ws, &enginev1.Withdrawal{
			Index: withdrawalIndex,
			ValidatorIndex: primitives.ValidatorIndex(
				uint64(primitives.BuilderIndex(w.BuilderIndex)) | cfg.BuilderIndexFlag,
			),
			Address: bytesutil.SafeCopyBytes(w.FeeRecipient),
			Amount:  uint64(w.Amount),
		})
		withdrawalIndex++
		processedCount++
	}
	*withdrawals = ws
	return withdrawalIndex, processedCount, nil
}

func (b *BeaconState) appendBuildersSweepWithdrawalsGloas(withdrawalIndex uint64, withdrawals *[]*enginev1.Withdrawal) (uint64, uint64, error) {
	cfg := params.BeaconConfig()
	withdrawalsLimit := cfg.MaxWithdrawalsPerPayload - 1
	priorWithdrawalCount := uint64(len(*withdrawals))

	if priorWithdrawalCount >= withdrawalsLimit || len(b.builders) == 0 {
		return withdrawalIndex, 0, nil
	}

	ws := *withdrawals
	epoch := slots.ToEpoch(b.slot)

	buildersLimit := len(b.builders)
	if maxBuilders := int(cfg.MaxBuildersPerWithdrawalsSweep); buildersLimit > maxBuilders {
		buildersLimit = maxBuilders
	}

	builderIndex := b.nextWithdrawalBuilderIndex
	if uint64(builderIndex) >= uint64(len(b.builders)) {
		return withdrawalIndex, 0, fmt.Errorf("next withdrawal builder index %d out of range", builderIndex)
	}
	var processedCount uint64
	for i := 0; i < buildersLimit; i++ {
		if uint64(len(ws)) >= withdrawalsLimit {
			break
		}

		builder := b.builders[builderIndex]
		if builder != nil && builder.WithdrawableEpoch <= epoch && builder.Balance > 0 {
			ws = append(ws, &enginev1.Withdrawal{
				Index:          withdrawalIndex,
				ValidatorIndex: primitives.ValidatorIndex(uint64(builderIndex) | cfg.BuilderIndexFlag),
				Address:        bytesutil.SafeCopyBytes(builder.ExecutionAddress),
				Amount:         uint64(builder.Balance),
			})
			withdrawalIndex++
		}

		builderIndex = primitives.BuilderIndex((uint64(builderIndex) + 1) % uint64(len(b.builders)))
		processedCount++
	}

	*withdrawals = ws
	return withdrawalIndex, processedCount, nil
}
