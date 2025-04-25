package state_native_test

import (
	"fmt"
	"testing"

	state_native "github.com/OffchainLabs/prysm/v6/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
)

func TestSetLatestExecutionPayloadHeader(t *testing.T) {
	versionOffset := version.Bellatrix // PayloadHeader only applies in Bellatrix and beyond.
	payloads := []interfaces.ExecutionData{
		func() interfaces.ExecutionData {
			e := util.NewBeaconBlockBellatrix().Block.Body.ExecutionPayload
			ee, err := blocks.WrappedExecutionPayload(e)
			require.NoError(t, err)
			return ee
		}(),
		func() interfaces.ExecutionData {
			e := util.NewBeaconBlockCapella().Block.Body.ExecutionPayload
			ee, err := blocks.WrappedExecutionPayloadCapella(e)
			require.NoError(t, err)
			return ee
		}(),
		func() interfaces.ExecutionData {
			e := util.NewBeaconBlockDeneb().Block.Body.ExecutionPayload
			ee, err := blocks.WrappedExecutionPayloadDeneb(e)
			require.NoError(t, err)
			return ee
		}(),
		func() interfaces.ExecutionData {
			e := util.NewBeaconBlockElectra().Block.Body.ExecutionPayload
			ee, err := blocks.WrappedExecutionPayloadDeneb(e)
			require.NoError(t, err)
			return ee
		}(),
	}

	payloadHeaders := []interfaces.ExecutionData{
		func() interfaces.ExecutionData {
			e := util.NewBlindedBeaconBlockBellatrix().Block.Body.ExecutionPayloadHeader
			ee, err := blocks.WrappedExecutionPayloadHeader(e)
			require.NoError(t, err)
			return ee
		}(),
		func() interfaces.ExecutionData {
			e := util.NewBlindedBeaconBlockCapella().Block.Body.ExecutionPayloadHeader
			ee, err := blocks.WrappedExecutionPayloadHeaderCapella(e)
			require.NoError(t, err)
			return ee
		}(),
		func() interfaces.ExecutionData {
			e := util.NewBlindedBeaconBlockDeneb().Message.Body.ExecutionPayloadHeader
			ee, err := blocks.WrappedExecutionPayloadHeaderDeneb(e)
			require.NoError(t, err)
			return ee
		}(),
		func() interfaces.ExecutionData {
			e := util.NewBlindedBeaconBlockElectra().Message.Body.ExecutionPayloadHeader
			ee, err := blocks.WrappedExecutionPayloadHeaderDeneb(e)
			require.NoError(t, err)
			return ee
		}(),
	}

	t.Run("can set payload", func(t *testing.T) {
		for i, p := range payloads {
			t.Run(version.String(i+versionOffset), func(t *testing.T) {
				s := state_native.EmptyStateFromVersion(t, i+versionOffset)
				require.NoError(t, s.SetLatestExecutionPayloadHeader(p))
			})
		}
	})

	t.Run("can set payload header", func(t *testing.T) {
		for i, ph := range payloadHeaders {
			t.Run(version.String(i+versionOffset), func(t *testing.T) {
				s := state_native.EmptyStateFromVersion(t, i+versionOffset)
				require.NoError(t, s.SetLatestExecutionPayloadHeader(ph))
			})
		}
	})

	t.Run("mismatched type version returns error", func(t *testing.T) {
		require.Equal(t, len(payloads), len(payloadHeaders), "This test will fail if the payloads and payload headers are not same length")
		for i := 0; i < len(payloads); i++ {
			for j := 0; j < len(payloads); j++ {
				if i == j {
					continue
				}
				// Skip Deneb-Electra combinations
				if i == len(payloads)-1 && j == len(payloads)-2 {
					continue
				}
				if i == len(payloads)-2 && j == len(payloads)-1 {
					continue
				}
				t.Run(fmt.Sprintf("%s state with %s payload", version.String(i+versionOffset), version.String(j+versionOffset)), func(t *testing.T) {
					s := state_native.EmptyStateFromVersion(t, i+versionOffset)
					p := payloads[j]
					require.ErrorContains(t, "wrong state version", s.SetLatestExecutionPayloadHeader(p))
					ph := payloadHeaders[j]
					require.ErrorContains(t, "wrong state version", s.SetLatestExecutionPayloadHeader(ph))
				})
			}
		}
	})

}
