package gloas

import (
	"bytes"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

// SameSlotAttestation checks if the attestation is for the same slot as the block root in the state.
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
func SameSlotAttestation(state state.ReadOnlyBeaconState, blockRoot [32]byte, slot primitives.Slot) (bool, error) {
	if slot == 0 {
		return true, nil
	}

	blockRootAtSlot, err := helpers.BlockRootAtSlot(state, slot)
	if err != nil {
		return false, errors.Wrapf(err, "block root at slot %d", slot)
	}
	matchingBlockRoot := bytes.Equal(blockRoot[:], blockRootAtSlot)

	blockRootAtPrevSlot, err := helpers.BlockRootAtSlot(state, slot-1)
	if err != nil {
		return false, errors.Wrapf(err, "block root at slot %d", slot-1)
	}
	matchingPrevBlockRoot := bytes.Equal(blockRoot[:], blockRootAtPrevSlot)

	return matchingBlockRoot && !matchingPrevBlockRoot, nil
}

// UpdatePendingPaymentWeight updates the builder pending payment weight based on attestation participation.
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
func UpdatePendingPaymentWeight(beaconState state.BeaconState, att ethpb.Att, indices []uint64, participatedFlags map[uint8]bool) (state.BeaconState, error) {
	if beaconState.Version() < version.Gloas {
		return beaconState, nil
	}

	data := att.GetData()
	epoch := slots.ToEpoch(beaconState.Slot())

	isSameSlot, err := SameSlotAttestation(beaconState, [32]byte(data.BeaconBlockRoot), data.Slot)
	if err != nil {
		return nil, errors.Wrap(err, "check same-slot attestation")
	}
	if !isSameSlot {
		return beaconState, nil
	}

	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	var (
		paymentSlot        primitives.Slot
		payment            *ethpb.BuilderPendingPayment
		epochParticipation []byte
	)
	if data.Target.Epoch == epoch {
		paymentSlot = slotsPerEpoch + (data.Slot % slotsPerEpoch)
		payment, err = beaconState.BuilderPendingPayment(uint64(paymentSlot))
		if err != nil {
			return nil, errors.Wrapf(err, "get builder pending payment at slot %d", paymentSlot)
		}
		epochParticipation, err = beaconState.CurrentEpochParticipation()
		if err != nil {
			return nil, errors.Wrap(err, "get current epoch participation")
		}
	} else {
		paymentSlot = data.Slot % slotsPerEpoch
		payment, err = beaconState.BuilderPendingPayment(uint64(paymentSlot))
		if err != nil {
			return nil, errors.Wrapf(err, "get builder pending payment at slot %d", paymentSlot)
		}
		epochParticipation, err = beaconState.PreviousEpochParticipation()
		if err != nil {
			return nil, errors.Wrap(err, "get previous epoch participation")
		}
	}
	if payment.Withdrawal.Amount == 0 {
		return beaconState, nil
	}

	cfg := params.BeaconConfig()
	flagIndices := []uint8{cfg.TimelySourceFlagIndex, cfg.TimelyTargetFlagIndex, cfg.TimelyHeadFlagIndex}
	for _, idx := range indices {
		if idx >= uint64(len(epochParticipation)) {
			return nil, errors.Errorf("index %d exceeds participation length %d", idx, len(epochParticipation))
		}
		participation := epochParticipation[idx]
		for _, f := range flagIndices {
			if !participatedFlags[f] {
				continue
			}
			if participation&(1<<f) == 0 {
				validator, err := beaconState.ValidatorAtIndex(primitives.ValidatorIndex(idx))
				if err != nil {
					return nil, errors.Wrapf(err, "validator at index %d", idx)
				}
				payment.Weight += primitives.Gwei(validator.EffectiveBalance)
				break
			}
		}
	}

	if err := beaconState.SetBuilderPendingPayment(uint64(paymentSlot), payment); err != nil {
		return nil, errors.Wrapf(err, "set builder pending payment at slot %d", paymentSlot)
	}

	return beaconState, nil
}
