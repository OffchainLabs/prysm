package enginehttp

import (
	"context"
	"errors"
	"io"
	"net/http"
	"slices"
	"testing"

	enginev2 "github.com/OffchainLabs/prysm/v7/proto/engine/v2"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// The tests below are scaffolds for the Phase 4 endpoint implementations. Each
// is skipped with a TODO until the matching Client method in engine.go is
// filled in; the body shows the intended call shape against the h2c test
// harness in client_test.go (testClient/stubSSZ). When implementing an
// endpoint, drop the t.Skip and assert method/path/headers + decoded response,
// mirroring TestSSZRequest_* in client_test.go.

func TestNewPayload(t *testing.T) {
	statusSSZ, err := (&enginev2.PayloadStatus{Status: enginev2.StatusByte(enginev2.PayloadStatusValid)}).MarshalSSZ()
	require.NoError(t, err)

	var gotMethod, gotPath, gotCT, gotAccept string
	var gotBody []byte
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotCT = r.Header.Get("Content-Type")
		gotAccept = r.Header.Get("Accept")
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", contentTypeSSZ)
		_, _ = w.Write(statusSSZ)
	})

	status, err := c.NewPayload(context.Background(), ForkAmsterdam, &stubSSZ{data: []byte("envelope")})
	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "/engine/v2/amsterdam/payloads", gotPath)
	assert.Equal(t, contentTypeSSZ, gotCT)
	assert.Equal(t, contentTypeSSZ, gotAccept)
	assert.DeepEqual(t, []byte("envelope"), gotBody)
	assert.Equal(t, enginev2.PayloadStatusValid, status.Enum())
}

func TestForkchoiceUpdated(t *testing.T) {
	payloadID := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	respSSZ, err := (&enginev2.ForkchoiceUpdateResponse{
		PayloadStatus: &enginev2.PayloadStatus{Status: enginev2.StatusByte(enginev2.PayloadStatusValid)},
		PayloadId:     enginev2.PresentBytes(payloadID),
	}).MarshalSSZ()
	require.NoError(t, err)

	var gotMethod, gotPath, gotCT string
	var gotBody []byte
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotCT = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", contentTypeSSZ)
		_, _ = w.Write(respSSZ)
	})

	resp, err := c.ForkchoiceUpdated(context.Background(), ForkAmsterdam, &stubSSZ{data: []byte("fcu")})
	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "/engine/v2/amsterdam/forkchoice", gotPath)
	assert.Equal(t, contentTypeSSZ, gotCT)
	assert.DeepEqual(t, []byte("fcu"), gotBody)
	assert.Equal(t, enginev2.PayloadStatusValid, resp.PayloadStatus.Enum())
	gotID, present := enginev2.OptionalBytes(resp.PayloadId)
	assert.Equal(t, true, present)
	assert.DeepEqual(t, payloadID, gotID)
}

func TestGetPayload(t *testing.T) {
	payloadID := [8]byte{0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef}
	respBytes := []byte("built-payload-ssz")

	var gotMethod, gotPath string
	var hadBody bool
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		hadBody = len(b) > 0
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(respBytes)
	})

	out := &stubSSZ{}
	err := c.GetPayload(context.Background(), ForkAmsterdam, payloadID, out)
	require.NoError(t, err)
	assert.Equal(t, http.MethodGet, gotMethod)
	assert.Equal(t, "/engine/v2/amsterdam/payloads/0x0123456789abcdef", gotPath) // hex-encoded opaque id
	assert.Equal(t, false, hadBody)
	assert.DeepEqual(t, respBytes, out.data)
}

func TestGetPayloadBodiesByHash(t *testing.T) {
	t.Skip("TODO(ssz-over-http): implement Client.GetPayloadBodiesByHash — POST /engine/v2/{fork}/bodies/hash")
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {})
	err := c.GetPayloadBodiesByHash(context.Background(), ForkAmsterdam, nil, &stubSSZ{})
	require.NoError(t, err)
}

func TestGetPayloadBodiesByRange(t *testing.T) {
	t.Skip("TODO(ssz-over-http): implement Client.GetPayloadBodiesByRange — GET /engine/v2/{fork}/bodies?from&count")
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {})
	err := c.GetPayloadBodiesByRange(context.Background(), ForkAmsterdam, 1, 2, &stubSSZ{})
	require.NoError(t, err)
}

