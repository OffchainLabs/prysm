package execution

import (
	"testing"

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
