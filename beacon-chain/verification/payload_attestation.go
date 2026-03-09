package verification

import (
	"context"
	"errors"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/gloas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	payloadattestation "github.com/OffchainLabs/prysm/v7/consensus-types/payload-attestation"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

// RequirementList defines a list of requirements.
type RequirementList []Requirement

// PayloadAttGossipRequirements defines the list of requirements for gossip payload attestation messages.
var PayloadAttGossipRequirements = []Requirement{
	RequireCurrentSlot,
	RequireMessageNotSeen,
	RequireValidatorInPTC,
	RequireBlockRootSeen,
	RequireBlockRootValid,
	RequireSignatureValid,
}

// GossipPayloadAttestationMessageRequirements is a requirement list for gossip payload attestation messages.
var GossipPayloadAttestationMessageRequirements = RequirementList(PayloadAttGossipRequirements)

var (
	ErrIncorrectPayloadAttSlot      = errors.New("payload att slot does not match the current slot")
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

	currentSlot := v.clock.CurrentSlot()
	if v.pa.Slot() != currentSlot {
		return fmt.Errorf("%w: got %d want %d", ErrIncorrectPayloadAttSlot, v.pa.Slot(), currentSlot)
	}

	return nil
}

// VerifyBlockRootSeen verifies if the block root has been seen before.
// Represents the following spec verification:
// [IGNORE] The attestation's data.beacon_block_root has been seen (via both gossip and non-gossip sources).
func (v *PayloadAttMsgVerifier) VerifyBlockRootSeen(blockRootSeen func([32]byte) bool) (err error) {
	defer v.record(RequireBlockRootSeen, &err)
	if blockRootSeen != nil && blockRootSeen(v.pa.BeaconBlockRoot()) {
		return nil
	}
	return fmt.Errorf("%w: root=%#x", ErrPayloadAttBlockRootNotSeen, v.pa.BeaconBlockRoot())
}

// VerifyBlockRootValid verifies if the block root is valid.
// Represents the following spec verification:
// [REJECT] The beacon block with root data.beacon_block_root passes validation.
func (v *PayloadAttMsgVerifier) VerifyBlockRootValid(badBlock func([32]byte) bool) (err error) {
	defer v.record(RequireBlockRootValid, &err)

	if badBlock != nil && badBlock(v.pa.BeaconBlockRoot()) {
		return fmt.Errorf("%w: root=%#x", ErrPayloadAttBlockRootInvalid, v.pa.BeaconBlockRoot())
	}

	return nil
}

// VerifyValidatorInPTC verifies if the validator is present.
// Represents the following spec verification:
// [REJECT] The validator index is within the payload committee in get_ptc(state, data.slot). For the current's slot head state.
func (v *PayloadAttMsgVerifier) VerifyValidatorInPTC(ctx context.Context, st state.ReadOnlyBeaconState) (err error) {
	defer v.record(RequireValidatorInPTC, &err)

	if _, err := gloas.PayloadCommitteeIndex(ctx, st, v.pa.Slot(), v.pa.ValidatorIndex()); errors.Is(err, gloas.ErrValidatorNotInPTC) {
		return fmt.Errorf("%w: validatorIndex=%d", ErrIncorrectPayloadAttValidator, v.pa.ValidatorIndex())
	} else if err != nil {
		return err
	}

	return nil
}

// VerifySignature verifies the signature of the payload attestation message.
// Represents the following spec verification:
// [REJECT] The signature of payload_attestation_message.signature is valid with respect to the validator index.
func (v *PayloadAttMsgVerifier) VerifySignature(st state.ReadOnlyBeaconState) (err error) {
	defer v.record(RequireSignatureValid, &err)

	err = validatePayloadAttestationMessageSignature(st, v.pa)
	if err != nil {
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
func validatePayloadAttestationMessageSignature(st state.ReadOnlyBeaconState, payloadAtt payloadattestation.ROMessage) error {
	val, err := st.ValidatorAtIndex(payloadAtt.ValidatorIndex())
	if err != nil {
		return fmt.Errorf("validator %d: %w", payloadAtt.ValidatorIndex(), err)
	}

	pub, err := bls.PublicKeyFromBytes(val.PublicKey)
	if err != nil {
		return fmt.Errorf("public key: %w", err)
	}

	s := payloadAtt.Signature()
	sig, err := bls.SignatureFromBytes(s[:])
	if err != nil {
		return fmt.Errorf("signature bytes: %w", err)
	}

	currentEpoch := slots.ToEpoch(st.Slot())
	domain, err := signing.Domain(st.Fork(), currentEpoch, params.BeaconConfig().DomainPTCAttester, st.GenesisValidatorsRoot())
	if err != nil {
		return fmt.Errorf("domain: %w", err)
	}

	root, err := payloadAtt.SigningRoot(domain)
	if err != nil {
		return fmt.Errorf("signing root: %w", err)
	}

	if !sig.Verify(pub, root[:]) {
		return fmt.Errorf("verify signature: %w", signing.ErrSigFailedToVerify)
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