func TestGetBlobs(t *testing.T) {
	respSSZ, err := (&enginev2.BlobsV1Response{}).MarshalSSZ()
	require.NoError(t, err)

	var gotMethod, gotPath, gotCT string
	var gotBody []byte
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotCT = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", contentTypeSSZ)
		_, _ = w.Write(respSSZ)
	})

	err = c.GetBlobs(context.Background(), 1, &stubSSZ{data: []byte("blobreq")}, &enginev2.BlobsV1Response{})
	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "/engine/v2/blobs/v1", gotPath) // version-scoped, not fork-scoped
	assert.Equal(t, contentTypeSSZ, gotCT)
	assert.DeepEqual(t, []byte("blobreq"), gotBody)
}

// A 204 No Content means the EL cannot serve the request (syncing / V2
// all-or-nothing miss); it surfaces as ErrNoContent.
func TestGetBlobs_NoContent(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	err := c.GetBlobs(context.Background(), 2, &stubSSZ{data: []byte("x")}, &enginev2.BlobsV2Response{})
	require.NotNil(t, err)
	require.Equal(t, true, errors.Is(err, ErrNoContent))
}

// capabilitiesBody mirrors docs/fixtures/ethrex-capabilities.json.
const capabilitiesBody = `{
  "supported_forks": ["paris","shanghai","cancun","prague","osaka","amsterdam"],
  "fork_scoped_endpoints": ["payloads","forkchoice","bodies"],
  "independently_versioned": {"blobs": ["v1","v2","v3","v4"]},
  "unscoped_endpoints": ["capabilities","identity"],
  "limits": {"blobs.max_versioned_hashes": 128, "bodies.max_count": 32, "payload.max_bytes": 268435456}
}`

func TestCapabilities(t *testing.T) {
	var gotPath, gotAccept string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAccept = r.Header.Get("Accept")
		w.Header().Set("Content-Type", contentTypeJSON)
		_, _ = w.Write([]byte(capabilitiesBody))
	})

	caps, err := c.Capabilities(context.Background())
	require.NoError(t, err)
	require.NotNil(t, caps)
	assert.Equal(t, "/engine/v2/capabilities", gotPath)
	assert.Equal(t, contentTypeJSON, gotAccept)
	assert.Equal(t, true, slices.Contains(caps.SupportedForks, "amsterdam"))
	assert.Equal(t, uint64(268435456), caps.Limits["payload.max_bytes"])
	require.Equal(t, 4, len(caps.IndependentlyVersioned["blobs"]))
}

// A 404 surfaces as an *Error; the connection-setup probe maps this to a
// JSON-RPC fallback (the EL has no engine v2 surface).
func TestCapabilities_NotFound(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	_, err := c.Capabilities(context.Background())
	require.NotNil(t, err)
	var apiErr *Error
	require.Equal(t, true, errors.As(err, &apiErr))
	assert.Equal(t, http.StatusNotFound, apiErr.Status)
}

// identityBody mirrors docs/fixtures/ethrex-identity.json.
const identityBody = `[{"code":"EX","name":"ethrex","version":"v15.0.0","commit":"30e847af"}]`

func TestIdentity(t *testing.T) {
	var gotPath, gotAccept, gotCV string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAccept = r.Header.Get("Accept")
		gotCV = r.Header.Get(clientVersionHeader)
		w.Header().Set("Content-Type", contentTypeJSON)
		_, _ = w.Write([]byte(identityBody))
	})

	versions, err := c.Identity(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, len(versions))
	assert.Equal(t, "/engine/v2/identity", gotPath)
	assert.Equal(t, contentTypeJSON, gotAccept)
	assert.Equal(t, "Prysm/test", gotCV) // CL identifies itself via the header
	assert.Equal(t, "EX", versions[0].Code)
	assert.Equal(t, "ethrex", versions[0].Name)
	assert.Equal(t, "v15.0.0", versions[0].Version)
	assert.Equal(t, "30e847af", versions[0].Commit)
}
