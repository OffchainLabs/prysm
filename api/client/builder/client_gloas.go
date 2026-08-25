package builder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/pkg/errors"
)

const postBeaconBlockPath = "/eth/v1/builder/beacon_block"

func executionPayloadBidPath(slot primitives.Slot, parentHash, parentRoot [32]byte, proposerPubkey [48]byte) string {
	return fmt.Sprintf("/eth/v1/builder/execution_payload_bid/%d/%#x/%#x/%#x", slot, parentHash, parentRoot, proposerPubkey)
}

func builderPreferencesPath(proposerPubkey [48]byte) string {
	return fmt.Sprintf("/eth/v1/builder/builder_preferences/%#x", proposerPubkey)
}

// contentTypeOpts sets the Content-Type and consensus-version headers for a request body.
func contentTypeOpts(contentType string, v int) reqOption {
	return func(r *http.Request) {
		r.Header.Set("Content-Type", contentType)
		r.Header.Set(api.VersionHeader, version.String(v))
	}
}

// GetExecutionPayloadBid requests an execution payload bid; returns nil on 204 (no bid).
// If the builder rejects the SSZ Accept header, it retries once requesting JSON.
func (c *Client) GetExecutionPayloadBid(
	ctx context.Context,
	slot primitives.Slot,
	parentHash, parentRoot [32]byte,
	proposerPubkey [48]byte,
	auth *ethpb.SignedRequestAuth,
) (*ethpb.SignedExecutionPayloadBid, error) {
	return sszFallback(c, func(ssz bool) (*ethpb.SignedExecutionPayloadBid, error) {
		return c.getExecutionPayloadBid(ctx, slot, parentHash, parentRoot, proposerPubkey, auth, ssz)
	})
}

