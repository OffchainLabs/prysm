package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/prysmaticlabs/prysm/v5/network/httputil"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/validator/client/beacon-api/mock"
	testhelpers "github.com/prysmaticlabs/prysm/v5/validator/client/beacon-api/test-helpers"
	"go.uber.org/mock/gomock"
)

func TestProposeAttestation(t *testing.T) {
	attestation := &ethpb.Attestation{
		AggregationBits: testhelpers.FillByteSlice(4, 74),
		Data: &ethpb.AttestationData{
			Slot:            75,
			CommitteeIndex:  76,
			BeaconBlockRoot: testhelpers.FillByteSlice(32, 38),
			Source: &ethpb.Checkpoint{
				Epoch: 78,
				Root:  testhelpers.FillByteSlice(32, 79),
			},
			Target: &ethpb.Checkpoint{
				Epoch: 80,
				Root:  testhelpers.FillByteSlice(32, 81),
			},
		},
		Signature: testhelpers.FillByteSlice(96, 82),
	}

	tests := []struct {
		name                 string
		attestation          *ethpb.Attestation
		expectedErrorMessage string
		endpointError        error
		endpointCall         int
	}{
		{
			name:         "valid",
			attestation:  attestation,
			endpointCall: 1,
		},
		{
			name:                 "nil attestation",
			expectedErrorMessage: "attestation can't be nil",
		},
		{
			name: "nil attestation data",
			attestation: &ethpb.Attestation{
				AggregationBits: testhelpers.FillByteSlice(4, 74),
				Signature:       testhelpers.FillByteSlice(96, 82),
			},
			expectedErrorMessage: "attestation can't be nil",
		},
		{
			name: "nil source checkpoint",
			attestation: &ethpb.Attestation{
				AggregationBits: testhelpers.FillByteSlice(4, 74),
				Data: &ethpb.AttestationData{
					Target: &ethpb.Checkpoint{},
				},
				Signature: testhelpers.FillByteSlice(96, 82),
			},
			expectedErrorMessage: "attestation's source can't be nil",
		},
		{
			name: "nil target checkpoint",
			attestation: &ethpb.Attestation{
				AggregationBits: testhelpers.FillByteSlice(4, 74),
				Data: &ethpb.AttestationData{
					Source: &ethpb.Checkpoint{},
				},
				Signature: testhelpers.FillByteSlice(96, 82),
			},
			expectedErrorMessage: "attestation's target can't be nil",
		},
		{
			name: "nil aggregation bits",
			attestation: &ethpb.Attestation{
				Data: &ethpb.AttestationData{
					Source: &ethpb.Checkpoint{},
					Target: &ethpb.Checkpoint{},
				},
				Signature: testhelpers.FillByteSlice(96, 82),
			},
			expectedErrorMessage: "attestation's bitfield can't be nil",
		},
		{
			name: "nil signature",
			attestation: &ethpb.Attestation{
				AggregationBits: testhelpers.FillByteSlice(4, 74),
				Data: &ethpb.AttestationData{
					Source: &ethpb.Checkpoint{},
					Target: &ethpb.Checkpoint{},
				},
			},
			expectedErrorMessage: "attestation signature can't be nil",
		},
		{
			name:                 "bad request",
			attestation:          attestation,
			expectedErrorMessage: "bad request",
			endpointError:        errors.New("bad request"),
			endpointCall:         1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)

			var marshalledAttestations []byte
			if validateNilAttestation(test.attestation) == nil {
				b, err := json.Marshal(jsonifyAttestations([]*ethpb.Attestation{test.attestation}))
				require.NoError(t, err)
				marshalledAttestations = b
			}

			ctx := context.Background()

			headers := map[string]string{"Eth-Consensus-Version": version.String(test.attestation.Version())}
			jsonRestHandler.EXPECT().Post(
				gomock.Any(),
				"/eth/v2/beacon/pool/attestations",
				headers,
				bytes.NewBuffer(marshalledAttestations),
				nil,
			).Return(
				test.endpointError,
			).Times(test.endpointCall)

			validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
			proposeResponse, err := validatorClient.proposeAttestation(ctx, test.attestation)
			if test.expectedErrorMessage != "" {
				require.ErrorContains(t, test.expectedErrorMessage, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, proposeResponse)

			expectedAttestationDataRoot, err := attestation.Data.HashTreeRoot()
			require.NoError(t, err)

			// Make sure that the attestation data root is set
			assert.DeepEqual(t, expectedAttestationDataRoot[:], proposeResponse.AttestationDataRoot)
		})
	}
}

