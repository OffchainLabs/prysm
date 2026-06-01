package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

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

// getExecutionPayloadEnvelope returns the envelope to sign for self-build. Stateless mode has the
// full envelope cached locally (from the v4 block fetch); stateful mode fetches the spec-wire
// blinded form from the BN, which exposes only the blinded envelope (beacon-APIs #580). Exactly one
// of the returned values is non-nil.
func (c *beaconApiValidatorClient) getExecutionPayloadEnvelope(
	ctx context.Context,
	slot primitives.Slot,
	beaconBlockRoot [32]byte,
) (*ethpb.ExecutionPayloadEnvelope, *ethpb.WireBlindedExecutionPayloadEnvelope, error) {
	if envelope, _, _ := c.envelopeCache.Peek(slot); envelope != nil {
		return envelope, nil, nil
	}

	endpoint := fmt.Sprintf("/eth/v1/validator/execution_payload_envelopes/%d/%s", slot, hexutil.Encode(beaconBlockRoot[:]))
	body, header, err := c.handler.GetSSZ(ctx, endpoint)
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not get blinded execution payload envelope")
	}
	if strings.Contains(header.Get("Content-Type"), api.OctetStreamMediaType) {
		blinded := &ethpb.WireBlindedExecutionPayloadEnvelope{}
		if err := blinded.UnmarshalSSZ(body); err != nil {
			return nil, nil, errors.Wrap(err, "could not unmarshal blinded envelope SSZ")
		}
		return nil, blinded, nil
	}
	var resp structs.GetValidatorBlindedExecutionPayloadEnvelopeResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, nil, errors.Wrap(err, "could not decode blinded envelope JSON")
	}
	if resp.Data == nil {
		return nil, nil, errors.New("blinded execution payload envelope data is nil")
	}
	blinded, err := resp.Data.ToConsensus()
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not convert blinded envelope")
	}
	return nil, blinded, nil
}

// publishExecutionPayloadEnvelope publishes the full envelope plus cached blobs/proofs as
// SignedExecutionPayloadEnvelopeContents (stateless flow). Stateful self-build uses
// publishBlindedExecutionPayloadEnvelope instead.
func (c *beaconApiValidatorClient) publishExecutionPayloadEnvelope(
	ctx context.Context,
	envelope *ethpb.SignedExecutionPayloadEnvelope,
) (*empty.Empty, error) {
	const endpoint = "/eth/v1/beacon/execution_payload_envelopes"
	if envelope == nil || envelope.Message == nil || envelope.Message.Payload == nil {
		return nil, errors.New("nil signed envelope or payload")
	}

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
	if err := c.postEnvelope(ctx, endpoint, envelopeHeaders(false), ssz, jsonFn); err != nil {
		return nil, errors.Wrap(err, "could not publish execution payload envelope contents")
	}
	return &empty.Empty{}, nil
}

// publishBlindedExecutionPayloadEnvelope publishes the signed blinded envelope (stateful flow); the
// BN reconstructs the full envelope from its cache. Signature is valid by HTR equivalence.
func (c *beaconApiValidatorClient) publishBlindedExecutionPayloadEnvelope(
	ctx context.Context,
	signed *ethpb.SignedWireBlindedExecutionPayloadEnvelope,
) (*empty.Empty, error) {
	const endpoint = "/eth/v1/beacon/execution_payload_envelopes"
	if signed == nil || signed.Message == nil {
		return nil, errors.New("nil signed blinded envelope")
	}
	ssz, err := signed.MarshalSSZ()
	if err != nil {
		return nil, errors.Wrap(err, "could not marshal blinded envelope SSZ")
	}
	jsonFn := func() ([]byte, error) {
		msg, jerr := structs.BlindedExecutionPayloadEnvelopeFromConsensus(signed.Message)
		if jerr != nil {
			return nil, jerr
		}
		j := &structs.SignedBlindedExecutionPayloadEnvelope{
			Message:   msg,
			Signature: hexutil.Encode(signed.Signature),
		}
		return json.Marshal(j)
	}
	if err := c.postEnvelope(ctx, endpoint, envelopeHeaders(true), ssz, jsonFn); err != nil {
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
