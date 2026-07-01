package builder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
)

const postBeaconBlockPath = "/eth/v1/builder/beacon_block"

func executionPayloadBidPath(slot primitives.Slot, parentHash, parentRoot [32]byte, proposerPubkey [48]byte) string {
	return fmt.Sprintf("/eth/v1/builder/execution_payload_bid/%d/%#x/%#x/%#x", slot, parentHash, parentRoot, proposerPubkey)
}

func builderPreferencesPath(validatorPubkey [48]byte) string {
	return fmt.Sprintf("/eth/v1/builder/builder_preferences/%#x", validatorPubkey)
}

// contentTypeOpts sets the Content-Type and consensus-version headers for a request body.
func contentTypeOpts(contentType string, v int) reqOption {
	return func(r *http.Request) {
		r.Header.Set("Content-Type", contentType)
		r.Header.Set(api.VersionHeader, version.String(v))
	}
}

// marshalRequestAuthJSON encodes a SignedRequestAuthV1 as the builder-spec JSON
// the builder expects: message.data is the hex builder_url, slot is decimal, signature is hex.
func marshalRequestAuthJSON(auth *ethpb.SignedRequestAuthV1) ([]byte, error) {
	type message struct {
		Data string `json:"data"`
		Slot string `json:"slot"`
	}
	return json.Marshal(&struct {
		Message   message `json:"message"`
		Signature string  `json:"signature"`
	}{
		Message: message{
			Data: hexutil.Encode(auth.GetMessage().GetData()),
			Slot: fmt.Sprintf("%d", auth.GetMessage().GetSlot()),
		},
		Signature: hexutil.Encode(auth.GetSignature()),
	})
}

// GetExecutionPayloadBid requests an execution payload bid; returns nil on 204 (no bid).
func (c *Client) GetExecutionPayloadBid(
	ctx context.Context,
	slot primitives.Slot,
	parentHash, parentRoot [32]byte,
	proposerPubkey [48]byte,
	auth *ethpb.SignedRequestAuthV1,
) (*ethpb.SignedExecutionPayloadBid, error) {
	var body []byte
	opts := []reqOption{func(r *http.Request) {
		r.Header.Set("Accept", api.OctetStreamMediaType)
		r.Header.Set(api.VersionHeader, version.String(version.Gloas))
	}}
	if auth != nil {
		var err error
		body, err = marshalRequestAuthJSON(auth)
		if err != nil {
			return nil, errors.Wrap(err, "could not json encode SignedRequestAuthV1")
		}
		opts = append(opts, func(r *http.Request) {
			r.Header.Set("Content-Type", api.JsonMediaType)
		})
	}

	path := executionPayloadBidPath(slot, parentHash, parentRoot, proposerPubkey)
	raw, status, header, err := c.doWithStatus(ctx, http.MethodPost, path, bytes.NewReader(body), []int{http.StatusOK, http.StatusNoContent}, opts...)
	if err != nil {
		return nil, errors.Wrap(err, "error requesting execution payload bid from builder")
	}
	if status == http.StatusNoContent {
		return nil, nil
	}
	contentType := header.Get("Content-Type")
	switch {
	case strings.Contains(contentType, api.JsonMediaType):
		resp := &struct {
			Data *structs.SignedExecutionPayloadBid `json:"data"`
		}{}
		if err := json.Unmarshal(raw, resp); err != nil {
			return nil, errors.Wrap(err, "could not json decode SignedExecutionPayloadBid")
		}
		if resp.Data == nil {
			return nil, errors.New("nil data in json SignedExecutionPayloadBid response")
		}
		return resp.Data.ToConsensus()
	case strings.Contains(contentType, api.OctetStreamMediaType):
		bid := &ethpb.SignedExecutionPayloadBid{}
		if err := bid.UnmarshalSSZ(raw); err != nil {
			return nil, errors.Wrap(err, "could not ssz decode SignedExecutionPayloadBid")
		}
		return bid, nil
	default:
		return nil, errors.Errorf("builder returned status %d with unexpected Content-Type %q: %s", status, contentType, bodySnippet(raw))
	}
}

// bodySnippet collapses a response body to a single-line preview for error messages.
func bodySnippet(b []byte) string {
	s := strings.Join(strings.Fields(string(b)), " ")
	if len(s) > 256 {
		return s[:256] + "..."
	}
	return s
}

// SubmitSignedBeaconBlock sends the signed block to the builder so it can reveal the envelope.
func (c *Client) SubmitSignedBeaconBlock(ctx context.Context, sb interfaces.ReadOnlySignedBeaconBlock) error {
	if sb.Version() < version.Gloas {
		return errors.Errorf("submitSignedBeaconBlock requires Gloas or later, got %s", version.String(sb.Version()))
	}
	body, err := sb.MarshalSSZ()
	if err != nil {
		return errors.Wrap(err, "could not ssz encode SignedBeaconBlock")
	}
	_, status, _, err := c.doWithStatus(ctx, http.MethodPost, postBeaconBlockPath, bytes.NewReader(body), []int{http.StatusAccepted, http.StatusUnsupportedMediaType}, contentTypeOpts(api.OctetStreamMediaType, sb.Version()))
	if err != nil {
		return errors.Wrap(err, "error submitting signed beacon block to builder")
	}
	if status != http.StatusUnsupportedMediaType {
		return nil
	}
	// Builder does not accept SSZ; retry as JSON.
	pb, err := sb.Proto()
	if err != nil {
		return errors.Wrap(err, "could not get protobuf block")
	}
	gloasBlock, ok := pb.(*ethpb.SignedBeaconBlockGloas)
	if !ok {
		return errors.Errorf("unexpected block type %T for builder json submission", pb)
	}
	jsonBlock, err := structs.SignedBeaconBlockGloasFromConsensus(gloasBlock)
	if err != nil {
		return errors.Wrap(err, "could not convert block for json encoding")
	}
	jsonBody, err := json.Marshal(jsonBlock)
	if err != nil {
		return errors.Wrap(err, "could not json encode SignedBeaconBlock")
	}
	jsonOpts := func(r *http.Request) {
		r.Header.Set("Content-Type", api.JsonMediaType)
		r.Header.Set(api.VersionHeader, version.String(sb.Version()))
	}
	if _, _, err := c.do(ctx, http.MethodPost, postBeaconBlockPath, bytes.NewReader(jsonBody), http.StatusAccepted, jsonOpts); err != nil {
		return errors.Wrap(err, "error submitting json signed beacon block to builder")
	}
	return nil
}

// SubmitBuilderPreferences submits a proposer's per-builder preferences ahead of the bid request.
func (c *Client) SubmitBuilderPreferences(ctx context.Context, validatorPubkey [48]byte, req *ethpb.BuilderPreferencesRequestV1) error {
	if req == nil {
		return errors.Wrap(errMalformedRequest, "nil builder preferences request")
	}
	body, err := req.MarshalSSZ()
	if err != nil {
		return errors.Wrap(err, "could not ssz encode BuilderPreferencesRequestV1")
	}
	if _, _, err := c.do(ctx, http.MethodPost, builderPreferencesPath(validatorPubkey), bytes.NewReader(body), http.StatusAccepted, contentTypeOpts(api.OctetStreamMediaType, version.Gloas)); err != nil {
		return errors.Wrap(err, "error submitting builder preferences")
	}
	return nil
}