func TestProposeAttestationFallBack(t *testing.T) {
	attestation := &ethpb.Attestation{
		AggregationBits: testhelpers.FillByteSlice(4, 74),
		Data: &ethpb.AttestationData{
			Slot:            75,
			CommitteeIndex:  76,
			BeaconBlockRoot: testhelpers.FillByteSlice(32, 38),
			Source: &ethpb.Checkpoint{
				Epoch: 78,
				Root:  testhelpers.FillByteSlice(32, 79),
			},
			Target: &ethpb.Checkpoint{
				Epoch: 80,
				Root:  testhelpers.FillByteSlice(32, 81),
			},
		},
		Signature: testhelpers.FillByteSlice(96, 82),
	}

	ctrl := gomock.NewController(t)
	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)

	var marshalledAttestations []byte
	if validateNilAttestation(attestation) == nil {
		b, err := json.Marshal(jsonifyAttestations([]*ethpb.Attestation{attestation}))
		require.NoError(t, err)
		marshalledAttestations = b
	}

	ctx := context.Background()
	headers := map[string]string{"Eth-Consensus-Version": version.String(attestation.Version())}
	jsonRestHandler.EXPECT().Post(
		gomock.Any(),
		"/eth/v2/beacon/pool/attestations",
		headers,
		bytes.NewBuffer(marshalledAttestations),
		nil,
	).Return(
		&httputil.DefaultJsonError{
			Code: http.StatusNotFound,
		},
	).Times(1)

	jsonRestHandler.EXPECT().Post(
		gomock.Any(),
		"/eth/v1/beacon/pool/attestations",
		nil,
		bytes.NewBuffer(marshalledAttestations),
		nil,
	).Return(
		nil,
	).Times(1)

	validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	proposeResponse, err := validatorClient.proposeAttestation(ctx, attestation)

	require.NoError(t, err)
	require.NotNil(t, proposeResponse)

	expectedAttestationDataRoot, err := attestation.Data.HashTreeRoot()
	require.NoError(t, err)

	// Make sure that the attestation data root is set
	assert.DeepEqual(t, expectedAttestationDataRoot[:], proposeResponse.AttestationDataRoot)
}

