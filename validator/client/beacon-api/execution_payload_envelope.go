package beacon_api

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/pkg/errors"
)

// TODO: Implement Gloas beacon API client methods.

// getExecutionPayloadEnvelope retrieves the execution payload envelope for the given slot.
func (c *beaconApiValidatorClient) getExecutionPayloadEnvelope(
	ctx context.Context,
	slot primitives.Slot,
) (*ethpb.ExecutionPayloadEnvelope, error) {
	return nil, errors.New("getExecutionPayloadEnvelope not yet implemented")
}

// publishExecutionPayloadEnvelope broadcasts a signed execution payload envelope.
func (c *beaconApiValidatorClient) publishExecutionPayloadEnvelope(
	ctx context.Context,
	envelope *ethpb.SignedExecutionPayloadEnvelope,
) (*empty.Empty, error) {
	return nil, errors.New("publishExecutionPayloadEnvelope not yet implemented")
}
