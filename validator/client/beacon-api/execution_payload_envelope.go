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
	envelope, _, _ := c.envelopeCache.peek(slot)
	if envelope != nil {
		return envelope, nil
	}
	endpoint := fmt.Sprintf("/eth/v1/validator/execution_payload_envelope/%d", slot)
	var resp structs.GetValidatorExecutionPayloadEnvelopeResponse
	if err := c.handler.Get(ctx, endpoint, &resp); err != nil {
		return nil, errors.Wrap(err, "could not get execution payload envelope")
	}
	if resp.Data == nil {
		return nil, errors.New("execution payload envelope data is nil")
	}
	envelope, err := resp.Data.ToConsensus()
	if err != nil {
		return nil, errors.Wrap(err, "could not convert execution payload envelope to consensus")
	}
	return envelope, nil
}

func (c *beaconApiValidatorClient) publishExecutionPayloadEnvelope(
	ctx context.Context,
	envelope *ethpb.SignedExecutionPayloadEnvelope,
) (*empty.Empty, error) {
	// In stateless mode, drain the envelope cache for this slot so the publish
	// includes blobs and KZG proofs the receiving beacon node may not have. A
	// missing cache entry (e.g. running stateless against a node that never
	// returned blobs) falls through to the bare-envelope path so the call still
	// reaches the receiver.
	if c.stateless && envelope != nil && envelope.Message != nil && envelope.Message.Payload != nil {
		cachedEnv, blobs, kzgProofs := c.envelopeCache.Take(primitives.Slot(envelope.Message.Payload.SlotNumber))
		if cachedEnv != nil {
			contents, err := structs.SignedExecutionPayloadEnvelopeContentsFromConsensus(envelope, kzgProofs, blobs)
			if err != nil {
				return nil, errors.Wrap(err, "could not convert envelope contents to JSON")
			}
			body, err := json.Marshal(contents)
			if err != nil {
				return nil, errors.Wrap(err, "could not marshal envelope contents")
			}
			if err := c.handler.Post(ctx, "/eth/v1/beacon/execution_payload_envelope", nil, bytes.NewBuffer(body), nil); err != nil {
				return nil, errors.Wrap(err, "could not publish execution payload envelope contents")
			}
			return &empty.Empty{}, nil
		}
	}

	jsonEnvelope, err := structs.SignedExecutionPayloadEnvelopeFromConsensus(envelope)
	if err != nil {
		return nil, errors.Wrap(err, "could not convert envelope to JSON")
	}
	body, err := json.Marshal(jsonEnvelope)
	if err != nil {
		return nil, errors.Wrap(err, "could not marshal envelope")
	}
	if err := c.handler.Post(ctx, "/eth/v1/beacon/execution_payload_envelope", nil, bytes.NewBuffer(body), nil); err != nil {
		return nil, errors.Wrap(err, "could not publish execution payload envelope")
	}
	return &empty.Empty{}, nil
}
