package beacon_api

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/pkg/errors"
)

// getExecutionPayloadEnvelope returns the envelope to sign for self-build: the locally cached one
// from the v4 block fetch (stateless) if present, otherwise fetched from the BN (stateful).
func (c *beaconApiValidatorClient) getExecutionPayloadEnvelope(
	ctx context.Context,
	slot primitives.Slot,
	beaconBlockRoot [32]byte,
) (*ethpb.ExecutionPayloadEnvelope, error) {
	if envelope, _, _ := c.envelopeCache.Peek(slot); envelope != nil {
		if bytesutil.ToBytes32(envelope.BeaconBlockRoot) != beaconBlockRoot {
			return nil, errors.New("cached execution payload envelope beacon_block_root does not match requested block")
		}
		return envelope, nil
	}

	endpoint := fmt.Sprintf("/eth/v1/validator/execution_payload_envelopes/%d/%s", slot, hexutil.Encode(beaconBlockRoot[:]))
	body, header, err := c.handler.GetSSZ(ctx, endpoint)
	if err != nil {
		return nil, errors.Wrap(err, "could not get execution payload envelope")
	}
	if strings.Contains(header.Get("Content-Type"), api.OctetStreamMediaType) {
		envelope := &ethpb.ExecutionPayloadEnvelope{}
		if err := envelope.UnmarshalSSZ(body); err != nil {
			return nil, errors.Wrap(err, "could not unmarshal envelope SSZ")
		}
		return envelope, nil
	}
	var resp structs.GetValidatorExecutionPayloadEnvelopeResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, errors.Wrap(err, "could not decode envelope JSON")
	}
	if resp.Data == nil {
		return nil, errors.New("execution payload envelope data is nil")
	}
	envelope, err := resp.Data.ToConsensus()
	if err != nil {
		return nil, errors.Wrap(err, "could not convert envelope")
	}
	return envelope, nil
}

// publishExecutionPayloadEnvelope posts contents (stateless) when blobs/proofs are cached locally,
// otherwise the bare signed envelope (stateful; the BN attaches its cached blobs and proofs).
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
		jsonFn := func() ([]byte, error) {
			j, jerr := structs.SignedExecutionPayloadEnvelopeFromConsensus(envelope)
			if jerr != nil {
				return nil, jerr
			}
			return json.Marshal(j)
		}
		if err := c.handler.PostSSZWithFallback(ctx, endpoint, envelopeHeaders(false), envelope.MarshalSSZ, jsonFn); err != nil {
			return nil, errors.Wrap(err, "could not publish execution payload envelope")
		}
		return &empty.Empty{}, nil
	}

	contents := &ethpb.SignedExecutionPayloadEnvelopeContents{
		SignedExecutionPayloadEnvelope: envelope,
		KzgProofs:                      kzgProofs,
		Blobs:                          blobs,
	}
	jsonFn := func() ([]byte, error) {
		j, jerr := structs.SignedExecutionPayloadEnvelopeContentsFromConsensus(envelope, kzgProofs, blobs)
		if jerr != nil {
			return nil, jerr
		}
		return json.Marshal(j)
	}
	if err := c.handler.PostSSZWithFallback(ctx, endpoint, envelopeHeaders(true), contents.MarshalSSZ, jsonFn); err != nil {
		return nil, errors.Wrap(err, "could not publish execution payload envelope contents")
	}
	return &empty.Empty{}, nil
}

func envelopeHeaders(blobDataIncluded bool) map[string]string {
	return map[string]string{
		api.VersionHeader:          version.String(version.Gloas),
		api.BlobDataIncludedHeader: strconv.FormatBool(blobDataIncluded),
	}
}
