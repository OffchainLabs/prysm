package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/pkg/errors"
)

func (c *beaconApiValidatorClient) getExecutionPayloadEnvelope(
	ctx context.Context,
	slot primitives.Slot,
) (*ethpb.ExecutionPayloadEnvelope, error) {
	endpoint := fmt.Sprintf("/eth/v1/validator/execution_payload_envelope/%d", slot)
	var resp structs.GetValidatorExecutionPayloadEnvelopeResponse
	if err := c.handler.Get(ctx, endpoint, &resp); err != nil {
		return nil, err
	}
	if resp.Data == nil {
		return nil, errors.New("execution payload envelope data is nil")
	}
	return resp.Data.ToConsensus()
}

func (c *beaconApiValidatorClient) publishExecutionPayloadEnvelope(
	ctx context.Context,
	envelope *ethpb.SignedExecutionPayloadEnvelope,
) (*empty.Empty, error) {
	jsonEnvelope, err := structs.SignedExecutionPayloadEnvelopeFromConsensus(envelope)
	if err != nil {
		return nil, errors.Wrap(err, "could not convert envelope to JSON")
	}
	body, err := json.Marshal(jsonEnvelope)
	if err != nil {
		return nil, errors.Wrap(err, "could not marshal envelope")
	}
	if err := c.handler.Post(ctx, "/eth/v1/beacon/execution_payload_envelope", nil, bytes.NewBuffer(body), nil); err != nil {
		return nil, err
	}
	return &empty.Empty{}, nil
}
