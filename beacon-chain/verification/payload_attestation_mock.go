package verification

import (
	"context"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	payloadattestation "github.com/prysmaticlabs/prysm/v5/consensus-types/epbs/payload-attestation"
)

type MockPayloadAttestation struct {
	ErrIncorrectPayloadAttSlot      error
	ErrIncorrectPayloadAttStatus    error
	ErrIncorrectPayloadAttValidator error
	ErrPayloadAttBlockRootNotSeen   error
	ErrPayloadAttBlockRootInvalid   error
	ErrInvalidPayloadAttMessage     error
	ErrInvalidMessageSignature      error
	ErrUnsatisfiedRequirement       error
}

var _ PayloadAttestationMsgVerifier = &MockPayloadAttestation{}

func (m *MockPayloadAttestation) VerifyCurrentSlot() error {
	return m.ErrIncorrectPayloadAttSlot
}

func (m *MockPayloadAttestation) VerifyPayloadStatus() error {
	return m.ErrIncorrectPayloadAttStatus
}

func (m *MockPayloadAttestation) VerifyValidatorInPTC(ctx context.Context, st state.BeaconState) error {
	return m.ErrIncorrectPayloadAttValidator
}

func (m *MockPayloadAttestation) VerifyBlockRootSeen(func([32]byte) bool) error {
	return m.ErrPayloadAttBlockRootNotSeen
}

func (m *MockPayloadAttestation) VerifyBlockRootValid(func([32]byte) bool) error {
	return m.ErrPayloadAttBlockRootInvalid
}

func (m *MockPayloadAttestation) VerifySignature(st state.BeaconState) (err error) {
	return m.ErrInvalidMessageSignature
}

func (m *MockPayloadAttestation) VerifiedPayloadAttestation() (payloadattestation.VerifiedROMessage, error) {
	return payloadattestation.VerifiedROMessage{}, nil
}

func (m *MockPayloadAttestation) SatisfyRequirement(req Requirement) {}
