package light_client

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestProgressiveExecutionPayloadSSZEnabled(t *testing.T) {
	gloasPayload, err := blocks.WrappedExecutionPayloadGloas(&enginev1.ExecutionPayloadGloas{})
	require.NoError(t, err)
	denebPayload, err := blocks.WrappedExecutionPayloadDeneb(&enginev1.ExecutionPayloadDeneb{})
	require.NoError(t, err)

	tests := []struct {
		want    bool
		enabled bool
		payload interfaces.ExecutionData
		name    string
	}{
		{name: "nil payload", enabled: true, want: false},
		{name: "feature disabled", payload: gloasPayload, want: false},
		{name: "non-Gloas payload", enabled: true, payload: denebPayload, want: false},
		{name: "Gloas payload", enabled: true, payload: gloasPayload, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reset := features.InitWithReset(&features.Flags{DisableProgressiveSSZ: !tt.enabled})
			defer reset()

			require.Equal(t, tt.want, progressiveExecutionPayloadSSZEnabled(tt.payload))
		})
	}
}
