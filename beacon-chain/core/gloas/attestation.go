package gloas

import (
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/pkg/errors"
)

// MatchingPayload returns true if the attestation's committee index matches the expected payload index.
//
// For pre-Gloas forks, this always returns true.
//
// The payload availability of the attested block is tracked at parentSlot (the
// slot of the parent block of the block being processed), even when the
// attestation's slot is a skipped slot.
//
// Spec v1.7.0-alpha.13 (pseudocode, from get_attestation_participation_flag_indices):
//
//	# [New in Gloas:EIP7732]
//	if is_attestation_same_slot(state, data):
//	    assert data.index == 0
//	    payload_matches = True
//	else:
//	    slot_index = parent_slot % SLOTS_PER_HISTORICAL_ROOT
//	    payload_index = state.execution_payload_availability[slot_index]
//	    payload_matches = data.index == payload_index
func MatchingPayload(
	beaconState state.ReadOnlyBeaconState,
	beaconBlockRoot [32]byte,
	dataSlot primitives.Slot,
	parentSlot primitives.Slot,
	committeeIndex uint64,
) (bool, error) {
	if beaconState.Version() < version.Gloas {
		return true, nil
	}

	sameSlot, err := beaconState.IsAttestationSameSlot(beaconBlockRoot, dataSlot)
	if err != nil {
		return false, errors.Wrap(err, "failed to get same slot attestation status")
	}
	if sameSlot {
		if committeeIndex != 0 {
			return false, fmt.Errorf("committee index %d for same slot attestation must be 0", committeeIndex)
		}
		return true, nil
	}

	executionPayloadAvail, err := beaconState.ExecutionPayloadAvailability(parentSlot)
	if err != nil {
		return false, errors.Wrap(err, "failed to get execution payload availability status")
	}
	return executionPayloadAvail == committeeIndex, nil
}

// ParentSlotFromState returns the slot of the parent block for attestation
// processing, read from the state's latest execution payload bid. This is only
// valid on a state to which the current block's bid has NOT yet been applied
// (e.g. reward computation or attestation scoring outside the transition);
// during block processing, use the slot returned by ProcessExecutionPayloadBid,
// which captures the value before the bid is overwritten.
//
// For pre-Gloas states it returns 0, which is ignored by MatchingPayload.
func ParentSlotFromState(beaconState state.ReadOnlyBeaconState) (primitives.Slot, error) {
	if beaconState.Version() < version.Gloas {
		return 0, nil
	}
	bid, err := beaconState.LatestExecutionPayloadBid()
	if err != nil {
		return 0, errors.Wrap(err, "failed to get latest execution payload bid")
	}
	if bid == nil {
		return 0, nil
	}
	return bid.Slot(), nil
}
