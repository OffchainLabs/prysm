// Typed REST + SSZ Engine API endpoint operations.

package enginehttp

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	enginev2 "github.com/OffchainLabs/prysm/v7/proto/engine/v2"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
	ssz "github.com/prysmaticlabs/fastssz"
)

// NewPayload submits POST /engine/v1/payloads.
func (c *Client) NewPayload(ctx context.Context, v int, envelope ssz.Marshaler) (*enginev2.PayloadStatus, error) {
	fork, err := version.ELForkName(v)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get EL fork name")
	}
	status := &enginev2.PayloadStatus{}
	if err := c.ForkSSZRequest(ctx, http.MethodPost, fork, "/payloads", nil, envelope, status); err != nil {
		return nil, err
	}
	return status, nil
}

// ForkchoiceUpdated submits POST /engine/v1/forkchoice.
func (c *Client) ForkchoiceUpdated(ctx context.Context, v int, update ssz.Marshaler) (*enginev2.ForkchoiceUpdateResponse, error) {
	fork, err := version.ELForkName(v)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get EL fork name")
	}
	resp := &enginev2.ForkchoiceUpdateResponse{}
	if err := c.ForkSSZRequest(ctx, http.MethodPost, fork, "/forkchoice", nil, update, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetPayload submits GET /engine/v1/payloads/{id}.
func (c *Client) GetPayload(ctx context.Context, v int, payloadID [8]byte, out ssz.Unmarshaler) error {
	fork, err := version.ELForkName(v)
	if err != nil {
		return errors.Wrap(err, "failed to get EL fork name")
	}
	return c.ForkSSZRequest(ctx, http.MethodGet, fork, "/payloads/"+hexutil.Encode(payloadID[:]), nil, nil, out)
}

// GetPayloadBodiesByHash submits POST /engine/v1/bodies/hash.
func (c *Client) GetPayloadBodiesByHash(ctx context.Context, v int, req *enginev2.BodiesByHashRequest, out ssz.Unmarshaler) error {
	fork, err := version.ELForkName(v)
	if err != nil {
		return errors.Wrap(err, "failed to get EL fork name")
	}
	return c.ForkSSZRequest(ctx, http.MethodPost, fork, "/bodies/hash", nil, req, out)
}

// GetPayloadBodiesByRange submits GET /engine/v1/bodies?from&count.
func (c *Client) GetPayloadBodiesByRange(ctx context.Context, v int, from, count uint64, out ssz.Unmarshaler) error {
	fork, err := version.ELForkName(v)
	if err != nil {
		return errors.Wrap(err, "failed to get EL fork name")
	}
	query := url.Values{}
	query.Set("from", strconv.FormatUint(from, 10))
	query.Set("count", strconv.FormatUint(count, 10))
	return c.ForkSSZRequest(ctx, http.MethodGet, fork, "/bodies", query, nil, out)
}

// GetBlobs submits POST /engine/v1/blobs/v{version}. Blob endpoints do not use
// Eth-Execution-Version.
func (c *Client) GetBlobs(ctx context.Context, version int, req ssz.Marshaler, out ssz.Unmarshaler) error {
	return c.SSZRequest(ctx, http.MethodPost, fmt.Sprintf("/blobs/v%d", version), nil, req, out)
}

// Capabilities is the JSON body of GET /engine/v1/capabilities.
type Capabilities struct {
	SupportedForks         []string            `json:"supported_forks"`
	ForkScopedEndpoints    []string            `json:"fork_scoped_endpoints"`
	IndependentlyVersioned map[string][]string `json:"independently_versioned"`
	UnscopedEndpoints      []string            `json:"unscoped_endpoints"`
	Limits                 map[string]uint64   `json:"limits"`
}

// Capabilities probes GET /engine/v1/capabilities.
func (c *Client) Capabilities(ctx context.Context) (*Capabilities, error) {
	var caps Capabilities
	if err := c.JSONGet(ctx, "/capabilities", &caps); err != nil {
		return nil, err
	}
	return &caps, nil
}

// Identity fetches GET /engine/v1/identity.
func (c *Client) Identity(ctx context.Context) ([]*structs.ClientVersionV1, error) {
	var versions []*structs.ClientVersionV1
	if err := c.JSONGet(ctx, "/identity", &versions); err != nil {
		return nil, err
	}
	return versions, nil
}
