package verification

import (
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
)

type MockExecutionPayloadEnvelope struct {
	ErrBlockRootNotSeen       error
	ErrBlockRootInvalid       error
	ErrBuilderIndexInvalid    error
	ErrBlockHashInvalid       error
	ErrSignatureInvalid       error
	ErrUnsatisfiedRequirement error
}

var _ ExecutionPayloadEnvelopeVerifier = &MockExecutionPayloadEnvelope{}

func (e *MockExecutionPayloadEnvelope) VerifyBlockRootSeen(_ func([32]byte) bool) error {
	return e.ErrBlockRootNotSeen
}

func (e *MockExecutionPayloadEnvelope) VerifyBlockRootValid(_ func([32]byte) bool) error {
	return e.ErrBlockRootInvalid
}

func (e *MockExecutionPayloadEnvelope) VerifyBuilderValid(_ interfaces.ROExecutionPayloadHeaderEPBS) error {
	return e.ErrBuilderIndexInvalid
}

func (e *MockExecutionPayloadEnvelope) VerifyPayloadHash(_ interfaces.ROExecutionPayloadHeaderEPBS) error {
	return e.ErrBlockHashInvalid
}

func (e *MockExecutionPayloadEnvelope) VerifySignature(_ state.BeaconState) error {
	return e.ErrSignatureInvalid
}

func (e *MockExecutionPayloadEnvelope) SetSlot(_ primitives.Slot) error {
	return nil
}

func (e *MockExecutionPayloadEnvelope) SatisfyRequirement(_ Requirement) {}
