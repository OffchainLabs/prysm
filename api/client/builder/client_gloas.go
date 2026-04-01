package builder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"text/template"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/pkg/errors"
)

const (
	postExecutionPayloadBidPath = "/eth/v1/builder/execution_payload_bid/{{.Slot}}/{{.ParentHash}}/{{.ParentRoot}}/{{.ProposerIndex}}"
	postBeaconBlockPath         = "/eth/v1/builder/beacon_block"
	postBuilderPreferencesPath  = "/eth/v1/builder/preferences"
)

var execPayloadBidTemplate = template.Must(template.New("").Parse(postExecutionPayloadBidPath))

func execPayloadBidPath(slot primitives.Slot, parentHash [32]byte, parentRoot [32]byte, proposerIndex primitives.ValidatorIndex) (string, error) {
	v := struct {
		Slot          primitives.Slot
		ParentHash    string
		ParentRoot    string
		ProposerIndex primitives.ValidatorIndex
	}{
		Slot:          slot,
		ParentHash:    fmt.Sprintf("%#x", parentHash),
		ParentRoot:    fmt.Sprintf("%#x", parentRoot),
		ProposerIndex: proposerIndex,
	}
	b := bytes.NewBuffer(nil)
	if err := execPayloadBidTemplate.Execute(b, v); err != nil {
		return "", errors.Wrapf(err, "error rendering exec payload bid path template")
	}
	return b.String(), nil
}

// GetExecutionPayloadBid requests a SignedExecutionPayloadBid from a builder
// for the given slot, parent hash, parent root, and proposer index. The
// proposer authenticates the request with a SignedRequestAuth.
func (c *Client) GetExecutionPayloadBid(
	ctx context.Context,
	slot primitives.Slot,
	parentHash [32]byte,
	parentRoot [32]byte,
	proposerIndex primitives.ValidatorIndex,
	auth *ethpb.SignedRequestAuth,
) (*ethpb.SignedExecutionPayloadBid, error) {
	ctx, span := trace.StartSpan(ctx, "builder.client.GetExecutionPayloadBid")
	defer span.End()

	if auth == nil {
		return nil, errors.New("signed request auth is required")
	}

	path, err := execPayloadBidPath(slot, parentHash, parentRoot, proposerIndex)
	if err != nil {
		return nil, err
	}

	var body []byte
	var postOpts reqOption
	if c.sszEnabled {
		body, err = auth.MarshalSSZ()
		if err != nil {
			return nil, errors.Wrap(err, "could not marshal SignedRequestAuth SSZ")
		}
		postOpts = func(r *http.Request) {
			r.Header.Set("Content-Type", api.OctetStreamMediaType)
			r.Header.Set("Accept", api.OctetStreamMediaType)
		}
	} else {
		body, err = json.Marshal(auth)
		if err != nil {
			return nil, errors.Wrap(err, "could not marshal SignedRequestAuth JSON")
		}
		postOpts = func(r *http.Request) {
			r.Header.Set("Content-Type", api.JsonMediaType)
			r.Header.Set("Accept", api.JsonMediaType)
		}
	}

	data, _, err := c.do(ctx, http.MethodPost, path, bytes.NewBuffer(body), http.StatusOK, postOpts)
	if err != nil {
		return nil, errors.Wrap(err, "error getting execution payload bid from builder")
	}

	bid := &ethpb.SignedExecutionPayloadBid{}
	if c.sszEnabled {
		if err := bid.UnmarshalSSZ(data); err != nil {
			return nil, errors.Wrap(err, "could not unmarshal SignedExecutionPayloadBid SSZ")
		}
	} else {
		if err := json.Unmarshal(data, bid); err != nil {
			return nil, errors.Wrap(err, "could not unmarshal SignedExecutionPayloadBid JSON")
		}
	}
	return bid, nil
}

// SubmitSignedBeaconBlock sends the full SignedBeaconBlock to the builder,
// committing the proposer to the embedded execution payload bid. The builder
// then constructs and broadcasts the execution payload envelope.
func (c *Client) SubmitSignedBeaconBlock(ctx context.Context, sb interfaces.ReadOnlySignedBeaconBlock) error {
	ctx, span := trace.StartSpan(ctx, "builder.client.SubmitSignedBeaconBlock")
	defer span.End()

	var body []byte
	var postOpts reqOption
	var err error

	if c.sszEnabled {
		body, err = sb.MarshalSSZ()
		if err != nil {
			return errors.Wrap(err, "could not marshal SignedBeaconBlock SSZ")
		}
		postOpts = func(r *http.Request) {
			r.Header.Set(api.VersionHeader, version.String(sb.Version()))
			r.Header.Set("Content-Type", api.OctetStreamMediaType)
		}
	} else {
		pb, err := sb.Proto()
		if err != nil {
			return errors.Wrap(err, "could not get proto for SignedBeaconBlock")
		}
		body, err = json.Marshal(pb)
		if err != nil {
			return errors.Wrap(err, "could not marshal SignedBeaconBlock JSON")
		}
		postOpts = func(r *http.Request) {
			r.Header.Set(api.VersionHeader, version.String(sb.Version()))
			r.Header.Set("Content-Type", api.JsonMediaType)
		}
	}

	_, _, err = c.do(ctx, http.MethodPost, postBeaconBlockPath, bytes.NewBuffer(body), http.StatusAccepted, postOpts)
	if err != nil {
		return errors.Wrap(err, "error submitting signed beacon block to builder")
	}
	return nil
}

// SubmitBuilderPreferences sends per-builder preferences (e.g. max_trusted_bid)
// to a specific builder. These are distinct from general ProposerPreferences
// which are broadcast via the gossip topic.
func (c *Client) SubmitBuilderPreferences(ctx context.Context, prefs []*ethpb.SignedBuilderPreferencesRPC) error {
	ctx, span := trace.StartSpan(ctx, "builder.client.SubmitBuilderPreferences")
	defer span.End()

	if len(prefs) == 0 {
		return errors.Wrap(errMalformedRequest, "empty builder preferences list")
	}

	var body []byte
	var postOpts reqOption
	var err error

	if c.sszEnabled {
		prefSize := prefs[0].SizeSSZ()
		ssz := make([]byte, 0, prefSize*len(prefs))
		for _, p := range prefs {
			b, err := p.MarshalSSZ()
			if err != nil {
				return errors.Wrap(err, "could not marshal SignedBuilderPreferencesRPC SSZ")
			}
			ssz = append(ssz, b...)
		}
		body = ssz
		postOpts = func(r *http.Request) {
			r.Header.Set("Content-Type", api.OctetStreamMediaType)
		}
	} else {
		body, err = json.Marshal(prefs)
		if err != nil {
			return errors.Wrap(err, "could not marshal SignedBuilderPreferencesRPC JSON")
		}
		postOpts = func(r *http.Request) {
			r.Header.Set("Content-Type", api.JsonMediaType)
		}
	}

	_, _, err = c.do(ctx, http.MethodPost, postBuilderPreferencesPath, bytes.NewBuffer(body), http.StatusOK, postOpts)
	if err != nil {
		return errors.Wrap(err, "error submitting builder preferences")
	}
	return nil
}
