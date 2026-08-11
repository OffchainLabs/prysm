package enginev1_test

import (
	"bytes"
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/params"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func sweepThresholdRequest(threshold uint64) *enginev1.SetSweepThresholdRequest {
	return &enginev1.SetSweepThresholdRequest{
		SourceAddress:   bytes.Repeat([]byte{0x11}, 20),
		ValidatorPubkey: bytes.Repeat([]byte{0x22}, 48),
		Threshold:       threshold,
	}
}

// TestSetSweepThresholdRequest_EncodeDecodeRoundTrip checks the EIP-7685 flat list encoding
// for request type 0x05.
func TestSetSweepThresholdRequest_EncodeDecodeRoundTrip(t *testing.T) {
	requests := &enginev1.ExecutionRequestsGloas{
		SweepThresholds: []*enginev1.SetSweepThresholdRequest{
			sweepThresholdRequest(64_000_000_000),
			sweepThresholdRequest(2048_000_000_000),
		},
	}

	encoded, err := enginev1.EncodeExecutionRequestsGloas(requests)
	require.NoError(t, err)
	require.Equal(t, 1, len(encoded))
	// SWEEP_THRESHOLD_REQUEST_TYPE is 0x05, after the two builder request types.
	require.Equal(t, uint8(0x05), encoded[0][0])
	require.Equal(t, uint8(enginev1.SetSweepThresholdRequestType), encoded[0][0])

	raw := make([][]byte, len(encoded))
	for i, e := range encoded {
		raw[i] = e
	}
	bundle := &enginev1.ExecutionBundleGloas{ExecutionRequests: raw}
	decoded, err := bundle.GetDecodedExecutionRequests(params.BeaconConfig().ExecutionRequestLimits())
	require.NoError(t, err)
	require.DeepEqual(t, requests.SweepThresholds, decoded.SweepThresholds)
}

// TestSetSweepThresholdRequest_DecodeOverLimit rejects a request list longer than
// MAX_SET_SWEEP_THRESHOLD_REQUESTS_PER_PAYLOAD.
func TestSetSweepThresholdRequest_DecodeOverLimit(t *testing.T) {
	limit := params.BeaconConfig().MaxSetSweepThresholdRequestsPerPayload
	requests := &enginev1.ExecutionRequestsGloas{}
	for range limit + 1 {
		requests.SweepThresholds = append(requests.SweepThresholds, sweepThresholdRequest(64_000_000_000))
	}

	encoded, err := enginev1.EncodeExecutionRequestsGloas(requests)
	require.NoError(t, err)

	raw := make([][]byte, len(encoded))
	for i, e := range encoded {
		raw[i] = e
	}
	bundle := &enginev1.ExecutionBundleGloas{ExecutionRequests: raw}
	_, err = bundle.GetDecodedExecutionRequests(params.BeaconConfig().ExecutionRequestLimits())
	require.ErrorContains(t, "should not be more than the max per payload", err)
}

// TestExecutionRequestsGloas_TypeOrdering checks that sweep thresholds are emitted last, so
// the flat list stays in strictly ascending request-type order.
func TestExecutionRequestsGloas_TypeOrdering(t *testing.T) {
	requests := &enginev1.ExecutionRequestsGloas{
		Consolidations: []*enginev1.ConsolidationRequest{{
			SourceAddress: bytes.Repeat([]byte{0x33}, 20),
			SourcePubkey:  bytes.Repeat([]byte{0x44}, 48),
			TargetPubkey:  bytes.Repeat([]byte{0x55}, 48),
		}},
		BuilderExits: []*enginev1.BuilderExitRequest{{
			SourceAddress: bytes.Repeat([]byte{0x66}, 20),
			Pubkey:        bytes.Repeat([]byte{0x77}, 48),
		}},
		SweepThresholds: []*enginev1.SetSweepThresholdRequest{sweepThresholdRequest(64_000_000_000)},
	}

	encoded, err := enginev1.EncodeExecutionRequestsGloas(requests)
	require.NoError(t, err)
	require.Equal(t, 3, len(encoded))

	prev := encoded[0][0]
	for _, e := range encoded[1:] {
		require.Equal(t, true, e[0] > prev)
		prev = e[0]
	}
}

// TestFlattenRequests_HidesSweepThresholdsFromEngine checks that type 0x05 never reaches
// the execution client. The execution block header's requests_hash does not commit to
// mocked requests, so including them would make the engine recompute a different block
// hash and reject the payload as INVALID.
func TestFlattenRequests_HidesSweepThresholdsFromEngine(t *testing.T) {
	requests := &enginev1.ExecutionRequestsGloas{
		BuilderExits: []*enginev1.BuilderExitRequest{{
			SourceAddress: bytes.Repeat([]byte{0x66}, 20),
			Pubkey:        bytes.Repeat([]byte{0x77}, 48),
		}},
		SweepThresholds: []*enginev1.SetSweepThresholdRequest{sweepThresholdRequest(64_000_000_000)},
	}

	// The engine API list omits sweep thresholds...
	flattened, err := requests.FlattenRequests()
	require.NoError(t, err)
	for _, r := range flattened {
		require.NotEqual(t, uint8(enginev1.SetSweepThresholdRequestType), r[0])
	}
	require.Equal(t, 1, len(flattened))

	// ...while the EIP-7685 encoder itself stays faithful, so encode/decode round trips.
	encoded, err := enginev1.EncodeExecutionRequestsGloas(requests)
	require.NoError(t, err)
	require.Equal(t, 2, len(encoded))

	// A payload with no sweep thresholds is unaffected.
	plain := &enginev1.ExecutionRequestsGloas{BuilderExits: requests.BuilderExits}
	flattened, err = plain.FlattenRequests()
	require.NoError(t, err)
	require.Equal(t, 1, len(flattened))
}
