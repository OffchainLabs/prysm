// This file scaffolds the typed REST + SSZ Engine API v2 endpoint operations
// (ethereum/execution-apis#793) as methods on Client. Each maps one engine_*
// JSON-RPC method to its v2 endpoint and is currently a stub returning
// errNotImplemented — filling the bodies is the Phase 4 work (one endpoint
// group per PR). They build on the generic SSZRequest/JSONGet primitives in
// client.go and the SSZ wire containers in proto/engine/v2.
//
// These methods are the transport's lower half. Phase 3 wraps them behind an
// engineTransport interface in beacon-chain/execution so the Service's
// EngineCaller/Reconstructor methods can select JSON-RPC vs SSZ-HTTP by the
// EnableEngineSSZHTTP feature flag; that wiring lives outside this package.

package enginehttp

import (
	"context"
	"net/http"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	enginev2 "github.com/OffchainLabs/prysm/v7/proto/engine/v2"
	"github.com/pkg/errors"
	ssz "github.com/prysmaticlabs/fastssz"
)

// The {fork} URL segment of a fork-scoped v2 endpoint. The spec keys endpoints
// by the EL fork name, not the CL fork name.
//
// TODO(ssz-over-http): add a Prysm-fork -> EL-fork-name resolver covering
// paris..amsterdam; these two are the current interop targets.
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
// GET /engine/v2/{fork}/payloads/{id} (replaces engine_getPayloadV1..6).
//
// TODO(ssz-over-http): hex-encode the opaque payload id into the path, GET via
// SSZRequest, decode the fork's BuiltPayload into out (e.g.
// enginev2.BuiltPayloadGloas). Honor Cache-Control: no-store (do not cache).
func (c *Client) GetPayload(ctx context.Context, fork string, payloadID [8]byte, out ssz.Unmarshaler) error {
	return errNotImplemented("GetPayload")
}

// GetPayloadBodiesByHash fetches execution bodies by block hash.
// POST /engine/v2/{fork}/bodies/hash (replaces engine_getPayloadBodiesByHashV1/2).
//
// TODO(ssz-over-http): POST req via SSZRequest, decode the fork's BodiesResponse
// into out; honor the per-entry available flag.
func (c *Client) GetPayloadBodiesByHash(ctx context.Context, fork string, req *enginev2.BodiesByHashRequest, out ssz.Unmarshaler) error {
	return errNotImplemented("GetPayloadBodiesByHash")
}

// GetPayloadBodiesByRange fetches execution bodies by [from, from+count) range.
// GET /engine/v2/{fork}/bodies?from&count (replaces engine_getPayloadBodiesByRangeV1/2).
//
// TODO(ssz-over-http): GET with from/count query params via SSZRequest, decode
// the fork's BodiesResponse into out. A cross-fork range needs multiple calls.
func (c *Client) GetPayloadBodiesByRange(ctx context.Context, fork string, from, count uint64, out ssz.Unmarshaler) error {
	return errNotImplemented("GetPayloadBodiesByRange")
}

// GetBlobs fetches blobs-and-proofs from the EL blob pool.
// POST /engine/v2/blobs/v{version} (replaces engine_getBlobsV1..4). The blob
// endpoints are version-scoped, not fork-scoped.
//
// TODO(ssz-over-http): build /blobs/v{version}, POST req via SSZRequest, decode
// the matching response (BlobsV1Response for v1, BlobsV2Response for v2/v3).
// 204 (ErrNoContent) means "cannot serve" vs a per-entry available=false. v4
// takes a bitvector cell-selection request whose container is not defined yet.
func (c *Client) GetBlobs(ctx context.Context, version int, req ssz.Marshaler, out ssz.Unmarshaler) error {
	return errNotImplemented("GetBlobs")
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

// errNotImplemented marks an unimplemented v2 endpoint operation (Phase 4).
func errNotImplemented(op string) error {
	return errors.Errorf("enginehttp: %s not implemented", op)
}
