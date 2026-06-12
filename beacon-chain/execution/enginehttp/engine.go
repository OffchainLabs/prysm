// This file holds the typed REST + SSZ Engine API v2 endpoint operations
// (ethereum/execution-apis#793) as methods on Client. Each maps one engine_*
// JSON-RPC method to its v2 endpoint, built on the generic SSZRequest/JSONGet
// primitives in client.go and the SSZ wire containers in proto/engine/v2.
//
// These methods are the transport's lower half. An engineTransport interface in
// beacon-chain/execution wraps them so the Service's EngineCaller/Reconstructor
// methods can select JSON-RPC vs SSZ-HTTP by the EnableEngineSSZHTTP feature
// flag; that wiring lives outside this package.

package enginehttp

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	enginev2 "github.com/OffchainLabs/prysm/v7/proto/engine/v2"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ssz "github.com/prysmaticlabs/fastssz"
)

// The {fork} URL segment of a fork-scoped v2 endpoint. The spec keys endpoints
// by the EL fork name, not the CL fork name;
// osaka/amsterdam are the current interop targets.
const (
	ForkOsaka     = "osaka"     // CL Fulu
	ForkAmsterdam = "amsterdam" // CL Gloas
)

// NewPayload submits an execution payload for validation/import.
// POST /engine/v2/{fork}/payloads (replaces engine_newPayloadV1..5). The fork's
// ExecutionPayloadEnvelope folds parent_beacon_block_root and execution_requests
// inside; expected_blob_versioned_hashes is removed (the EL recomputes it). The
// four validation outcomes all return HTTP 200 with a PayloadStatus body — the
// caller maps the uint8 status enum back to Prysm's sentinels.
func (c *Client) NewPayload(ctx context.Context, fork string, envelope ssz.Marshaler) (*enginev2.PayloadStatus, error) {
	status := &enginev2.PayloadStatus{}
	if err := c.SSZRequest(ctx, http.MethodPost, "/"+fork+"/payloads", nil, envelope, status); err != nil {
		return nil, err
	}
	return status, nil
}

// ForkchoiceUpdated updates fork choice and optionally starts a build.
// POST /engine/v2/{fork}/forkchoice (replaces engine_forkchoiceUpdatedV1..4).
// The fork's ForkchoiceUpdate carries the optional payload_attributes (and, for
// Gloas, an optional custody_columns bitvector). The response carries the
// payload_status plus an opaque server-assigned payload_id; the caller echoes
// that token verbatim and never recomputes it.
func (c *Client) ForkchoiceUpdated(ctx context.Context, fork string, update ssz.Marshaler) (*enginev2.ForkchoiceUpdateResponse, error) {
	resp := &enginev2.ForkchoiceUpdateResponse{}
	if err := c.SSZRequest(ctx, http.MethodPost, "/"+fork+"/forkchoice", nil, update, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetPayload retrieves a previously started build by its opaque id.
// GET /engine/v2/{fork}/payloads/{id} (replaces engine_getPayloadV1..6). The
// opaque payload id is hex-encoded (0x-prefixed Bytes8) into the path. The EL
// keeps optimising the build, so the response is never cacheable (it carries
// Cache-Control: no-store); each call returns the latest snapshot — Prysm does
// not cache it.
func (c *Client) GetPayload(ctx context.Context, fork string, payloadID [8]byte, out ssz.Unmarshaler) error {
	return c.SSZRequest(ctx, http.MethodGet, "/"+fork+"/payloads/"+hexutil.Encode(payloadID[:]), nil, nil, out)
}

// GetPayloadBodiesByHash fetches execution bodies by block hash.
// POST /engine/v2/{fork}/bodies/hash (replaces engine_getPayloadBodiesByHashV1/2).
// The {fork} selects both the response schema and the era of returned blocks;
// out-of-era or pruned blocks come back as a per-entry available=false. The
// caller decodes the fork's BodiesResponse into out.
func (c *Client) GetPayloadBodiesByHash(ctx context.Context, fork string, req *enginev2.BodiesByHashRequest, out ssz.Unmarshaler) error {
	return c.SSZRequest(ctx, http.MethodPost, "/"+fork+"/bodies/hash", nil, req, out)
}

// GetPayloadBodiesByRange fetches execution bodies by [from, from+count) range.
// GET /engine/v2/{fork}/bodies?from&count (replaces engine_getPayloadBodiesByRangeV1/2).
// The range travels in the query (no SSZ body); a range straddling a fork
// boundary needs one call per fork URL. The caller decodes the fork's
// BodiesResponse into out.
func (c *Client) GetPayloadBodiesByRange(ctx context.Context, fork string, from, count uint64, out ssz.Unmarshaler) error {
	query := url.Values{}
	query.Set("from", strconv.FormatUint(from, 10))
	query.Set("count", strconv.FormatUint(count, 10))
	return c.SSZRequest(ctx, http.MethodGet, "/"+fork+"/bodies", query, nil, out)
}

// GetBlobs fetches blobs-and-proofs from the EL blob pool.
// POST /engine/v2/blobs/v{version} (replaces engine_getBlobsV1..4). The blob
// endpoints are version-scoped, not fork-scoped. The caller decodes the
// matching response (BlobsV1Response for v1, BlobsV2Response for v2/v3). HTTP
// 204 surfaces as ErrNoContent and means "the EL cannot serve this request"
// (syncing, or a V2 all-or-nothing miss) — distinct from a per-entry
// available=false within a 200 body. The v4 cell-range request container is not
// yet defined in proto/engine/v2, so v4 is not wired here.
func (c *Client) GetBlobs(ctx context.Context, version int, req ssz.Marshaler, out ssz.Unmarshaler) error {
	return c.SSZRequest(ctx, http.MethodPost, fmt.Sprintf("/blobs/v%d", version), nil, req, out)
}

// Capabilities is the JSON body of GET /engine/v2/capabilities. Field shape
// matches the EL ground truth in docs/fixtures/*-capabilities.json.
type Capabilities struct {
	SupportedForks         []string            `json:"supported_forks"`
	ForkScopedEndpoints    []string            `json:"fork_scoped_endpoints"`
	IndependentlyVersioned map[string][]string `json:"independently_versioned"`
	UnscopedEndpoints      []string            `json:"unscoped_endpoints"`
	Limits                 map[string]uint64   `json:"limits"`
}

// Capabilities probes the EL's v2 capabilities (replaces
// engine_exchangeCapabilities). GET /engine/v2/capabilities (JSON). A 404
// (returned as an *Error) means the EL has no v2 surface and the caller should
// fall back to JSON-RPC for the connection's lifetime.
func (c *Client) Capabilities(ctx context.Context) (*Capabilities, error) {
	var caps Capabilities
	if err := c.JSONGet(ctx, "/capabilities", &caps); err != nil {
		return nil, err
	}
	return &caps, nil
}

// Identity fetches the EL client versions (replaces engine_getClientVersionV1).
// GET /engine/v2/identity (JSON array). Prysm identifies itself via the
// X-Engine-Client-Version header sent on every request, so this is a one-way
// GET with no body — the mutual-exchange handshake is gone.
func (c *Client) Identity(ctx context.Context) ([]*structs.ClientVersionV1, error) {
	var versions []*structs.ClientVersionV1
	if err := c.JSONGet(ctx, "/identity", &versions); err != nil {
		return nil, err
	}
	return versions, nil
}
