package verification

import (
	"context"
	"fmt"
	"slices"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/helpers"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/signing"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	payloadattestation "github.com/prysmaticlabs/prysm/v5/consensus-types/epbs/payload-attestation"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/crypto/bls"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
	log "github.com/sirupsen/logrus"
)

// RequirementList defines a list of requirements.
type RequirementList []Requirement

const (
	RequireCurrentSlot Requirement = iota
	RequireMessageNotSeen
	RequireKnownPayloadStatus
	RequireValidatorInPTC
	RequireBlockRootSeen
	RequireBlockRootValid
	RequireSignatureValid
)

// PayloadAttGossipRequirements defines the list of requirements for gossip payload attestation messages.
var PayloadAttGossipRequirements = []Requirement{
	RequireCurrentSlot,
	RequireMessageNotSeen,
	RequireKnownPayloadStatus,
	RequireValidatorInPTC,
	RequireBlockRootSeen,
	RequireBlockRootValid,
	RequireSignatureValid,
}

// GossipPayloadAttestationMessageRequirements is a requirement list for gossip payload attestation messages.
var GossipPayloadAttestationMessageRequirements = RequirementList(PayloadAttGossipRequirements)

var (
	ErrIncorrectPayloadAttSlot      = errors.New("payload att slot does not match the current slot")
	ErrIncorrectPayloadAttStatus    = errors.New("unknown payload att status")
	ErrPayloadAttBlockRootNotSeen   = errors.New("block root not seen")
	ErrPayloadAttBlockRootInvalid   = errors.New("block root invalid")
	ErrIncorrectPayloadAttValidator = errors.New("validator not present in payload timeliness committee")
	ErrInvalidPayloadAttMessage     = errors.New("invalid payload attestation message")
)

var _ PayloadAttestationMsgVerifier = &PayloadAttMsgVerifier{}

// PayloadAttMsgVerifier is a read-only verifier for payload attestation messages.
type PayloadAttMsgVerifier struct {
	*sharedResources
	results *results
	pa      payloadattestation.ROMessage
}

// VerifyCurrentSlot verifies if the current slot matches the expected slot.
// Represents the following spec verification:
// [IGNORE] data.slot is the current slot.
func (v *PayloadAttMsgVerifier) VerifyCurrentSlot() (err error) {
	defer v.record(RequireCurrentSlot, &err)

	if v.pa.Slot() != v.clock.CurrentSlot() {
		log.WithFields(logFields(v.pa)).Errorf("does not match current slot %d", v.clock.CurrentSlot())
		return ErrIncorrectPayloadAttSlot
	}

	return nil
}

// VerifyPayloadStatus verifies if the payload status is known.
// Represents the following spec verification:
// [REJECT] data.payload_status < PAYLOAD_INVALID_STATUS.
func (v *PayloadAttMsgVerifier) VerifyPayloadStatus() (err error) {
	defer v.record(RequireKnownPayloadStatus, &err)

	if v.pa.PayloadStatus() >= primitives.PAYLOAD_INVALID_STATUS {
		log.WithFields(logFields(v.pa)).Error(ErrIncorrectPayloadAttStatus.Error())
		return ErrIncorrectPayloadAttStatus
	}

	return nil
}

// VerifyBlockRootSeen verifies if the block root has been seen before.
// Represents the following spec verification:
// [IGNORE] The attestation's data.beacon_block_root has been seen (via both gossip and non-gossip sources).
func (v *PayloadAttMsgVerifier) VerifyBlockRootSeen(parentSeen func([32]byte) bool) (err error) {
	defer v.record(RequireBlockRootSeen, &err)
	if parentSeen != nil && parentSeen(v.pa.BeaconBlockRoot()) {
		return nil
	}
	log.WithFields(logFields(v.pa)).Error(ErrPayloadAttBlockRootNotSeen.Error())
	return ErrPayloadAttBlockRootNotSeen
}

// VerifyBlockRootValid verifies if the block root is valid.
// Represents the following spec verification:
// [REJECT] The beacon block with root data.beacon_block_root passes validation.
func (v *PayloadAttMsgVerifier) VerifyBlockRootValid(badBlock func([32]byte) bool) (err error) {
	defer v.record(RequireBlockRootValid, &err)

	if badBlock != nil && badBlock(v.pa.BeaconBlockRoot()) {
		log.WithFields(logFields(v.pa)).Error(ErrPayloadAttBlockRootInvalid.Error())
		return ErrPayloadAttBlockRootInvalid
	}

	return nil
}

