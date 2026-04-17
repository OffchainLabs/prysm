package state_native

import (
	"errors"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native/types"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stateutil"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	pkgerrors "github.com/pkg/errors"
)

// RotateBuilderPendingPayments rotates the queue by dropping slots per epoch payments from the
// front and appending slots per epoch empty payments to the end.
// This implements: state.builder_pending_payments = state.builder_pending_payments[SLOTS_PER_EPOCH:] + [BuilderPendingPayment() for _ in range(SLOTS_PER_EPOCH)]
func (b *BeaconState) RotateBuilderPendingPayments() error {
	if b.version < version.Gloas {
		return errNotSupported("RotateBuilderPendingPayments", b.version)
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	copy(b.builderPendingPayments[:slotsPerEpoch], b.builderPendingPayments[slotsPerEpoch:2*slotsPerEpoch])

	for i := slotsPerEpoch; i < primitives.Slot(len(b.builderPendingPayments)); i++ {
		b.builderPendingPayments[i] = emptyBuilderPendingPayment
	}

	b.markFieldAsDirty(types.BuilderPendingPayments)
	b.rebuildTrie[types.BuilderPendingPayments] = true
	return nil
}

// emptyBuilderPendingPayment is a shared zero-value payment used to clear entries.
var emptyBuilderPendingPayment = &ethpb.BuilderPendingPayment{
	Withdrawal: &ethpb.BuilderPendingWithdrawal{
		FeeRecipient: make([]byte, 20),
	},
}

// AppendBuilderPendingWithdrawals appends builder pending withdrawals to the beacon state.
// If the withdrawals slice is shared, it copies the slice first to preserve references.
func (b *BeaconState) AppendBuilderPendingWithdrawals(withdrawals []*ethpb.BuilderPendingWithdrawal) error {
	if b.version < version.Gloas {
		return errNotSupported("AppendBuilderPendingWithdrawals", b.version)
	}

	if len(withdrawals) == 0 {
		return nil
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	pendingWithdrawals := b.builderPendingWithdrawals
	if b.sharedFieldReferences[types.BuilderPendingWithdrawals].Refs() > 1 {
		pendingWithdrawals = make([]*ethpb.BuilderPendingWithdrawal, 0, len(b.builderPendingWithdrawals)+len(withdrawals))
		pendingWithdrawals = append(pendingWithdrawals, b.builderPendingWithdrawals...)
		b.sharedFieldReferences[types.BuilderPendingWithdrawals].MinusRef()
		b.sharedFieldReferences[types.BuilderPendingWithdrawals] = stateutil.NewRef(1)
	}

	b.builderPendingWithdrawals = append(pendingWithdrawals, withdrawals...)
	b.markFieldAsDirty(types.BuilderPendingWithdrawals)
	return nil
}

// SetExecutionPayloadBid sets the latest execution payload bid in the state.
func (b *BeaconState) SetExecutionPayloadBid(h interfaces.ROExecutionPayloadBid) error {
	if b.version < version.Gloas {
		return errNotSupported("SetExecutionPayloadBid", b.version)
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	parentBlockHash := h.ParentBlockHash()
	parentBlockRoot := h.ParentBlockRoot()
	blockHash := h.BlockHash()
	randao := h.PrevRandao()
	blobKzgCommitments := h.BlobKzgCommitments()
	feeRecipient := h.FeeRecipient()
	b.latestExecutionPayloadBid = &ethpb.ExecutionPayloadBid{
		ParentBlockHash:    parentBlockHash[:],
		ParentBlockRoot:    parentBlockRoot[:],
		BlockHash:          blockHash[:],
		PrevRandao:         randao[:],
		GasLimit:           h.GasLimit(),
		BuilderIndex:       h.BuilderIndex(),
		Slot:               h.Slot(),
		Value:              h.Value(),
		ExecutionPayment:   h.ExecutionPayment(),
		BlobKzgCommitments: blobKzgCommitments,
		FeeRecipient:       feeRecipient[:],
	}
	b.markFieldAsDirty(types.LatestExecutionPayloadBid)

	return nil
}

// ClearBuilderPendingPayment clears a builder pending payment at the specified index.
func (b *BeaconState) ClearBuilderPendingPayment(index primitives.Slot) error {
	if b.version < version.Gloas {
		return errNotSupported("ClearBuilderPendingPayment", b.version)
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	if uint64(index) >= uint64(len(b.builderPendingPayments)) {
		return fmt.Errorf("builder pending payments index %d out of range (len=%d)", index, len(b.builderPendingPayments))
	}

	b.builderPendingPayments[index] = emptyBuilderPendingPayment

	b.markFieldAsDirty(types.BuilderPendingPayments)
	return nil
}

// QueueBuilderPayment implements the builder payment queuing logic for Gloas.
// Spec v1.7.0-alpha.0 (pseudocode):
// payment = state.builder_pending_payments[SLOTS_PER_EPOCH + state.slot % SLOTS_PER_EPOCH]
// amount = payment.withdrawal.amount
// if amount > 0:
//
//	state.builder_pending_withdrawals.append(payment.withdrawal)
//
// state.builder_pending_payments[SLOTS_PER_EPOCH + state.slot % SLOTS_PER_EPOCH] = BuilderPendingPayment()
func (b *BeaconState) QueueBuilderPayment() error {
	if b.version < version.Gloas {
		return errNotSupported("QueueBuilderPayment", b.version)
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	slot := b.slot
	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	paymentIndex := slotsPerEpoch + (slot % slotsPerEpoch)
	if uint64(paymentIndex) >= uint64(len(b.builderPendingPayments)) {
		return fmt.Errorf("builder pending payments index %d out of range (len=%d)", paymentIndex, len(b.builderPendingPayments))
	}

	payment := b.builderPendingPayments[paymentIndex]
	if payment != nil && payment.Withdrawal != nil && payment.Withdrawal.Amount > 0 {
		b.builderPendingWithdrawals = append(b.builderPendingWithdrawals, ethpb.CopyBuilderPendingWithdrawal(payment.Withdrawal))
		b.markFieldAsDirty(types.BuilderPendingWithdrawals)
	}

	b.builderPendingPayments[paymentIndex] = emptyBuilderPendingPayment
	b.markFieldAsDirty(types.BuilderPendingPayments)
	return nil
}

// SetBuilderPendingPayment sets a builder pending payment at the specified index.
func (b *BeaconState) SetBuilderPendingPayment(index primitives.Slot, payment *ethpb.BuilderPendingPayment) error {
	if b.version < version.Gloas {
		return errNotSupported("SetBuilderPendingPayment", b.version)
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	if uint64(index) >= uint64(len(b.builderPendingPayments)) {
		return fmt.Errorf("builder pending payments index %d out of range (len=%d)", index, len(b.builderPendingPayments))
	}

	b.builderPendingPayments[index] = ethpb.CopyBuilderPendingPayment(payment)

	b.markFieldAsDirty(types.BuilderPendingPayments)
	return nil
}

// UpdateExecutionPayloadAvailabilityAtIndex updates the execution payload availability bit at a specific index.
func (b *BeaconState) UpdateExecutionPayloadAvailabilityAtIndex(idx uint64, val byte) error {
	b.lock.Lock()
	defer b.lock.Unlock()

	byteIndex := idx / 8
	bitIndex := idx % 8

	if byteIndex >= uint64(len(b.executionPayloadAvailability)) {
		return fmt.Errorf("bit index %d (byte index %d) out of range for execution payload availability length %d", idx, byteIndex, len(b.executionPayloadAvailability))
	}

	if val != 0 {
		b.executionPayloadAvailability[byteIndex] |= (1 << bitIndex)
	} else {
		b.executionPayloadAvailability[byteIndex] &^= (1 << bitIndex)
	}

	b.markFieldAsDirty(types.ExecutionPayloadAvailability)
	return nil
}

// SetLatestBlockHash sets the latest execution block hash.
func (b *BeaconState) SetLatestBlockHash(hash [32]byte) error {
	if b.version < version.Gloas {
		return errNotSupported("SetLatestBlockHash", b.version)
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	b.latestBlockHash = hash[:]
	b.markFieldAsDirty(types.LatestBlockHash)
	return nil
}

// SetPayloadExpectedWithdrawals stores the expected withdrawals for the next payload.
func (b *BeaconState) SetPayloadExpectedWithdrawals(withdrawals []*enginev1.Withdrawal) error {
	if b.version < version.Gloas {
		return errNotSupported("SetPayloadExpectedWithdrawals", b.version)
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	b.payloadExpectedWithdrawals = withdrawals
	b.markFieldAsDirty(types.PayloadExpectedWithdrawals)

	return nil
}

// SetExecutionPayloadAvailability sets the execution payload availability bit for a specific slot.
func (b *BeaconState) SetExecutionPayloadAvailability(index primitives.Slot, available bool) error {
	if b.version < version.Gloas {
		return errNotSupported("SetExecutionPayloadAvailability", b.version)
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	bitIndex := index % params.BeaconConfig().SlotsPerHistoricalRoot
	byteIndex := bitIndex / 8
	bitPosition := bitIndex % 8

	if uint64(byteIndex) >= uint64(len(b.executionPayloadAvailability)) {
		return fmt.Errorf("bit index %d (byte index %d) out of range for execution payload availability length %d", bitIndex, byteIndex, len(b.executionPayloadAvailability))
	}

	// Set or clear the bit
	if available {
		b.executionPayloadAvailability[byteIndex] |= 1 << bitPosition
	} else {
		b.executionPayloadAvailability[byteIndex] &^= 1 << bitPosition
	}

	b.markFieldAsDirty(types.ExecutionPayloadAvailability)
	return nil
}

// UpdateBuilderAtIndex updates the builder at the given index.
func (b *BeaconState) UpdateBuilderAtIndex(index primitives.BuilderIndex, builder *ethpb.Builder) error {
	if b.version < version.Gloas {
		return errNotSupported("UpdateBuilderAtIndex", b.version)
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	idx := uint64(index)
	if idx >= uint64(len(b.builders)) {
		return fmt.Errorf("builder index %d out of range (len=%d)", index, len(b.builders))
	}

	builders := b.builders
	if b.sharedFieldReferences[types.Builders].Refs() > 1 {
		builders = make([]*ethpb.Builder, len(b.builders))
		copy(builders, b.builders)
		b.sharedFieldReferences[types.Builders].MinusRef()
		b.sharedFieldReferences[types.Builders] = stateutil.NewRef(1)
	}

	builders[idx] = ethpb.CopyBuilder(builder)
	b.builders = builders

	b.markFieldAsDirty(types.Builders)
	return nil
}

// IncreaseBuilderBalance increases the balance of the builder at the given index.
func (b *BeaconState) IncreaseBuilderBalance(index primitives.BuilderIndex, amount uint64) error {
	if b.version < version.Gloas {
		return errNotSupported("IncreaseBuilderBalance", b.version)
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	return b.increaseBuilderBalance(index, amount)
}

func (b *BeaconState) increaseBuilderBalance(index primitives.BuilderIndex, amount uint64) error {
	if b.builders == nil || uint64(index) >= uint64(len(b.builders)) {
		return fmt.Errorf("builder index %d out of bounds", index)
	}
	if b.builders[index] == nil {
		return fmt.Errorf("builder at index %d is nil", index)
	}

	builders := b.builders
	if b.sharedFieldReferences[types.Builders].Refs() > 1 {
		builders = make([]*ethpb.Builder, len(b.builders))
		copy(builders, b.builders)
		b.sharedFieldReferences[types.Builders].MinusRef()
		b.sharedFieldReferences[types.Builders] = stateutil.NewRef(1)
	}

	builder := ethpb.CopyBuilder(builders[index])
	builder.Balance += primitives.Gwei(amount)
	builders[index] = builder
	b.builders = builders

	b.markFieldAsDirty(types.Builders)
	return nil
}

// AddBuilderFromDeposit creates or replaces a builder entry derived from a deposit.
func (b *BeaconState) AddBuilderFromDeposit(pubkey [fieldparams.BLSPubkeyLength]byte, withdrawalCredentials [fieldparams.RootLength]byte, amount uint64) error {
	if b.version < version.Gloas {
		return errNotSupported("AddBuilderFromDeposit", b.version)
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	return b.addBuilderFromDepositAtEpoch(pubkey, withdrawalCredentials, amount, slots.ToEpoch(b.slot))
}

func (b *BeaconState) addBuilderFromDepositAtEpoch(pubkey [fieldparams.BLSPubkeyLength]byte, withdrawalCredentials [fieldparams.RootLength]byte, amount uint64, depositEpoch primitives.Epoch) error {
	if b.version < version.Gloas {
		return errNotSupported("AddBuilderFromDeposit", b.version)
	}

	currentEpoch := slots.ToEpoch(b.slot)
	index := b.builderInsertionIndex(currentEpoch)

	builder := &ethpb.Builder{
		Pubkey:            bytesutil.SafeCopyBytes(pubkey[:]),
		Version:           []byte{withdrawalCredentials[0]},
		ExecutionAddress:  bytesutil.SafeCopyBytes(withdrawalCredentials[12:]),
		Balance:           primitives.Gwei(amount),
		DepositEpoch:      depositEpoch,
		WithdrawableEpoch: params.BeaconConfig().FarFutureEpoch,
	}

	builders := b.builders
	if b.sharedFieldReferences[types.Builders].Refs() > 1 {
		builders = make([]*ethpb.Builder, len(b.builders))
		copy(builders, b.builders)
		b.sharedFieldReferences[types.Builders].MinusRef()
		b.sharedFieldReferences[types.Builders] = stateutil.NewRef(1)
	}

	if index < primitives.BuilderIndex(len(builders)) {
		builders[index] = builder
	} else {
		gap := index - primitives.BuilderIndex(len(builders)) + 1
		builders = append(builders, make([]*ethpb.Builder, gap)...)
		builders[index] = builder
	}
	b.builders = builders

	b.markFieldAsDirty(types.Builders)
	return nil
}

func (b *BeaconState) builderInsertionIndex(currentEpoch primitives.Epoch) primitives.BuilderIndex {
	for i, builder := range b.builders {
		if builder.WithdrawableEpoch <= currentEpoch && builder.Balance == 0 {
			return primitives.BuilderIndex(i)
		}
	}
	return primitives.BuilderIndex(len(b.builders))
}

// UpdatePendingPaymentWeight updates the builder pending payment weight based on attestation participation.
//
// This is a no-op for pre-Gloas forks.
//
// Spec v1.7.0-alpha pseudocode:
//
//	if data.target.epoch == get_current_epoch(state):
//	    current_epoch_target = True
//	    epoch_participation = state.current_epoch_participation
//	    payment = state.builder_pending_payments[SLOTS_PER_EPOCH + data.slot % SLOTS_PER_EPOCH]
//	else:
//	    current_epoch_target = False
//	    epoch_participation = state.previous_epoch_participation
//	    payment = state.builder_pending_payments[data.slot % SLOTS_PER_EPOCH]
//
//	proposer_reward_numerator = 0
//	for index in get_attesting_indices(state, attestation):
//	    will_set_new_flag = False
//	    for flag_index, weight in enumerate(PARTICIPATION_FLAG_WEIGHTS):
//	        if flag_index in participation_flag_indices and not has_flag(epoch_participation[index], flag_index):
//	            epoch_participation[index] = add_flag(epoch_participation[index], flag_index)
//	            proposer_reward_numerator += get_base_reward(state, index) * weight
//	            # [New in Gloas:EIP7732]
//	            will_set_new_flag = True
//	    if (
//	        will_set_new_flag
//	        and is_attestation_same_slot(state, data)
//	        and payment.withdrawal.amount > 0
//	    ):
//	        payment.weight += state.validators[index].effective_balance
//	if current_epoch_target:
//	    state.builder_pending_payments[SLOTS_PER_EPOCH + data.slot % SLOTS_PER_EPOCH] = payment
//	else:
//	    state.builder_pending_payments[data.slot % SLOTS_PER_EPOCH] = payment
func (b *BeaconState) UpdatePendingPaymentWeight(att ethpb.Att, indices []uint64, participatedFlags map[uint8]bool) error {
	var (
		paymentSlot    primitives.Slot
		currentPayment *ethpb.BuilderPendingPayment
		weight         primitives.Gwei
	)

	early, err := func() (bool, error) {
		b.lock.RLock()
		defer b.lock.RUnlock()

		if b.version < version.Gloas {
			return true, nil
		}

		data := att.GetData()
		var beaconBlockRoot [32]byte
		copy(beaconBlockRoot[:], data.BeaconBlockRoot)
		sameSlot, err := b.IsAttestationSameSlot(beaconBlockRoot, data.Slot)
		if err != nil {
			return false, err
		}
		if !sameSlot {
			return true, nil
		}

		slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
		var epochParticipation []byte

		if data.Target != nil && data.Target.Epoch == slots.ToEpoch(b.slot) {
			paymentSlot = slotsPerEpoch + (data.Slot % slotsPerEpoch)
			epochParticipation = b.currentEpochParticipation
		} else {
			paymentSlot = data.Slot % slotsPerEpoch
			epochParticipation = b.previousEpochParticipation
		}

		if uint64(paymentSlot) >= uint64(len(b.builderPendingPayments)) {
			return false, fmt.Errorf("builder pending payments index %d out of range (len=%d)", paymentSlot, len(b.builderPendingPayments))
		}
		currentPayment = b.builderPendingPayments[paymentSlot]
		if currentPayment.Withdrawal.Amount == 0 {
			return true, nil
		}

		cfg := params.BeaconConfig()
		flagIndices := []uint8{cfg.TimelySourceFlagIndex, cfg.TimelyTargetFlagIndex, cfg.TimelyHeadFlagIndex}
		for _, idx := range indices {
			if idx >= uint64(len(epochParticipation)) {
				return false, fmt.Errorf("index %d exceeds participation length %d", idx, len(epochParticipation))
			}
			participation := epochParticipation[idx]
			for _, f := range flagIndices {
				if !participatedFlags[f] {
					continue
				}
				if participation&(1<<f) == 0 {
					v, err := b.validatorAtIndexReadOnly(primitives.ValidatorIndex(idx))
					if err != nil {
						return false, fmt.Errorf("validator at index %d: %w", idx, err)
					}
					weight += primitives.Gwei(v.EffectiveBalance())
					break
				}
			}
		}
		return false, nil
	}()
	if err != nil {
		return err
	}
	if early || weight == 0 {
		return nil
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	newPayment := ethpb.CopyBuilderPendingPayment(currentPayment)
	newPayment.Weight += weight
	b.builderPendingPayments[paymentSlot] = newPayment
	b.markFieldAsDirty(types.BuilderPendingPayments)

	return nil
}

// DequeueBuilderPendingWithdrawals removes processed builder withdrawals from the front of the queue.
func (b *BeaconState) DequeueBuilderPendingWithdrawals(n uint64) error {
	if b.version < version.Gloas {
		return errNotSupported("DequeueBuilderPendingWithdrawals", b.version)
	}

	if n == 0 {
		return nil
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	if n > uint64(len(b.builderPendingWithdrawals)) {
		return errors.New("cannot dequeue more builder withdrawals than are in the queue")
	}

	if b.sharedFieldReferences[types.BuilderPendingWithdrawals].Refs() > 1 {
		withdrawals := make([]*ethpb.BuilderPendingWithdrawal, len(b.builderPendingWithdrawals))
		copy(withdrawals, b.builderPendingWithdrawals)
		b.builderPendingWithdrawals = withdrawals
		b.sharedFieldReferences[types.BuilderPendingWithdrawals].MinusRef()
		b.sharedFieldReferences[types.BuilderPendingWithdrawals] = stateutil.NewRef(1)
	}

	b.builderPendingWithdrawals = b.builderPendingWithdrawals[n:]
	b.markFieldAsDirty(types.BuilderPendingWithdrawals)
	b.rebuildTrie[types.BuilderPendingWithdrawals] = true

	return nil
}

// SetNextWithdrawalBuilderIndex sets the next builder index for the withdrawals sweep.
func (b *BeaconState) SetNextWithdrawalBuilderIndex(index primitives.BuilderIndex) error {
	if b.version < version.Gloas {
		return errNotSupported("SetNextWithdrawalBuilderIndex", b.version)
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	b.nextWithdrawalBuilderIndex = index
	b.markFieldAsDirty(types.NextWithdrawalBuilderIndex)
	return nil
}

// DecreaseWithdrawalBalances applies withdrawal balance decreases for validators and builders.
// This method holds the state lock for the full batch to avoid lock churn.
func (b *BeaconState) DecreaseWithdrawalBalances(withdrawals []*enginev1.Withdrawal) error {
	if b.version < version.Gloas {
		return errNotSupported("DecreaseWithdrawalBalances", b.version)
	}
	if len(withdrawals) == 0 {
		return nil
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	var (
		balanceIndices []uint64
		builderIndices []uint64
	)

	for _, withdrawal := range withdrawals {
		if withdrawal == nil {
			return errors.New("withdrawal is nil")
		}
		if withdrawal.Amount == 0 {
			continue
		}

		if withdrawal.ValidatorIndex.IsBuilderIndex() {
			builderIndex := withdrawal.ValidatorIndex.ToBuilderIndex()
			if err := b.decreaseBuilderBalanceLockFree(builderIndex, withdrawal.Amount); err != nil {
				return err
			}
			builderIndices = append(builderIndices, uint64(builderIndex))
			continue
		}

		balAtIdx, err := b.balanceAtIndex(withdrawal.ValidatorIndex)
		if err != nil {
			return err
		}
		newBal := decreaseBalanceWithVal(primitives.Gwei(balAtIdx), primitives.Gwei(withdrawal.Amount))
		if err := b.balancesMultiValue.UpdateAt(b, uint64(withdrawal.ValidatorIndex), uint64(newBal)); err != nil {
			return pkgerrors.Wrap(err, "could not update balances")
		}
		balanceIndices = append(balanceIndices, uint64(withdrawal.ValidatorIndex))
	}

	if len(balanceIndices) > 0 {
		b.markFieldAsDirty(types.Balances)
		b.addDirtyIndices(types.Balances, balanceIndices)
	}
	if len(builderIndices) > 0 {
		b.markFieldAsDirty(types.Builders)
		b.addDirtyIndices(types.Builders, builderIndices)
	}

	return nil
}

func (b *BeaconState) decreaseBuilderBalanceLockFree(builderIndex primitives.BuilderIndex, amount uint64) error {
	idx := uint64(builderIndex)
	if idx >= uint64(len(b.builders)) {
		return fmt.Errorf("builder index %d out of range (len=%d)", builderIndex, len(b.builders))
	}

	builders := b.builders
	if b.sharedFieldReferences[types.Builders].Refs() > 1 {
		builders = make([]*ethpb.Builder, len(b.builders))
		copy(builders, b.builders)
		b.sharedFieldReferences[types.Builders].MinusRef()
		b.sharedFieldReferences[types.Builders] = stateutil.NewRef(1)
	}

	builder := ethpb.CopyBuilder(builders[idx])
	builder.Balance = decreaseBalanceWithVal(builder.Balance, primitives.Gwei(amount))
	builders[idx] = builder
	b.builders = builders

	return nil
}

func decreaseBalanceWithVal(currBalance, delta primitives.Gwei) primitives.Gwei {
	if delta > currBalance {
		return 0
	}
	return currBalance - delta
}

// OnboardBuildersFromPendingDeposits applies any pending builder deposits at the fork.
// It mutates the state and prunes pending deposits accordingly.
//
//	<spec fn="onboard_builders_from_pending_deposits" fork="gloas" hash="2bd662c7">
//	def onboard_builders_from_pending_deposits(state: BeaconState) -> None:
//	    """
//	    Applies any pending deposit for builders, effectively
//	    onboarding builders at the fork.
//	    """
//	    validator_pubkeys = [v.pubkey for v in state.validators]
//
//	    pending_deposits = []
//	    for deposit in state.pending_deposits:
//	        # Deposits for existing validators stay in pending queue
//	        if deposit.pubkey in validator_pubkeys:
//	            pending_deposits.append(deposit)
//	            continue
//
//	        # If the pubkey is associated with a builder that was created in a
//	        # previous iteration or it is a builder deposit, try to apply the
//	        # deposit to the new/existing builder. Note that the function
//	        # apply_deposit_for_builder can mutate the state and may add a builder
//	        # to the registry. For this reason, the list of builder pubkeys must
//	        # be recomputed each iteration.
//	        builder_pubkeys = [b.pubkey for b in state.builders]
//	        is_existing_builder = deposit.pubkey in builder_pubkeys
//	        has_builder_credentials = is_builder_withdrawal_credential(deposit.withdrawal_credentials)
//	        if is_existing_builder or has_builder_credentials:
//	            apply_deposit_for_builder(
//	                state,
//	                deposit.pubkey,
//	                deposit.withdrawal_credentials,
//	                deposit.amount,
//	                deposit.signature,
//	                deposit.slot,
//	            )
//	            continue
//
//	        # If there is a pending deposit for a new validator that has a valid
//	        # signature, track the pubkey so that subsequent builder deposits for
//	        # the same pubkey stay in pending (applied to the validator later)
//	        # rather than creating a builder. Deposits with invalid signatures are
//	        # dropped here since they would fail in apply_pending_deposit anyway.
//	        if is_valid_deposit_signature(
//	            deposit.pubkey, deposit.withdrawal_credentials, deposit.amount, deposit.signature
//	        ):
//	            validator_pubkeys.append(deposit.pubkey)
//	            pending_deposits.append(deposit)
//
//	    state.pending_deposits = pending_deposits
//	</spec>
func (b *BeaconState) OnboardBuildersFromPendingDeposits() error {
	if b.version < version.Gloas {
		return errNotSupported("OnboardBuildersFromPendingDeposits", b.version)
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	pendingDeposits := b.pendingDeposits
	newPendingDeposits := make([]*ethpb.PendingDeposit, 0, len(pendingDeposits))
	newValidatorPubkeys := make(map[[fieldparams.BLSPubkeyLength]byte]bool)

	for _, deposit := range pendingDeposits {
		if deposit == nil {
			continue
		}
		pubkey := bytesutil.ToBytes48(deposit.PublicKey)
		if _, ok := newValidatorPubkeys[pubkey]; ok {
			newPendingDeposits = append(newPendingDeposits, deposit)
			continue
		}
		if _, ok := b.validatorIndexByPubkey(pubkey); ok {
			newPendingDeposits = append(newPendingDeposits, deposit)
			continue
		}

		if idx, ok := b.builderIndexByPubkey(pubkey); ok {
			if err := b.increaseBuilderBalance(idx, deposit.Amount); err != nil {
				return err
			}
			continue
		}

		if helpers.IsBuilderWithdrawalCredential(deposit.WithdrawalCredentials) {
			valid, err := helpers.IsValidDepositSignature(&ethpb.Deposit_Data{
				PublicKey:             deposit.PublicKey,
				WithdrawalCredentials: deposit.WithdrawalCredentials,
				Amount:                deposit.Amount,
				Signature:             deposit.Signature,
			})
			if err != nil {
				log.WithField("pubkey", fmt.Sprintf("%x", deposit.PublicKey)).WithError(err).Debug("Could not verify builder deposit signature")
				continue
			}
			if valid {
				depositEpoch := slots.ToEpoch(deposit.Slot)
				if err := b.addBuilderFromDepositAtEpoch(pubkey, bytesutil.ToBytes32(deposit.WithdrawalCredentials), deposit.Amount, depositEpoch); err != nil {
					log.WithField("pubkey", fmt.Sprintf("%x", deposit.PublicKey)).WithError(err).Debug("Failed to apply builder deposit")
					continue
				}
			} else {
				log.WithField("pubkey", fmt.Sprintf("%x", deposit.PublicKey)).Debug("Invalid signature for builder deposit")
			}
			continue
		}

		valid, err := helpers.IsValidDepositSignature(&ethpb.Deposit_Data{
			PublicKey:             deposit.PublicKey,
			WithdrawalCredentials: deposit.WithdrawalCredentials,
			Amount:                deposit.Amount,
			Signature:             deposit.Signature,
		})
		if err != nil {
			log.WithField("pubkey", fmt.Sprintf("%x", deposit.PublicKey)).WithError(err).Debug("Could not verify validator deposit signature")
		}
		if valid {
			newValidatorPubkeys[pubkey] = true
			newPendingDeposits = append(newPendingDeposits, deposit)
		} else {
			log.WithField("pubkey", fmt.Sprintf("%x", deposit.PublicKey)).Debug("Invalid signature for validator deposit")
		}
	}

	b.sharedFieldReferences[types.PendingDeposits].MinusRef()
	b.sharedFieldReferences[types.PendingDeposits] = stateutil.NewRef(1)
	b.pendingDeposits = newPendingDeposits
	b.markFieldAsDirty(types.PendingDeposits)

	return nil
}

// SetPTCWindow is a mutating call to the beacon state which sets the cached PTC window.
func (b *BeaconState) SetPTCWindow(window []*ethpb.PTCs) error {
	if b.version < version.Gloas {
		return errNotSupported("SetPTCWindow", b.version)
	}

	expected := expectedPTCWindowSize()
	if uint64(len(window)) != uint64(expected) {
		return fmt.Errorf("invalid size for ptc window: got %d want %d", len(window), expected)
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	b.sharedFieldReferences[types.PTCWindow].MinusRef()
	b.sharedFieldReferences[types.PTCWindow] = stateutil.NewRef(1)
	b.ptcWindow = ethpb.CopyPTCWindow(window)
	b.markFieldAsDirty(types.PTCWindow)
	return nil
}

// RotatePTCWindow shifts the PTC window left by one epoch and fills the last epoch
// with the provided new slots. This performs the rotation in-place under lock.
func (b *BeaconState) RotatePTCWindow(newEpochSlots []*ethpb.PTCs) error {
	if b.version < version.Gloas {
		return errNotSupported("RotatePTCWindow", b.version)
	}

	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	if uint64(len(newEpochSlots)) != uint64(slotsPerEpoch) {
		return fmt.Errorf("invalid new epoch slots size: got %d want %d", len(newEpochSlots), slotsPerEpoch)
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	expected := expectedPTCWindowSize()
	if uint64(len(b.ptcWindow)) != uint64(expected) {
		return fmt.Errorf("invalid ptc window size: got %d want %d", len(b.ptcWindow), expected)
	}

	b.sharedFieldReferences[types.PTCWindow].MinusRef()
	b.sharedFieldReferences[types.PTCWindow] = stateutil.NewRef(1)

	newWindow := make([]*ethpb.PTCs, expected)

	// Shift left by one epoch.
	lastEpochStart := expected - slotsPerEpoch
	copy(newWindow[:lastEpochStart], b.ptcWindow[slotsPerEpoch:])

	// Fill the last epoch with copied new slots.
	copy(newWindow[lastEpochStart:], ethpb.CopyPTCWindow(newEpochSlots))

	b.ptcWindow = newWindow

	b.markFieldAsDirty(types.PTCWindow)
	return nil
}

func expectedPTCWindowSize() primitives.Slot {
	return params.BeaconConfig().SlotsPerEpoch.Mul(uint64(2 + params.BeaconConfig().MinSeedLookahead))
}
