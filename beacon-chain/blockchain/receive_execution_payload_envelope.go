package blockchain

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
)

// ExecutionPayloadEnvelopeReceiver interface defines the methods of chain service for receiving
// validated execution payload envelopes.
type ExecutionPayloadEnvelopeReceiver interface {
	ReceiveExecutionPayloadEnvelope(context.Context, interfaces.ROSignedExecutionPayloadEnvelope) error
}

// ReceiveExecutionPayloadEnvelope accepts a signed execution payload envelope.
func (s *Service) ReceiveExecutionPayloadEnvelope(_ context.Context, _ interfaces.ROSignedExecutionPayloadEnvelope) error {
	// TODO: wire into execution payload envelope processing pipeline.
	return nil
}