// VerifyValidatorInPTC verifies if the validator is present.
// Represents the following spec verification:
// [REJECT] The validator index is within the payload committee in get_ptc(state, data.slot). For the current's slot head state.
func (v *PayloadAttMsgVerifier) VerifyValidatorInPTC(ctx context.Context, st state.BeaconState) (err error) {
	defer v.record(RequireValidatorInPTC, &err)

	ptc, err := helpers.GetPayloadTimelinessCommittee(ctx, st, v.pa.Slot())
	if err != nil {
		return err
	}

	idx := slices.Index(ptc, v.pa.ValidatorIndex())
	if idx == -1 {
		log.WithFields(logFields(v.pa)).Error(ErrIncorrectPayloadAttValidator.Error())
		return ErrIncorrectPayloadAttValidator
	}

	return nil
}

// VerifySignature verifies the signature of the payload attestation message.
// Represents the following spec verification:
// [REJECT] The signature of payload_attestation_message.signature is valid with respect to the validator index.
func (v *PayloadAttMsgVerifier) VerifySignature(st state.BeaconState) (err error) {
	defer v.record(RequireSignatureValid, &err)

	err = validatePayloadAttestationMessageSignature(st, v.pa)
	if err != nil {
		if errors.Is(err, signing.ErrSigFailedToVerify) {
			log.WithFields(logFields(v.pa)).Error("signature failed to validate")
		} else {
			log.WithFields(logFields(v.pa)).WithError(err).Error("could not validate signature")
		}
		return err
	}

	return nil
}

// VerifiedPayloadAttestation returns a verified payload attestation message by checking all requirements.
func (v *PayloadAttMsgVerifier) VerifiedPayloadAttestation() (payloadattestation.VerifiedROMessage, error) {
	if v.results.allSatisfied() {
		return payloadattestation.NewVerifiedROMessage(v.pa), nil
	}
	return payloadattestation.VerifiedROMessage{}, ErrInvalidPayloadAttMessage
}

// SatisfyRequirement allows the caller to manually mark a requirement as satisfied.
func (v *PayloadAttMsgVerifier) SatisfyRequirement(req Requirement) {
	v.record(req, nil)
}

// ValidatePayloadAttestationMessageSignature verifies the signature of a payload attestation message.
func validatePayloadAttestationMessageSignature(st state.BeaconState, payloadAtt payloadattestation.ROMessage) error {
	val, err := st.ValidatorAtIndex(payloadAtt.ValidatorIndex())
	if err != nil {
		return err
	}

	pub, err := bls.PublicKeyFromBytes(val.PublicKey)
	if err != nil {
		return err
	}

	s := payloadAtt.Signature()
	sig, err := bls.SignatureFromBytes(s[:])
	if err != nil {
		return err
	}

	currentEpoch := slots.ToEpoch(st.Slot())
	domain, err := signing.Domain(st.Fork(), currentEpoch, params.BeaconConfig().DomainPTCAttester, st.GenesisValidatorsRoot())
	if err != nil {
		return err
	}

	root, err := payloadAtt.SigningRoot(domain)
	if err != nil {
		return err
	}

	if !sig.Verify(pub, root[:]) {
		return signing.ErrSigFailedToVerify
	}
	return nil
}

// record records the result of a requirement verification.
func (v *PayloadAttMsgVerifier) record(req Requirement, err *error) {
	if err == nil || *err == nil {
		v.results.record(req, nil)
		return
	}

	v.results.record(req, *err)
}

// logFields returns log fields for a ROMessage instance.
func logFields(payload payloadattestation.ROMessage) log.Fields {
	return log.Fields{
		"slot":            payload.Slot(),
		"validatorIndex":  payload.ValidatorIndex(),
		"signature":       fmt.Sprintf("%#x", payload.Signature()),
		"beaconBlockRoot": fmt.Sprintf("%#x", payload.BeaconBlockRoot()),
		"payloadStatus":   payload.PayloadStatus(),
	}
}
