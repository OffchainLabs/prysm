package verification

import (
	"context"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	payloadattestation "github.com/prysmaticlabs/prysm/v5/consensus-types/epbs/payload-attestation"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
)

// PayloadAttestationMsgVerifier defines the methods implemented by the ROPayloadAttestation.
// It is similar to BlobVerifier, but for payload attestation messages.
type PayloadAttestationMsgVerifier interface {
	VerifyCurrentSlot() error
	VerifyPayloadStatus() error
	VerifyBlockRootSeen(func([32]byte) bool) error
	VerifyBlockRootValid(func([32]byte) bool) error
	VerifyValidatorInPTC(context.Context, state.BeaconState) error
	VerifySignature(state.BeaconState) error
	VerifiedPayloadAttestation() (payloadattestation.VerifiedROMessage, error)
	SatisfyRequirement(Requirement)
}

// NewPayloadAttestationMsgVerifier is a function signature that can be used by code that needs to be
// able to mock Initializer.NewPayloadAttestationMsgVerifier without complex setup.
type NewPayloadAttestationMsgVerifier func(pa payloadattestation.ROMessage, reqs []Requirement) PayloadAttestationMsgVerifier

// ExecutionPayloadEnvelopeVerifier defines the methods implemented by the ROSignedExecutionPayloadEnvelope.
// It is similar to BlobVerifier, but for signed execution payload envelope.
type ExecutionPayloadEnvelopeVerifier interface {
	VerifyBlockRootSeen(func([32]byte) bool) error
	VerifyBlockRootValid(func([32]byte) bool) error
	VerifyBuilderValid(interfaces.ROExecutionPayloadHeaderEPBS) error
	VerifyPayloadHash(interfaces.ROExecutionPayloadHeaderEPBS) error
	VerifySignature(state.BeaconState) error
	SetSlot(primitives.Slot) error
	SatisfyRequirement(Requirement)
}

// NewExecutionPayloadEnvelopeVerifier is a function signature that can be used by code that needs to be
// able to mock Initializer.NewExecutionPayloadEnvelopeVerifier without complex setup.
type NewExecutionPayloadEnvelopeVerifier func(e interfaces.ROSignedExecutionPayloadEnvelope, reqs []Requirement) ExecutionPayloadEnvelopeVerifier
