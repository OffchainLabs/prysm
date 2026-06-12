package execution

import (
	"errors"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/execution/enginehttp"
	pb "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	enginev2 "github.com/OffchainLabs/prysm/v7/proto/engine/v2"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// payloadStatusResult must map the v2 PayloadStatus enum onto the same sentinels
// and latest-valid-hash returns as the JSON-RPC path (engine_jsonrpc.go).
func TestPayloadStatusResult(t *testing.T) {
	lvh := make([]byte, 32)
	lvh[0] = 0xab

	t.Run("VALID returns latest valid hash and no error", func(t *testing.T) {
		out, err := payloadStatusResult(&enginev2.PayloadStatus{
			Status:          enginev2.StatusByte(enginev2.PayloadStatusValid),
			LatestValidHash: enginev2.PresentBytes(lvh),
		})
		require.NoError(t, err)
		assert.DeepEqual(t, lvh, out)
	})

	t.Run("INVALID returns latest valid hash and the INVALID sentinel", func(t *testing.T) {
		out, err := payloadStatusResult(&enginev2.PayloadStatus{
			Status:          enginev2.StatusByte(enginev2.PayloadStatusInvalid),
			LatestValidHash: enginev2.PresentBytes(lvh),
		})
		require.ErrorIs(t, err, ErrInvalidPayloadStatus)
		assert.DeepEqual(t, lvh, out)
	})

	t.Run("SYNCING maps to the accepted/syncing sentinel", func(t *testing.T) {
		out, err := payloadStatusResult(&enginev2.PayloadStatus{Status: enginev2.StatusByte(enginev2.PayloadStatusSyncing)})
		require.ErrorIs(t, err, ErrAcceptedSyncingPayloadStatus)
		require.IsNil(t, out)
	})

	t.Run("ACCEPTED maps to the accepted/syncing sentinel", func(t *testing.T) {
		_, err := payloadStatusResult(&enginev2.PayloadStatus{Status: enginev2.StatusByte(enginev2.PayloadStatusAccepted)})
		require.ErrorIs(t, err, ErrAcceptedSyncingPayloadStatus)
	})

	t.Run("unknown status maps to the unknown sentinel", func(t *testing.T) {
		_, err := payloadStatusResult(&enginev2.PayloadStatus{Status: enginev2.StatusByte(9)})
		require.ErrorIs(t, err, ErrUnknownPayloadStatus)
	})
}

// forkchoiceResult must mirror jsonEngine.ForkchoiceUpdated's returns: the
// opaque payload id echoed verbatim, latest-valid-hash, and the same sentinels.
func TestForkchoiceResult(t *testing.T) {
	lvh := make([]byte, 32)
	lvh[0] = 0xcd
	id := []byte{1, 2, 3, 4, 5, 6, 7, 8}

	t.Run("VALID echoes the opaque payload id verbatim", func(t *testing.T) {
		gotID, gotLVH, err := forkchoiceResult(&enginev2.ForkchoiceUpdateResponse{
			PayloadStatus: &enginev2.PayloadStatus{Status: enginev2.StatusByte(enginev2.PayloadStatusValid), LatestValidHash: enginev2.PresentBytes(lvh)},
			PayloadId:     enginev2.PresentBytes(id),
		})
		require.NoError(t, err)
		require.NotNil(t, gotID)
		assert.DeepEqual(t, id, gotID[:])
		assert.DeepEqual(t, lvh, gotLVH)
	})

	t.Run("VALID with no build started has a nil payload id", func(t *testing.T) {
		gotID, _, err := forkchoiceResult(&enginev2.ForkchoiceUpdateResponse{
			PayloadStatus: &enginev2.PayloadStatus{Status: enginev2.StatusByte(enginev2.PayloadStatusValid)},
		})
		require.NoError(t, err)
		require.IsNil(t, gotID)
	})

	t.Run("INVALID returns latest valid hash and the INVALID sentinel", func(t *testing.T) {
		gotID, gotLVH, err := forkchoiceResult(&enginev2.ForkchoiceUpdateResponse{
			PayloadStatus: &enginev2.PayloadStatus{Status: enginev2.StatusByte(enginev2.PayloadStatusInvalid), LatestValidHash: enginev2.PresentBytes(lvh)},
		})
		require.ErrorIs(t, err, ErrInvalidPayloadStatus)
		require.IsNil(t, gotID)
		assert.DeepEqual(t, lvh, gotLVH)
	})

	t.Run("SYNCING maps to the accepted/syncing sentinel", func(t *testing.T) {
		_, _, err := forkchoiceResult(&enginev2.ForkchoiceUpdateResponse{
			PayloadStatus: &enginev2.PayloadStatus{Status: enginev2.StatusByte(enginev2.PayloadStatusSyncing)},
		})
		require.ErrorIs(t, err, ErrAcceptedSyncingPayloadStatus)
	})

	t.Run("nil payload status returns ErrNilResponse", func(t *testing.T) {
		_, _, err := forkchoiceResult(&enginev2.ForkchoiceUpdateResponse{})
		require.ErrorIs(t, err, ErrNilResponse)
	})

	t.Run("ACCEPTED on forkchoice is a protocol error", func(t *testing.T) {
		_, _, err := forkchoiceResult(&enginev2.ForkchoiceUpdateResponse{
			PayloadStatus: &enginev2.PayloadStatus{Status: enginev2.StatusByte(enginev2.PayloadStatusAccepted)},
		})
		require.ErrorIs(t, err, ErrUnknownPayloadStatus)
	})
}

// mapEngineError must translate RFC 7807 problem types into the JSON-RPC
// sentinels consumers branch on, and pass non-transport errors through.
func TestMapEngineError(t *testing.T) {
	cases := []struct {
		problemType string
		want        error
	}{
		{enginehttp.ProblemInvalidForkchoice, ErrInvalidForkchoiceState},
		{enginehttp.ProblemInvalidAttributes, ErrInvalidPayloadAttributes},
		{enginehttp.ProblemUnknownPayload, ErrUnknownPayload},
		{enginehttp.ProblemRequestTooLarge, ErrRequestTooLarge},
		{enginehttp.ProblemInvalidBody, ErrInvalidParams},
	}
	for _, tc := range cases {
		err := mapEngineError(&enginehttp.Error{Status: 409, Problem: enginehttp.Problem{Type: tc.problemType}})
		require.ErrorIs(t, err, tc.want)
	}

	require.NoError(t, mapEngineError(nil))

	ioErr := errors.New("io failure")
	assert.Equal(t, ioErr, mapEngineError(ioErr)) // non-*Error passes through

	other := &enginehttp.Error{Status: 500, Problem: enginehttp.Problem{Type: "/engine-api/errors/teapot"}}
	assert.Equal(t, other, mapEngineError(other)) // unmapped problem type passes through
}

// builtPayloadToBundle must copy a decoded v2 BuiltPayload onto the matching
// ExecutionBundle proto field-for-field so the JSON-RPC response builder applies.
func TestBuiltPayloadToBundle(t *testing.T) {
	val := []byte{0xaa, 0xbb}
	reqs := [][]byte{{0x01, 0x02}}

	t.Run("Fulu", func(t *testing.T) {
		bundle, err := builtPayloadToBundle(&enginev2.BuiltPayloadFulu{
			BlockValue:            val,
			ShouldOverrideBuilder: true,
			ExecutionRequests:     reqs,
		})
		require.NoError(t, err)
		fb, ok := bundle.(*pb.ExecutionBundleFulu)
		require.Equal(t, true, ok)
		assert.DeepEqual(t, val, fb.Value)
		assert.Equal(t, true, fb.ShouldOverrideBuilder)
		assert.DeepEqual(t, reqs, fb.ExecutionRequests)
	})

	t.Run("Gloas", func(t *testing.T) {
		bundle, err := builtPayloadToBundle(&enginev2.BuiltPayloadGloas{
			BlockValue:            val,
			ShouldOverrideBuilder: false,
			ExecutionRequests:     reqs,
		})
		require.NoError(t, err)
		gb, ok := bundle.(*pb.ExecutionBundleGloas)
		require.Equal(t, true, ok)
		assert.DeepEqual(t, val, gb.Value)
		assert.Equal(t, false, gb.ShouldOverrideBuilder)
		assert.DeepEqual(t, reqs, gb.ExecutionRequests)
	})

	t.Run("unexpected type errors", func(t *testing.T) {
		_, err := builtPayloadToBundle(&enginev2.PayloadStatus{})
		require.ErrorContains(t, "unexpected BuiltPayload type", err)
	})
}

// supportsBlob gates the blob endpoints on the probed v2 capability document,
// mirroring jsonEngine's caps.has check.
func TestSupportsBlob(t *testing.T) {
	e := &sszEngine{caps: &enginehttp.Capabilities{
		IndependentlyVersioned: map[string][]string{"blobs": {"v1", "v2", "v3", "v4"}},
	}}
	assert.Equal(t, true, e.supportsBlob("v1"))
	assert.Equal(t, true, e.supportsBlob("v2"))

	none := &sszEngine{caps: &enginehttp.Capabilities{IndependentlyVersioned: map[string][]string{}}}
	assert.Equal(t, false, none.supportsBlob("v1"))

	// No capability document (defensive): permit the request to surface support.
	assert.Equal(t, true, (&sszEngine{}).supportsBlob("v1"))
}

// bodiesEntries must be request-aligned, mapping available=false to a nil body
// (the reconstructor's missing marker) and dropping block_access_list.
func TestBodiesEntries(t *testing.T) {
	tx := []byte{0xde, 0xad}
	wd := []*pb.Withdrawal{{Index: 7}}
	resp := &enginev2.BodiesResponseGloas{
		Entries: []*enginev2.BodyEntryGloas{
			{Available: true, Body: &enginev2.ExecutionPayloadBodyGloas{
				Transactions:    [][]byte{tx},
				Withdrawals:     wd,
				BlockAccessList: []byte{0x01, 0x02},
			}},
			{Available: false, Body: &enginev2.ExecutionPayloadBodyGloas{}},
		},
	}

	out, err := bodiesEntries(resp)
	require.NoError(t, err)
	require.Equal(t, 2, len(out))
	require.NotNil(t, out[0])

	transactions, err := out[0].Transactions()
	require.NoError(t, err)
	assert.DeepEqual(t, tx, transactions[0])

	withdrawals, err := out[0].Withdrawals()
	require.NoError(t, err)
	assert.DeepEqual(t, wd, withdrawals)
	require.IsNil(t, out[1]) // available=false -> nil body

	_, err = bodiesEntries(&enginev2.PayloadStatus{})
	require.ErrorContains(t, "unexpected BodiesResponse type", err)
}