func (c *Client) getExecutionPayloadBid(
	ctx context.Context,
	slot primitives.Slot,
	parentHash, parentRoot [32]byte,
	proposerPubkey [48]byte,
	auth *ethpb.SignedRequestAuth,
	ssz bool,
) (*ethpb.SignedExecutionPayloadBid, error) {
	if auth == nil {
		return nil, errors.Wrap(errMalformedRequest, "nil signed request auth")
	}
	accept := api.JsonMediaType
	if ssz {
		accept = api.OctetStreamMediaType
	}
	var body []byte
	var err error
	contentType := api.JsonMediaType
	if ssz {
		contentType = api.OctetStreamMediaType
		body, err = auth.MarshalSSZ()
		if err != nil {
			return nil, errors.Wrap(err, "could not ssz encode SignedRequestAuth")
		}
	} else {
		body, err = json.Marshal(structs.SignedRequestAuthFromConsensus(auth))
		if err != nil {
			return nil, errors.Wrap(err, "could not json encode SignedRequestAuth")
		}
	}
	opts := []reqOption{func(r *http.Request) {
		r.Header.Set("Accept", accept)
		r.Header.Set("Content-Type", contentType)
		r.Header.Set(api.VersionHeader, version.String(version.Gloas))
		r.Header.Set("Date-Milliseconds", strconv.FormatInt(time.Now().UnixMilli(), 10))
		timeout := c.hc.Timeout
		if d, ok := ctx.Deadline(); ok {
			if remaining := time.Until(d); timeout <= 0 || remaining < timeout {
				timeout = remaining
			}
		}
		if timeout > 0 {
			r.Header.Set("X-Timeout-Ms", strconv.FormatInt(timeout.Milliseconds(), 10))
		}
	}}

	path := executionPayloadBidPath(slot, parentHash, parentRoot, proposerPubkey)
	raw, status, header, err := c.doWithStatus(ctx, http.MethodPost, path, bytes.NewReader(body), []int{http.StatusOK, http.StatusNoContent}, opts...)
	if err != nil {
		return nil, errors.Wrap(err, "error requesting execution payload bid from builder")
	}
	if status == http.StatusNoContent {
		return nil, nil
	}
	respContentType := header.Get("Content-Type")
	switch {
	case strings.Contains(respContentType, api.JsonMediaType):
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
	case strings.Contains(respContentType, api.OctetStreamMediaType):
		bid := &ethpb.SignedExecutionPayloadBid{}
		if err := bid.UnmarshalSSZ(raw); err != nil {
			return nil, errors.Wrap(err, "could not ssz decode SignedExecutionPayloadBid")
		}
		return bid, nil
	default:
		return nil, errors.Errorf("builder returned status %d with unexpected Content-Type %q: %s", status, respContentType, bodySnippet(raw))
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
// If the builder rejects the SSZ request, it retries once using JSON.
func (c *Client) SubmitSignedBeaconBlock(ctx context.Context, sb interfaces.ReadOnlySignedBeaconBlock) error {
	if sb.Version() < version.Gloas {
		return errors.Errorf("SubmitSignedBeaconBlock requires Gloas or later, got %s", version.String(sb.Version()))
	}
	return c.sszFallbackErr(func(ssz bool) error {
		return c.submitSignedBeaconBlock(ctx, sb, ssz)
	})
}

func (c *Client) submitSignedBeaconBlock(ctx context.Context, sb interfaces.ReadOnlySignedBeaconBlock, ssz bool) error {
	var (
		body        []byte
		err         error
		contentType string
	)
	if ssz {
		contentType = api.OctetStreamMediaType
		body, err = sb.MarshalSSZ()
		if err != nil {
			return errors.Wrap(err, "could not ssz encode SignedBeaconBlock")
		}
	} else {
		contentType = api.JsonMediaType
		body, err = jsonSignedBeaconBlock(sb)
		if err != nil {
			return err
		}
	}
	if _, _, err := c.do(ctx, http.MethodPost, postBeaconBlockPath, bytes.NewReader(body), http.StatusAccepted, contentTypeOpts(contentType, sb.Version())); err != nil {
		return errors.Wrap(err, "error submitting signed beacon block to builder")
	}
	return nil
}

func jsonSignedBeaconBlock(sb interfaces.ReadOnlySignedBeaconBlock) ([]byte, error) {
	pb, err := sb.Proto()
	if err != nil {
		return nil, errors.Wrap(err, "could not get protobuf block")
	}
	gloasBlock, ok := pb.(*ethpb.SignedBeaconBlockGloas)
	if !ok {
		return nil, errors.Errorf("unexpected block type %T for builder json submission", pb)
	}
	jsonBlock, err := structs.SignedBeaconBlockGloasFromConsensus(gloasBlock)
	if err != nil {
		return nil, errors.Wrap(err, "could not convert block for json encoding")
	}
	return json.Marshal(jsonBlock)
}

// SubmitBuilderPreferences submits a proposer's per-builder preferences ahead of the bid request.
// If the builder rejects the SSZ request, it retries once using JSON.
func (c *Client) SubmitBuilderPreferences(ctx context.Context, proposerPubkey [48]byte, req *ethpb.BuilderPreferencesRequest) error {
	if req == nil {
		return errors.Wrap(errMalformedRequest, "nil builder preferences request")
	}
	return c.sszFallbackErr(func(ssz bool) error {
		return c.submitBuilderPreferences(ctx, proposerPubkey, req, ssz)
	})
}

func (c *Client) submitBuilderPreferences(ctx context.Context, proposerPubkey [48]byte, req *ethpb.BuilderPreferencesRequest, ssz bool) error {
	var (
		body        []byte
		err         error
		contentType string
	)
	if ssz {
		contentType = api.OctetStreamMediaType
		body, err = req.MarshalSSZ()
		if err != nil {
			return errors.Wrap(err, "could not ssz encode BuilderPreferencesRequest")
		}
	} else {
		contentType = api.JsonMediaType
		body, err = json.Marshal(structs.BuilderPreferencesRequestFromConsensus(req))
		if err != nil {
			return errors.Wrap(err, "could not json encode BuilderPreferencesRequest")
		}
	}
	if _, _, err := c.do(ctx, http.MethodPost, builderPreferencesPath(proposerPubkey), bytes.NewReader(body), http.StatusAccepted, contentTypeOpts(contentType, version.Gloas)); err != nil {
		return errors.Wrap(err, "error submitting builder preferences")
	}
	return nil
}
