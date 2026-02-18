package blockchain

import (
	"context"

	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// PayloadAttestationReceiver interface defines the methods of chain service for receiving
// validated payload attestation messages.
type PayloadAttestationReceiver interface {
	ReceivePayloadAttestationMessage(context.Context, *ethpb.PayloadAttestationMessage) error
}

// ReceivePayloadAttestationMessage accepts a payload attestation message.
func (s *Service) ReceivePayloadAttestationMessage(ctx context.Context, a *ethpb.PayloadAttestationMessage) error {
	// TODO: Handle payload attestation message processing once Gloas is fully wired.
	return nil
}
