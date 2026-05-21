package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
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
	body, header, err := c.handler.GetSSZ(ctx, endpoint)
	if err != nil {
		return nil, errors.Wrap(err, "could not get execution payload envelope")
	}
	if strings.Contains(header.Get("Content-Type"), api.OctetStreamMediaType) {
		env := &ethpb.ExecutionPayloadEnvelope{}
		if err := env.UnmarshalSSZ(body); err != nil {
			return nil, errors.Wrap(err, "could not unmarshal execution payload envelope SSZ")
		}
		return env, nil
	}
	var resp structs.GetValidatorExecutionPayloadEnvelopeResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, errors.Wrap(err, "could not decode execution payload envelope JSON")
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
	const endpoint = "/eth/v1/beacon/execution_payload_envelope"

	// Stateless mode requires the envelope cache (populated by the v4 block
	// fetch) to provide blobs+proofs; a miss means we would silently lose blob
	// broadcast against a stateless target BN, so error out.
	if c.stateless && envelope != nil && envelope.Message != nil && envelope.Message.Payload != nil {
		slot := primitives.Slot(envelope.Message.Payload.SlotNumber)
		cachedEnv, blobs, kzgProofs := c.envelopeCache.Take(slot)
		if cachedEnv == nil {
			return nil, errors.Errorf("stateless publish: envelope cache miss for slot %d", slot)
		}
		contents := &ethpb.SignedExecutionPayloadEnvelopeContents{
			SignedExecutionPayloadEnvelope: envelope,
			KzgProofs:                      kzgProofs,
			Blobs:                          blobs,
		}
		ssz, err := contents.MarshalSSZ()
		if err != nil {
			return nil, errors.Wrap(err, "could not marshal envelope contents SSZ")
		}
		jsonFn := func() ([]byte, error) {
			j, jerr := structs.SignedExecutionPayloadEnvelopeContentsFromConsensus(envelope, kzgProofs, blobs)
			if jerr != nil {
				return nil, jerr
			}
			return json.Marshal(j)
		}
		if err := c.postEnvelope(ctx, endpoint, ssz, jsonFn); err != nil {
			return nil, errors.Wrap(err, "could not publish execution payload envelope contents")
		}
		return &empty.Empty{}, nil
	}

	ssz, err := envelope.MarshalSSZ()
	if err != nil {
		return nil, errors.Wrap(err, "could not marshal envelope SSZ")
	}
	jsonFn := func() ([]byte, error) {
		j, jerr := structs.SignedExecutionPayloadEnvelopeFromConsensus(envelope)
		if jerr != nil {
			return nil, jerr
		}
		return json.Marshal(j)
	}
	if err := c.postEnvelope(ctx, endpoint, ssz, jsonFn); err != nil {
		return nil, errors.Wrap(err, "could not publish execution payload envelope")
	}
	return &empty.Empty{}, nil
}

// postEnvelope publishes SSZ first; on 406 Not Acceptable falls back to JSON.
func (c *beaconApiValidatorClient) postEnvelope(ctx context.Context, endpoint string, ssz []byte, jsonFn func() ([]byte, error)) error {
	_, _, err := c.handler.PostSSZ(ctx, endpoint, nil, bytes.NewBuffer(ssz))
	if err == nil {
		return nil
	}
	errJson := &httputil.DefaultJsonError{}
	if !errors.As(err, &errJson) {
		return err
	}
	if errJson.Code != http.StatusNotAcceptable {
		return errJson
	}
	log.WithError(err).Warn("Envelope SSZ publish rejected, falling back to JSON")
	body, jerr := jsonFn()
	if jerr != nil {
		return errors.Wrap(jerr, "could not marshal envelope JSON for fallback")
	}
	return c.handler.Post(ctx, endpoint, nil, bytes.NewBuffer(body), nil)
}
