package helpers

import (
	"context"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/math"
	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
)

var (
	errNilMessage            = errors.New("nil PayloadAttestationMessage")
	errNilData               = errors.New("nil PayloadAttestationData")
	errNilBeaconBlockRoot    = errors.New("nil BeaconBlockRoot")
	errNilPayloadAttestation = errors.New("nil PayloadAttestation")
	errNilSignature          = errors.New("nil Signature")
	errNilAggregationBits    = errors.New("nil AggregationBits")
	errPreEPBSState          = errors.New("beacon state pre ePBS fork")
	errCommitteeOverflow     = errors.New("beacon committee of insufficient size")
)

// ValidateNilPayloadAttestationData checks if any composite field of the
// payload attestation data is nil
func ValidateNilPayloadAttestationData(data *eth.PayloadAttestationData) error {
	if data == nil {
		return errNilData
	}
	if data.BeaconBlockRoot == nil {
		return errNilBeaconBlockRoot
	}
	return nil
}

// ValidateNilPayloadAttestationMessage checks if any composite field of the
// payload attestation message is nil
func ValidateNilPayloadAttestationMessage(att *eth.PayloadAttestationMessage) error {
	if att == nil {
		return errNilMessage
	}
	if att.Signature == nil {
		return errNilSignature
	}
	return ValidateNilPayloadAttestationData(att.Data)
}

// ValidateNilPayloadAttestation checks if any composite field of the
// payload attestation is nil
func ValidateNilPayloadAttestation(att *eth.PayloadAttestation) error {
	if att == nil {
		return errNilPayloadAttestation
	}
	if att.AggregationBits == nil {
		return errNilAggregationBits
	}
	if att.Signature == nil {
		return errNilSignature
	}
	return ValidateNilPayloadAttestationData(att.Data)
}

// GetPayloadTimelinessCommittee returns the PTC for the given slot, computed from the passed state as in the
// spec function `get_ptc`.
func GetPayloadTimelinessCommittee(ctx context.Context, state state.ReadOnlyBeaconState, slot primitives.Slot) (indices []primitives.ValidatorIndex, err error) {
	if state.Version() < version.EPBS {
		return nil, errPreEPBSState
	}
	epoch := slots.ToEpoch(slot)
	activeCount, err := ActiveValidatorCount(ctx, state, epoch)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute active validator count")
	}
	committeesPerSlot, membersPerCommittee := PtcAllocation(activeCount)
	for i := uint64(0); i < committeesPerSlot; i++ {
		committee, err := BeaconCommitteeFromState(ctx, state, slot, primitives.CommitteeIndex(i))
		if err != nil {
			return nil, err
		}
		if uint64(len(committee)) < membersPerCommittee {
			return nil, errCommitteeOverflow
		}
		start := uint64(len(committee)) - membersPerCommittee
		indices = append(indices, committee[start:]...)
	}
	return
}

// PtcAllocation returns:
// 1. The number of beacon committees that PTC will borrow from in a slot.
// 2. The number of validators that PTC will borrow from in a beacon committee.
func PtcAllocation(totalActive uint64) (committeesPerSlot, membersPerCommittee uint64) {
	slotCommittees := SlotCommitteeCount(totalActive)
	committeesPerSlot = math.LargestPowerOfTwo(math.Min(slotCommittees, fieldparams.PTCSize))
	membersPerCommittee = fieldparams.PTCSize / committeesPerSlot
	return
}
