package helpers

import (
	"context"
	"slices"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/signing"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/epbs"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/crypto/bls"
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
	committees, err := BeaconCommittees(ctx, state, slot)
	if err != nil {
		return nil, errors.Wrap(err, "could not get beacon committees")
	}
	committeesPerSlot, membersPerCommittee := PtcAllocation(len(committees))
	for i, committee := range committees {
		if uint64(i) >= committeesPerSlot {
			return
		}
		if uint64(len(committee)) < membersPerCommittee {
			return nil, errCommitteeOverflow
		}
		indices = append(indices, committee[:membersPerCommittee]...)
	}
	return
}

// PtcAllocation returns:
// 1. The number of beacon committees that PTC will borrow from in a slot.
// 2. The number of validators that PTC will borrow from in a beacon committee.
func PtcAllocation(slotCommittees int) (committeesPerSlot, membersPerCommittee uint64) {
	committeesPerSlot = math.LargestPowerOfTwo(math.Min(uint64(slotCommittees), fieldparams.PTCSize))
	membersPerCommittee = fieldparams.PTCSize / committeesPerSlot
	return
}

// GetPayloadAttestingIndices returns the set of attester indices corresponding to the given PayloadAttestation.
//
// Spec pseudocode definition:
//
//	def get_payload_attesting_indices(state: BeaconState, slot: Slot,
//		payload_attestation: PayloadAttestation) -> Set[ValidatorIndex]:
//	"""
//	Return the set of attesting indices corresponding to ``payload_attestation``.
//	"""
//	ptc = get_ptc(state, slot)
//	return set(index for i, index in enumerate(ptc) if payload_attestation.aggregation_bits[i])
func GetPayloadAttestingIndices(ctx context.Context, state state.ReadOnlyBeaconState, slot primitives.Slot, att *eth.PayloadAttestation) (indices []primitives.ValidatorIndex, err error) {
	if state.Version() < version.EPBS {
		return nil, errPreEPBSState
	}

	ptc, err := GetPayloadTimelinessCommittee(ctx, state, slot)
	if err != nil {
		return nil, err
	}

	for i, validatorIndex := range ptc {
		if att.AggregationBits.BitAt(uint64(i)) {
			indices = append(indices, validatorIndex)
		}
	}

	return
}

// GetIndexedPayloadAttestation replaces a PayloadAttestation's AggregationBits with sorted AttestingIndices and returns an IndexedPayloadAttestation.
//
// Spec pseudocode definition:
//
//	def get_indexed_payload_attestation(state: BeaconState, slot: Slot,
//		payload_attestation: PayloadAttestation) -> IndexedPayloadAttestation:
//	"""
//	Return the indexed payload attestation corresponding to ``payload_attestation``.
//	"""
//	attesting_indices = get_payload_attesting_indices(state, slot, payload_attestation)
//
//	return IndexedPayloadAttestation(
//	attesting_indices=sorted(attesting_indices),
//	data=payload_attestation.data,
//	signature=payload_attestation.signature,
//	)
func GetIndexedPayloadAttestation(ctx context.Context, state state.ReadOnlyBeaconState, slot primitives.Slot, att *eth.PayloadAttestation) (*epbs.IndexedPayloadAttestation, error) {
	if state.Version() < version.EPBS {
		return nil, errPreEPBSState
	}

	attestingIndices, err := GetPayloadAttestingIndices(ctx, state, slot, att)
	if err != nil {
		return nil, err
	}

	slices.Sort(attestingIndices)

	return &epbs.IndexedPayloadAttestation{
		AttestingIndices: attestingIndices,
		Data:             att.Data,
		Signature:        att.Signature,
	}, nil
}

// IsValidIndexedPayloadAttestation validates the given IndexedPayloadAttestation.
//
// Spec pseudocode definition:
//
//	def is_valid_indexed_payload_attestation(
//		state: BeaconState,
//		indexed_payload_attestation: IndexedPayloadAttestation) -> bool:
//	"""
//	Check if ``indexed_payload_attestation`` is not empty, has sorted and unique indices and has
//	a valid aggregate signature.
//	"""
//	# Verify the data is valid
//	if indexed_payload_attestation.data.payload_status >= PAYLOAD_INVALID_STATUS:
//		return False
//
//	# Verify indices are sorted and unique
//	indices = indexed_payload_attestation.attesting_indices
//	if len(indices) == 0 or not indices == sorted(set(indices)):
//		return False
//
//	# Verify aggregate signature
//	pubkeys = [state.validators[i].pubkey for i in indices]
//	domain = get_domain(state, DOMAIN_PTC_ATTESTER, None)
//	signing_root = compute_signing_root(indexed_payload_attestation.data, domain)
//	return bls.FastAggregateVerify(pubkeys, signing_root, indexed_payload_attestation.signature)
func IsValidIndexedPayloadAttestation(state state.ReadOnlyBeaconState, att *epbs.IndexedPayloadAttestation) (bool, error) {
	if state.Version() < version.EPBS {
		return false, errPreEPBSState
	}

	// Verify the data is valid.
	if att.Data.PayloadStatus >= primitives.PAYLOAD_INVALID_STATUS {
		return false, nil
	}

	// Verify indices are sorted and unique.
	indices := att.AttestingIndices
	slices.Sort(indices)
	if len(indices) == 0 || !slices.Equal(att.AttestingIndices, indices) {
		return false, nil
	}

	// Verify aggregate signature.
	publicKeys := make([]bls.PublicKey, len(indices))
	for i, index := range indices {
		validator, err := state.ValidatorAtIndexReadOnly(index)
		if err != nil {
			return false, err
		}

		publicKeyBytes := validator.PublicKey()
		publicKey, err := bls.PublicKeyFromBytes(publicKeyBytes[:])
		if err != nil {
			return false, err
		}

		publicKeys[i] = publicKey
	}

	domain, err := signing.Domain(
		state.Fork(),
		slots.ToEpoch(state.Slot()),
		params.BeaconConfig().DomainPTCAttester,
		state.GenesisValidatorsRoot(),
	)
	if err != nil {
		return false, err
	}

	signingRoot, err := signing.ComputeSigningRoot(att.Data, domain)
	if err != nil {
		return false, err
	}

	signature, err := bls.SignatureFromBytes(att.Signature)
	if err != nil {
		return false, err
	}

	return signature.FastAggregateVerify(publicKeys, signingRoot), nil
}
