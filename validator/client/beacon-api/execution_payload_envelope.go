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
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/pkg/errors"
)

// getExecutionPayloadEnvelope returns the envelope to sign for self-build. Stateless mode has the
// full envelope cached locally (from the v4 block fetch); otherwise it is fetched from the BN,
// which keeps the blobs and KZG proofs for the stateful publish.
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

// publishExecutionPayloadEnvelope publishes the signed envelope. With locally cached blobs/proofs
// it posts SignedExecutionPayloadEnvelopeContents (stateless, Eth-Blob-Data-Included: true);
// otherwise it posts the bare signed envelope and the BN attaches its cached blobs and KZG proofs
// (stateful, Eth-Blob-Data-Included: false).
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
		if err := c.postEnvelope(ctx, endpoint, envelopeHeaders(false), ssz, jsonFn); err != nil {
			return nil, errors.Wrap(err, "could not publish execution payload envelope")
		}
		return &empty.Empty{}, nil
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
	if err := c.postEnvelope(ctx, endpoint, envelopeHeaders(true), ssz, jsonFn); err != nil {
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