func TestProposeAttestationElectra(t *testing.T) {
	attestation := &ethpb.AttestationElectra{
		AggregationBits: testhelpers.FillByteSlice(4, 74),
		Data: &ethpb.AttestationData{
			Slot:            75,
			CommitteeIndex:  76,
			BeaconBlockRoot: testhelpers.FillByteSlice(32, 38),
			Source: &ethpb.Checkpoint{
				Epoch: 78,
				Root:  testhelpers.FillByteSlice(32, 79),
			},
			Target: &ethpb.Checkpoint{
				Epoch: 80,
				Root:  testhelpers.FillByteSlice(32, 81),
			},
		},
		Signature:     testhelpers.FillByteSlice(96, 82),
		CommitteeBits: testhelpers.FillByteSlice(8, 83),
	}

	tests := []struct {
		name                 string
		attestation          *ethpb.AttestationElectra
		expectedErrorMessage string
		endpointError        error
		endpointCall         int
	}{
		{
			name:         "valid",
			attestation:  attestation,
			endpointCall: 1,
		},
		{
			name:                 "nil attestation",
			expectedErrorMessage: "attestation can't be nil",
		},
		{
			name: "nil attestation data",
			attestation: &ethpb.AttestationElectra{
				AggregationBits: testhelpers.FillByteSlice(4, 74),
				Signature:       testhelpers.FillByteSlice(96, 82),
				CommitteeBits:   testhelpers.FillByteSlice(8, 83),
			},
			expectedErrorMessage: "attestation can't be nil",
		},
		{
			name: "nil source checkpoint",
			attestation: &ethpb.AttestationElectra{
				AggregationBits: testhelpers.FillByteSlice(4, 74),
				Data: &ethpb.AttestationData{
					Target: &ethpb.Checkpoint{},
				},
				Signature:     testhelpers.FillByteSlice(96, 82),
				CommitteeBits: testhelpers.FillByteSlice(8, 83),
			},
			expectedErrorMessage: "attestation's source can't be nil",
		},
		{
			name: "nil target checkpoint",
			attestation: &ethpb.AttestationElectra{
				AggregationBits: testhelpers.FillByteSlice(4, 74),
				Data: &ethpb.AttestationData{
					Source: &ethpb.Checkpoint{},
				},
				Signature:     testhelpers.FillByteSlice(96, 82),
				CommitteeBits: testhelpers.FillByteSlice(8, 83),
			},
			expectedErrorMessage: "attestation's target can't be nil",
		},
		{
			name: "nil aggregation bits",
			attestation: &ethpb.AttestationElectra{
				Data: &ethpb.AttestationData{
					Source: &ethpb.Checkpoint{},
					Target: &ethpb.Checkpoint{},
				},
				Signature:     testhelpers.FillByteSlice(96, 82),
				CommitteeBits: testhelpers.FillByteSlice(8, 83),
			},
			expectedErrorMessage: "attestation's bitfield can't be nil",
		},
		{
			name: "nil signature",
			attestation: &ethpb.AttestationElectra{
				AggregationBits: testhelpers.FillByteSlice(4, 74),
				Data: &ethpb.AttestationData{
					Source: &ethpb.Checkpoint{},
					Target: &ethpb.Checkpoint{},
				},
				CommitteeBits: testhelpers.FillByteSlice(8, 83),
			},
			expectedErrorMessage: "attestation signature can't be nil",
		},
		{
			name: "nil committee bits",
			attestation: &ethpb.AttestationElectra{
				AggregationBits: testhelpers.FillByteSlice(4, 74),
				Data: &ethpb.AttestationData{
					Slot:            75,
					CommitteeIndex:  76,
					BeaconBlockRoot: testhelpers.FillByteSlice(32, 38),
					Source: &ethpb.Checkpoint{
						Epoch: 78,
						Root:  testhelpers.FillByteSlice(32, 79),
					},
					Target: &ethpb.Checkpoint{
						Epoch: 80,
						Root:  testhelpers.FillByteSlice(32, 81),
					},
				},
				Signature: testhelpers.FillByteSlice(96, 82),
			},
			expectedErrorMessage: "attestation committee bits can't be nil",
		},
		{
			name:                 "bad request",
			attestation:          attestation,
			expectedErrorMessage: "bad request",
			endpointError:        errors.New("bad request"),
			endpointCall:         1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)

			var marshalledAttestations []byte
			if validateNilAttestation(test.attestation) == nil {
				b, err := json.Marshal(jsonifyAttestationsElectra([]*ethpb.AttestationElectra{test.attestation}))
				require.NoError(t, err)
				marshalledAttestations = b
			}

			ctx := context.Background()
			headers := map[string]string{"Eth-Consensus-Version": version.String(test.attestation.Version())}
			jsonRestHandler.EXPECT().Post(
				gomock.Any(),
				"/eth/v2/beacon/pool/attestations",
				headers,
				bytes.NewBuffer(marshalledAttestations),
				nil,
			).Return(
				test.endpointError,
			).Times(test.endpointCall)

			validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
			proposeResponse, err := validatorClient.proposeAttestationElectra(ctx, test.attestation)
			if test.expectedErrorMessage != "" {
				require.ErrorContains(t, test.expectedErrorMessage, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, proposeResponse)

			expectedAttestationDataRoot, err := attestation.Data.HashTreeRoot()
			require.NoError(t, err)

			// Make sure that the attestation data root is set
			assert.DeepEqual(t, expectedAttestationDataRoot[:], proposeResponse.AttestationDataRoot)
		})
	}
}
