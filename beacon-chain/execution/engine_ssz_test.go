package execution

import (
	"errors"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/execution/enginehttp"
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
