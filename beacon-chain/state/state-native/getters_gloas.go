package state_native

import (
	"bytes"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

// LatestBlockHash returns the hash of the latest execution block.
func (b *BeaconState) LatestBlockHash() ([32]byte, error) {
	if b.version < version.Gloas {
		return [32]byte{}, errNotSupported("LatestBlockHash", b.version)
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	if b.latestBlockHash == nil {
		return [32]byte{}, nil
	}

	return [32]byte(b.latestBlockHash), nil
}

// IsAttestationSameSlot checks if the attestation is for the same slot as the block root in the state.
// Spec v1.7.0-alpha pseudocode:
//
//	is_attestation_same_slot(state, data):
//	    if data.slot == 0:
//	        return True
//
//	    blockroot = data.beacon_block_root
//	    slot_blockroot = get_block_root_at_slot(state, data.slot)
//	    prev_blockroot = get_block_root_at_slot(state, Slot(data.slot - 1))
//
//	    return blockroot == slot_blockroot and blockroot != prev_blockroot
func (b *BeaconState) IsAttestationSameSlot(blockRoot [32]byte, slot primitives.Slot) (bool, error) {
	if b.version < version.Gloas {
		return false, errNotSupported("IsAttestationSameSlot", b.version)
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	if slot == 0 {
		return true, nil
	}

	blockRootAtSlot, err := helpers.BlockRootAtSlot(b, slot)
	if err != nil {
		return false, errors.Wrapf(err, "block root at slot %d", slot)
	}
	matchingBlockRoot := bytes.Equal(blockRoot[:], blockRootAtSlot)

	blockRootAtPrevSlot, err := helpers.BlockRootAtSlot(b, slot-1)
	if err != nil {
		return false, errors.Wrapf(err, "block root at slot %d", slot-1)
	}
	matchingPrevBlockRoot := bytes.Equal(blockRoot[:], blockRootAtPrevSlot)

	return matchingBlockRoot && !matchingPrevBlockRoot, nil
}

// BuilderPubkey returns the builder pubkey at the provided index.
func (b *BeaconState) BuilderPubkey(builderIndex primitives.BuilderIndex) ([fieldparams.BLSPubkeyLength]byte, error) {
	if b.version < version.Gloas {
		return [fieldparams.BLSPubkeyLength]byte{}, errNotSupported("BuilderPubkey", b.version)
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	builder, err := b.builderAtIndex(builderIndex)
	if err != nil {
		return [fieldparams.BLSPubkeyLength]byte{}, err
	}

	var pk [fieldparams.BLSPubkeyLength]byte
	copy(pk[:], builder.Pubkey)
	return pk, nil
}

// IsActiveBuilder returns true if the builder placement is finalized and it has not initiated exit.
//
//	<spec fn="is_active_builder" fork="gloas" hash="1a599fb2">
//	def is_active_builder(state: BeaconState, builder_index: BuilderIndex) -> bool:
//	    """
//	    Check if the builder at ``builder_index`` is active for the given ``state``.
//	    """
//	    builder = state.builders[builder_index]
//	    return (
//	        # Placement in builder list is finalized
//	        builder.deposit_epoch < state.finalized_checkpoint.epoch
//	        # Has not initiated exit
//	        and builder.withdrawable_epoch == FAR_FUTURE_EPOCH
//	    )
//	</spec>
func (b *BeaconState) IsActiveBuilder(builderIndex primitives.BuilderIndex) (bool, error) {
	if b.version < version.Gloas {
		return false, errNotSupported("IsActiveBuilder", b.version)
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	builder, err := b.builderAtIndex(builderIndex)
	if err != nil {
		return false, err
	}

	finalizedEpoch := b.finalizedCheckpoint.Epoch
	return builder.DepositEpoch < finalizedEpoch && builder.WithdrawableEpoch == params.BeaconConfig().FarFutureEpoch, nil
}

// CanBuilderCoverBid returns true if the builder has enough balance to cover the given bid amount.
//
//	<spec fn="can_builder_cover_bid" fork="gloas" hash="9e3f2d7c">
//	def can_builder_cover_bid(
//	    state: BeaconState, builder_index: BuilderIndex, bid_amount: Gwei
//	) -> bool:
//	    builder_balance = state.builders[builder_index].balance
//	    pending_withdrawals_amount = get_pending_balance_to_withdraw_for_builder(state, builder_index)
//	    min_balance = MIN_DEPOSIT_AMOUNT + pending_withdrawals_amount
//	    if builder_balance < min_balance:
//	        return False
//	    return builder_balance - min_balance >= bid_amount
//	</spec>
func (b *BeaconState) CanBuilderCoverBid(builderIndex primitives.BuilderIndex, bidAmount primitives.Gwei) (bool, error) {
	if b.version < version.Gloas {
		return false, errNotSupported("CanBuilderCoverBid", b.version)
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	builder, err := b.builderAtIndex(builderIndex)
	if err != nil {
		return false, err
	}

	pendingBalanceToWithdraw := b.builderPendingBalanceToWithdraw(builderIndex)
	minBalance := params.BeaconConfig().MinDepositAmount + pendingBalanceToWithdraw

	balance := uint64(builder.Balance)
	if balance < minBalance {
		return false, nil
	}

	return balance-minBalance >= uint64(bidAmount), nil
}

// builderAtIndex intentionally returns the underlying pointer without copying.
func (b *BeaconState) builderAtIndex(builderIndex primitives.BuilderIndex) (*ethpb.Builder, error) {
	idx := uint64(builderIndex)
	if idx >= uint64(len(b.builders)) {
		return nil, fmt.Errorf("builder index %d out of range (len=%d)", builderIndex, len(b.builders))
	}

	builder := b.builders[idx]
	if builder == nil {
		return nil, fmt.Errorf("builder at index %d is nil", builderIndex)
	}
	return builder, nil
}

// builderPendingBalanceToWithdraw mirrors get_pending_balance_to_withdraw_for_builder in the spec,
// summing both pending withdrawals and pending payments for a builder.
func (b *BeaconState) builderPendingBalanceToWithdraw(builderIndex primitives.BuilderIndex) uint64 {
	var total uint64
	for _, withdrawal := range b.builderPendingWithdrawals {
		if withdrawal.BuilderIndex == builderIndex {
			total += uint64(withdrawal.Amount)
		}
	}
	for _, payment := range b.builderPendingPayments {
		if payment.Withdrawal.BuilderIndex == builderIndex {
			total += uint64(payment.Withdrawal.Amount)
		}
	}
	return total
}

// BuilderPendingPayments returns a copy of the builder pending payments.
func (b *BeaconState) BuilderPendingPayments() ([]*ethpb.BuilderPendingPayment, error) {
	if b.version < version.Gloas {
		return nil, errNotSupported("BuilderPendingPayments", b.version)
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	return b.builderPendingPaymentsVal(), nil
}

// BuilderPendingPayment returns the builder pending payment for the given index.
func (b *BeaconState) BuilderPendingPayment(index uint64) (*ethpb.BuilderPendingPayment, error) {
	if b.version < version.Gloas {
		return nil, errNotSupported("BuilderPendingPayment", b.version)
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	if index >= uint64(len(b.builderPendingPayments)) {
		return nil, fmt.Errorf("builder pending payment index %d out of range (len=%d)", index, len(b.builderPendingPayments))
	}
	return ethpb.CopyBuilderPendingPayment(b.builderPendingPayments[index]), nil
}

// LatestExecutionPayloadBid returns the cached latest execution payload bid for Gloas.
func (b *BeaconState) LatestExecutionPayloadBid() (interfaces.ROExecutionPayloadBid, error) {
	if b.version < version.Gloas {
		return nil, errNotSupported("LatestExecutionPayloadBid", b.version)
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	if b.latestExecutionPayloadBid == nil {
		return nil, nil
	}

	return blocks.WrappedROExecutionPayloadBid(b.latestExecutionPayloadBid.Copy())
}

// WithdrawalsMatchPayloadExpected returns true if the given withdrawals root matches the state's
// payload_expected_withdrawals root.
func (b *BeaconState) WithdrawalsMatchPayloadExpected(withdrawals []*enginev1.Withdrawal) (bool, error) {
	if b.version < version.Gloas {
		return false, errNotSupported("WithdrawalsMatchPayloadExpected", b.version)
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	return withdrawalsEqual(withdrawals, b.payloadExpectedWithdrawals), nil
}

func withdrawalsEqual(a, b []*enginev1.Withdrawal) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		wa := a[i]
		wb := b[i]
		if wa.Index != wb.Index ||
			wa.ValidatorIndex != wb.ValidatorIndex ||
			wa.Amount != wb.Amount ||
			!bytes.Equal(wa.Address, wb.Address) {
			return false
		}
	}
	return true
}

// IsParentBlockFull returns true if the last committed payload bid was fulfilled with a payload,
// which can only happen when both beacon block and payload were present.
//
// WARNING: This must be called on a beacon state before processing the bid for the current block
// (process_execution_payload_bid), otherwise it will compare against the in-flight bid and produce
// incorrect results.
//
//	<spec fn="is_parent_block_full" fork="gloas" hash="b59640c9">
//	def is_parent_block_full(state: BeaconState) -> bool:
//	    return state.latest_execution_payload_bid.block_hash == state.latest_block_hash
//	</spec>
func (b *BeaconState) IsParentBlockFull() (bool, error) {
	if b.version < version.Gloas {
		return false, errNotSupported("IsParentBlockFull", b.version)
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	if b.latestExecutionPayloadBid == nil {
		return false, nil
	}

	return bytes.Equal(b.latestExecutionPayloadBid.BlockHash, b.latestBlockHash), nil
}

// ExecutionPayloadAvailability returns the execution payload availability bit for the given slot.
func (b *BeaconState) ExecutionPayloadAvailability(slot primitives.Slot) (uint64, error) {
	if b.version < version.Gloas {
		return 0, errNotSupported("ExecutionPayloadAvailability", b.version)
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	slotIndex := slot % params.BeaconConfig().SlotsPerHistoricalRoot
	byteIndex := slotIndex / 8
	bitIndex := slotIndex % 8

	bit := (b.executionPayloadAvailability[byteIndex] >> bitIndex) & 1

	return uint64(bit), nil
}

// Builder returns the builder at the given index.
func (b *BeaconState) Builder(index primitives.BuilderIndex) (*ethpb.Builder, error) {
	b.lock.RLock()
	defer b.lock.RUnlock()

	if uint64(index) >= uint64(len(b.builders)) {
		return nil, fmt.Errorf("builder index %d out of bounds", index)
	}
	if b.builders[index] == nil {
		return nil, nil
	}

	return ethpb.CopyBuilder(b.builders[index]), nil
}

// BuilderIndexByPubkey returns the builder index for the given pubkey, if present.
func (b *BeaconState) BuilderIndexByPubkey(pubkey [fieldparams.BLSPubkeyLength]byte) (primitives.BuilderIndex, bool) {
	b.lock.RLock()
	defer b.lock.RUnlock()

	return b.builderIndexByPubkey(pubkey)
}

func (b *BeaconState) builderIndexByPubkey(pubkey [fieldparams.BLSPubkeyLength]byte) (primitives.BuilderIndex, bool) {
	for i, builder := range b.builders {
		if builder == nil {
			continue
		}
		if bytes.Equal(builder.Pubkey, pubkey[:]) {
			return primitives.BuilderIndex(i), true
		}
	}
	return 0, false
}

// ExpectedWithdrawalsGloas returns the withdrawals that a proposer will need to pack in the next block
// applied to the current state. It is also used by validators to check that the execution payload carried
// the right number of withdrawals.
//
//	<spec fn="get_expected_withdrawals" fork="gloas" hash="8d0675cb">
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
//	</spec>
func (b *BeaconState) ExpectedWithdrawalsGloas() (state.ExpectedWithdrawalsGloasResult, error) {
	if b.version < version.Gloas {
		return state.ExpectedWithdrawalsGloasResult{}, errNotSupported("ExpectedWithdrawalsGloas", b.version)
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	cfg := params.BeaconConfig()
	withdrawals := make([]*enginev1.Withdrawal, 0, cfg.MaxWithdrawalsPerPayload)
	withdrawalIndex := b.nextWithdrawalIndex

	withdrawalIndex, processedBuilderWithdrawalsCount, err := b.appendBuilderWithdrawals(withdrawalIndex, &withdrawals)
	if err != nil {
		return state.ExpectedWithdrawalsGloasResult{}, err
	}

	withdrawalIndex, processedPartialWithdrawalsCount, err := b.appendPendingPartialWithdrawals(withdrawalIndex, &withdrawals)
	if err != nil {
		return state.ExpectedWithdrawalsGloasResult{}, err
	}

	withdrawalIndex, nextBuilderIndex, err := b.appendBuildersSweepWithdrawals(withdrawalIndex, &withdrawals)
	if err != nil {
		return state.ExpectedWithdrawalsGloasResult{}, err
	}

	err = b.appendValidatorsSweepWithdrawals(withdrawalIndex, &withdrawals)
	if err != nil {
		return state.ExpectedWithdrawalsGloasResult{}, err
	}

	return state.ExpectedWithdrawalsGloasResult{
		Withdrawals:                      withdrawals,
		ProcessedBuilderWithdrawalsCount: processedBuilderWithdrawalsCount,
		ProcessedPartialWithdrawalsCount: processedPartialWithdrawalsCount,
		NextWithdrawalBuilderIndex:       nextBuilderIndex,
	}, nil
}

// appendBuilderWithdrawals returns builder pending withdrawals, the updated withdrawal index,
// and the processed count.
//
//	<spec fn="get_builder_withdrawals" fork="gloas" hash="d54dd146">
//	def get_builder_withdrawals(
//	    state: BeaconState,
//	    withdrawal_index: WithdrawalIndex,
//	    prior_withdrawals: Sequence[Withdrawal],
//	) -> Tuple[Sequence[Withdrawal], WithdrawalIndex, uint64]:
//	    withdrawals_limit = MAX_WITHDRAWALS_PER_PAYLOAD - 1
//	    assert len(prior_withdrawals) <= withdrawals_limit
//
//	    processed_count: uint64 = 0
//	    withdrawals: List[Withdrawal] = []
//	    for withdrawal in state.builder_pending_withdrawals:
//	        all_withdrawals = prior_withdrawals + withdrawals
//	        has_reached_limit = len(all_withdrawals) >= withdrawals_limit
//	        if has_reached_limit:
//	            break
//
//	        builder_index = withdrawal.builder_index
//	        withdrawals.append(
//	            Withdrawal(
//	                index=withdrawal_index,
//	                validator_index=convert_builder_index_to_validator_index(builder_index),
//	                address=withdrawal.fee_recipient,
//	                amount=withdrawal.amount,
//	            )
//	        )
//	        withdrawal_index += WithdrawalIndex(1)
//	        processed_count += 1
//
//	    return withdrawals, withdrawal_index, processed_count
//	</spec>
func (b *BeaconState) appendBuilderWithdrawals(withdrawalIndex uint64, withdrawals *[]*enginev1.Withdrawal) (uint64, uint64, error) {
	cfg := params.BeaconConfig()
	withdrawalsLimit := int(cfg.MaxWithdrawalsPerPayload - 1)
	ws := *withdrawals
	if len(ws) > withdrawalsLimit {
		return withdrawalIndex, 0, fmt.Errorf("prior withdrawals length %d exceeds limit %d", len(ws), withdrawalsLimit)
	}

	var processedCount uint64
	for _, w := range b.builderPendingWithdrawals {
		if len(ws) >= withdrawalsLimit {
			break
		}

		ws = append(ws, &enginev1.Withdrawal{
			Index:          withdrawalIndex,
			ValidatorIndex: w.BuilderIndex.ToValidatorIndex(),
			Address:        w.FeeRecipient,
			Amount:         uint64(w.Amount),
		})
		withdrawalIndex++
		processedCount++
	}

	*withdrawals = ws
	return withdrawalIndex, processedCount, nil
}

// appendBuildersSweepWithdrawals returns builder sweep withdrawals, the updated withdrawal index,
// and the processed count.
//
//	<spec fn="get_builders_sweep_withdrawals" fork="gloas" hash="04c1cb10">
//	def get_builders_sweep_withdrawals(
//	    state: BeaconState,
//	    withdrawal_index: WithdrawalIndex,
//	    prior_withdrawals: Sequence[Withdrawal],
//	) -> Tuple[Sequence[Withdrawal], WithdrawalIndex, uint64]:
//	    epoch = get_current_epoch(state)
//	    builders_limit = min(len(state.builders), MAX_BUILDERS_PER_WITHDRAWALS_SWEEP)
//	    withdrawals_limit = MAX_WITHDRAWALS_PER_PAYLOAD - 1
//	    assert len(prior_withdrawals) <= withdrawals_limit
//
//	    processed_count: uint64 = 0
//	    withdrawals: List[Withdrawal] = []
//	    builder_index = state.next_withdrawal_builder_index
//	    for _ in range(builders_limit):
//	        all_withdrawals = prior_withdrawals + withdrawals
//	        has_reached_limit = len(all_withdrawals) >= withdrawals_limit
//	        if has_reached_limit:
//	            break
//
//	        builder = state.builders[builder_index]
//	        if builder.withdrawable_epoch <= epoch and builder.balance > 0:
//	            withdrawals.append(
//	                Withdrawal(
//	                    index=withdrawal_index,
//	                    validator_index=convert_builder_index_to_validator_index(builder_index),
//	                    address=builder.execution_address,
//	                    amount=builder.balance,
//	                )
//	            )
//	            withdrawal_index += WithdrawalIndex(1)
//
//	        builder_index = BuilderIndex((builder_index + 1) % len(state.builders))
//	        processed_count += 1
//
//	    return withdrawals, withdrawal_index, processed_count
//	</spec>
func (b *BeaconState) appendBuildersSweepWithdrawals(withdrawalIndex uint64, withdrawals *[]*enginev1.Withdrawal) (uint64, primitives.BuilderIndex, error) {
	cfg := params.BeaconConfig()
	withdrawalsLimit := int(cfg.MaxWithdrawalsPerPayload - 1)
	if len(*withdrawals) > withdrawalsLimit {
		return withdrawalIndex, 0, fmt.Errorf("prior withdrawals length %d exceeds limit %d", len(*withdrawals), withdrawalsLimit)
	}

	ws := *withdrawals

	buildersCount := len(b.builders)
	buildersLimit := min(buildersCount, int(cfg.MaxBuildersPerWithdrawalsSweep))

	builderIndex := b.nextWithdrawalBuilderIndex
	if buildersLimit == 0 {
		return withdrawalIndex, builderIndex, nil
	}
	if uint64(builderIndex) >= uint64(buildersCount) {
		return withdrawalIndex, builderIndex, fmt.Errorf("builder index %d out of range (builders length %d)", builderIndex, buildersCount)
	}
	epoch := slots.ToEpoch(b.slot)
	for range buildersLimit {
		if len(ws) >= withdrawalsLimit {
			break
		}

		builder := b.builders[builderIndex]
		if builder == nil {
			return withdrawalIndex, 0, fmt.Errorf("builder at index %d is nil", builderIndex)
		}
		if builder.WithdrawableEpoch <= epoch && builder.Balance > 0 {
			ws = append(ws, &enginev1.Withdrawal{
				Index:          withdrawalIndex,
				ValidatorIndex: builderIndex.ToValidatorIndex(),
				Address:        builder.ExecutionAddress,
				Amount:         uint64(builder.Balance),
			})
			withdrawalIndex++
		}

		builderIndex = primitives.BuilderIndex((uint64(builderIndex) + 1) % uint64(buildersCount))
	}

	*withdrawals = ws
	return withdrawalIndex, builderIndex, nil
}
