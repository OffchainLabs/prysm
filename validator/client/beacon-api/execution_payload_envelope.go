package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/ethereum/go-ethereum/common/hexutil"
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
	// BN GET returns the blinded form, which can't be rebuilt into a full envelope client-side.
	return nil, errors.Errorf("execution payload envelope cache miss for slot %d: include_payload=true required for cross-VC envelope fetch", slot)
}

func (c *beaconApiValidatorClient) publishExecutionPayloadEnvelope(
	ctx context.Context,
	envelope *ethpb.SignedExecutionPayloadEnvelope,
) (*empty.Empty, error) {
	const endpoint = "/eth/v1/beacon/execution_payload_envelopes"
	if envelope == nil || envelope.Message == nil || envelope.Message.Payload == nil {
		return nil, errors.New("nil signed envelope or payload")
	}

	// Stateless mode requires the envelope cache (populated by the v4 block
	// fetch) to provide blobs+proofs; a miss means we would silently lose blob
	// broadcast against a stateless target BN, so error out.
	if c.stateless {
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
		headers := envelopeHeaders(false)
		if err := c.postEnvelope(ctx, endpoint, headers, ssz, jsonFn); err != nil {
			return nil, errors.Wrap(err, "could not publish execution payload envelope contents")
		}
		return &empty.Empty{}, nil
	}

	// Stateful single-BN: send blinded; BN reconstructs full from cache, sig valid by HTR equivalence.
	blinded, err := ethpb.SignedWireBlindedFromFull(envelope)
	if err != nil {
		return nil, errors.Wrap(err, "could not derive blinded envelope")
	}
	ssz, err := blinded.MarshalSSZ()
	if err != nil {
		return nil, errors.Wrap(err, "could not marshal blinded envelope SSZ")
	}
	jsonFn := func() ([]byte, error) {
		msg, jerr := structs.BlindedExecutionPayloadEnvelopeFromConsensus(blinded.Message)
		if jerr != nil {
			return nil, jerr
		}
		j := &structs.SignedBlindedExecutionPayloadEnvelope{
			Message:   msg,
			Signature: hexutil.Encode(blinded.Signature),
		}
		return json.Marshal(j)
	}
	headers := envelopeHeaders(true)
	if err := c.postEnvelope(ctx, endpoint, headers, ssz, jsonFn); err != nil {
		return nil, errors.Wrap(err, "could not publish blinded execution payload envelope")
	}
	return &empty.Empty{}, nil
}

func envelopeHeaders(blinded bool) map[string]string {
	return map[string]string{
		api.VersionHeader:                 version.String(version.Gloas),
		api.ExecutionPayloadBlindedHeader: strconv.FormatBool(blinded),
	}
}

// postEnvelope publishes SSZ first; on 406 Not Acceptable falls back to JSON.
func (c *beaconApiValidatorClient) postEnvelope(ctx context.Context, endpoint string, headers map[string]string, ssz []byte, jsonFn func() ([]byte, error)) error {
	_, _, err := c.handler.PostSSZ(ctx, endpoint, headers, bytes.NewBuffer(ssz))
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
	return c.handler.Post(ctx, endpoint, headers, bytes.NewBuffer(body), nil)
}
