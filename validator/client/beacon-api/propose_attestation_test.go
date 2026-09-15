package beacon_api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/OffchainLabs/prysm/v7/network/httputil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/OffchainLabs/prysm/v7/validator/client/beacon-api/mock"
	testhelpers "github.com/OffchainLabs/prysm/v7/validator/client/beacon-api/test-helpers"
	"go.uber.org/mock/gomock"
)

func TestProposeAttestation(t *testing.T) {
	attestation1 := &ethpb.Attestation{
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
	attestation2 := &ethpb.Attestation{
		AggregationBits: testhelpers.FillByteSlice(4, 74),
		Data: &ethpb.AttestationData{
			Slot:            75,
			CommitteeIndex:  77,
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
		Signature: testhelpers.FillByteSlice(96, 83),
	}

	tests := []struct {
		name         string
		attestations []*ethpb.Attestation
		setupMock    func(handler *mock.MockHandler)
		expectedErr  string
	}{
		{
			name:         "single attestation",
			attestations: []*ethpb.Attestation{attestation1},
			setupMock: func(handler *mock.MockHandler) {
				marshalledAttestations, err := json.Marshal(jsonifyAttestations([]*ethpb.Attestation{attestation1}))
				require.NoError(t, err)
				headers := map[string]string{"Eth-Consensus-Version": version.String(attestation1.Version())}
				handler.EXPECT().Post(
					gomock.Any(),
					"/eth/v2/beacon/pool/attestations",
					headers,
					bytes.NewBuffer(marshalledAttestations),
					nil,
				).Return(nil).Times(1)
			},
		},
		{
			name:         "multiple attestations",
			attestations: []*ethpb.Attestation{attestation1, attestation2},
			setupMock: func(handler *mock.MockHandler) {
				marshalledAttestations, err := json.Marshal(jsonifyAttestations([]*ethpb.Attestation{attestation1, attestation2}))
				require.NoError(t, err)
				headers := map[string]string{"Eth-Consensus-Version": version.String(attestation1.Version())}
				handler.EXPECT().Post(
					gomock.Any(),
					"/eth/v2/beacon/pool/attestations",
					headers,
					bytes.NewBuffer(marshalledAttestations),
					nil,
				).Return(nil).Times(1)
			},
		},
		{
			name:         "no attestations",
			attestations: nil,
		},
		{
			name:         "bad request",
			attestations: []*ethpb.Attestation{attestation1},
			setupMock: func(handler *mock.MockHandler) {
				handler.EXPECT().Post(
					gomock.Any(),
					"/eth/v2/beacon/pool/attestations",
					gomock.Any(),
					gomock.Any(),
					nil,
				).Return(errors.New("bad request")).Times(1)
			},
			expectedErr: "bad request",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			handler := mock.NewMockHandler(ctrl)
			if tt.setupMock != nil {
				tt.setupMock(handler)
			}
			validatorClient := &beaconApiValidatorClient{handler: handler}
			resp, err := validatorClient.ProposeAttestation(t.Context(), tt.attestations)
			if tt.expectedErr != "" {
				require.ErrorContains(t, tt.expectedErr, err)
				return
			}
			require.NoError(t, err)
			if len(tt.attestations) == 0 {
				return
			}
			attestationDataRoot, err := tt.attestations[0].Data.HashTreeRoot()
			require.NoError(t, err)
			require.DeepEqual(t, attestationDataRoot[:], resp.AttestationDataRoot)
		})
	}
}

func TestProposeAttestation_InvalidInput(t *testing.T) {
	validAttestation := &ethpb.Attestation{
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
		name         string
		attestations []*ethpb.Attestation
		expectedErrs []string
	}{
		{
			name:         "nil attestation",
			attestations: []*ethpb.Attestation{validAttestation, nil},
			expectedErrs: []string{"attestation at index 1 is invalid", "attestation is nil"},
		},
		{
			name: "nil attestation data",
			attestations: []*ethpb.Attestation{validAttestation, {
				AggregationBits: testhelpers.FillByteSlice(4, 74),
				Signature:       testhelpers.FillByteSlice(96, 82),
			}},
			expectedErrs: []string{"attestation at index 1 is invalid", "attestation is nil"},
		},
		{
			name: "nil source checkpoint",
			attestations: []*ethpb.Attestation{validAttestation, {
				AggregationBits: testhelpers.FillByteSlice(4, 74),
				Data: &ethpb.AttestationData{
					Target: &ethpb.Checkpoint{},
				},
				Signature: testhelpers.FillByteSlice(96, 82),
			}},
			expectedErrs: []string{"attestation at index 1 is invalid", "attestation's source can't be nil"},
		},
		{
			name: "nil target checkpoint",
			attestations: []*ethpb.Attestation{validAttestation, {
				AggregationBits: testhelpers.FillByteSlice(4, 74),
				Data: &ethpb.AttestationData{
					Source: &ethpb.Checkpoint{},
				},
				Signature: testhelpers.FillByteSlice(96, 82),
			}},
			expectedErrs: []string{"attestation at index 1 is invalid", "attestation's target can't be nil"},
		},
		{
			name: "nil aggregation bits",
			attestations: []*ethpb.Attestation{validAttestation, {
				Data: &ethpb.AttestationData{
					Source: &ethpb.Checkpoint{},
					Target: &ethpb.Checkpoint{},
				},
				Signature: testhelpers.FillByteSlice(96, 82),
			}},
			expectedErrs: []string{"attestation at index 1 is invalid", "attestation's bitfield can't be nil"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			handler := mock.NewMockHandler(ctrl)
			validatorClient := &beaconApiValidatorClient{handler: handler}
			_, err := validatorClient.ProposeAttestation(t.Context(), tt.attestations)
			for _, expectedErr := range tt.expectedErrs {
				require.ErrorContains(t, expectedErr, err)
			}
		})
	}
}

func TestProposeAttestationElectra_Success(t *testing.T) {
	attestation1 := &ethpb.SingleAttestation{
		AttesterIndex: 1,
		Data: &ethpb.AttestationData{
			Slot:            75,
			CommitteeIndex:  0,
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
	attestation2 := &ethpb.SingleAttestation{
		AttesterIndex: 2,
		Data: &ethpb.AttestationData{
			Slot:            75,
			CommitteeIndex:  1,
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
		Signature: testhelpers.FillByteSlice(96, 83),
	}
	consensusVersion := version.String(slots.ToForkVersion(attestation1.Data.Slot))

	tests := []struct {
		name         string
		attestations []*ethpb.SingleAttestation
		setupMock    func(handler *mock.MockHandler)
		expectedErr  string
	}{
		{
			name:         "single attestation",
			attestations: []*ethpb.SingleAttestation{attestation1},
			setupMock: func(handler *mock.MockHandler) {
				var sszBuf bytes.Buffer
				sszBytes, err := attestation1.MarshalSSZ()
				require.NoError(t, err)
				sszBuf.Write(sszBytes)

				headers := map[string]string{"Eth-Consensus-Version": consensusVersion}
				expectPostSSZWithFallback(handler)
				handler.EXPECT().PostSSZ(
					gomock.Any(),
					"/eth/v2/beacon/pool/attestations",
					headers,
					bytes.NewBuffer(sszBuf.Bytes()),
				).Return(nil).Times(1)
			},
		},
		{
			name:         "multiple attestations",
			attestations: []*ethpb.SingleAttestation{attestation1, attestation2},
			setupMock: func(handler *mock.MockHandler) {
				var sszBuf bytes.Buffer
				for _, att := range []*ethpb.SingleAttestation{attestation1, attestation2} {
					sszBytes, err := att.MarshalSSZ()
					require.NoError(t, err)
					sszBuf.Write(sszBytes)
				}

				headers := map[string]string{"Eth-Consensus-Version": consensusVersion}
				expectPostSSZWithFallback(handler)
				handler.EXPECT().PostSSZ(
					gomock.Any(),
					"/eth/v2/beacon/pool/attestations",
					headers,
					bytes.NewBuffer(sszBuf.Bytes()),
				).Return(nil).Times(1)
			},
		},
		{
			name:         "no attestations",
			attestations: nil,
		},
		{
			name:         "unsupported media type falls back to JSON",
			attestations: []*ethpb.SingleAttestation{attestation1, attestation2},
			setupMock: func(handler *mock.MockHandler) {
				headers := map[string]string{"Eth-Consensus-Version": consensusVersion}
				expectPostSSZWithFallback(handler)
				handler.EXPECT().PostSSZ(
					gomock.Any(),
					"/eth/v2/beacon/pool/attestations",
					headers,
					gomock.Any(),
				).Return(&httputil.DefaultJsonError{Code: http.StatusUnsupportedMediaType}).Times(1)

				marshalledAttestations, err := json.Marshal(jsonifySingleAttestations([]*ethpb.SingleAttestation{attestation1, attestation2}))
				require.NoError(t, err)
				handler.EXPECT().Post(
					gomock.Any(),
					"/eth/v2/beacon/pool/attestations",
					headers,
					bytes.NewBuffer(marshalledAttestations),
					nil,
				).Return(nil).Times(1)
			},
		},
		{
			name:         "bad request",
			attestations: []*ethpb.SingleAttestation{attestation1},
			setupMock: func(handler *mock.MockHandler) {
				expectPostSSZWithFallback(handler)
				handler.EXPECT().PostSSZ(
					gomock.Any(),
					"/eth/v2/beacon/pool/attestations",
					gomock.Any(),
					gomock.Any(),
				).Return(errors.New("bad request")).Times(1)
			},
			expectedErr: "bad request",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			handler := mock.NewMockHandler(ctrl)
			if tt.setupMock != nil {
				tt.setupMock(handler)
			}
			validatorClient := &beaconApiValidatorClient{handler: handler}
			_, err := validatorClient.ProposeAttestationElectra(t.Context(), tt.attestations)
			if tt.expectedErr != "" {
				require.ErrorContains(t, tt.expectedErr, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestProposeAttestationElectra_InvalidInput(t *testing.T) {
	validAttestation := &ethpb.SingleAttestation{
		AttesterIndex: 1,
		Data: &ethpb.AttestationData{
			Slot:            75,
			CommitteeIndex:  0,
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
		name         string
		attestations []*ethpb.SingleAttestation
		expectedErrs []string
	}{
		{
			name:         "nil attestation",
			attestations: []*ethpb.SingleAttestation{validAttestation, nil},
			expectedErrs: []string{"attestation at index 1 is invalid", "attestation is nil"},
		},
		{
			name: "nil attestation data",
			attestations: []*ethpb.SingleAttestation{validAttestation, {
				AttesterIndex: 74,
				Signature:     testhelpers.FillByteSlice(96, 82),
				CommitteeId:   83,
			}},
			expectedErrs: []string{"attestation at index 1 is invalid", "attestation is nil"},
		},
		{
			name: "nil source checkpoint",
			attestations: []*ethpb.SingleAttestation{validAttestation, {
				AttesterIndex: 74,
				Data: &ethpb.AttestationData{
					Target: &ethpb.Checkpoint{},
				},
				Signature:   testhelpers.FillByteSlice(96, 82),
				CommitteeId: 83,
			}},
			expectedErrs: []string{"attestation at index 1 is invalid", "attestation's source can't be nil"},
		},
		{
			name: "nil target checkpoint",
			attestations: []*ethpb.SingleAttestation{validAttestation, {
				AttesterIndex: 74,
				Data: &ethpb.AttestationData{
					Source: &ethpb.Checkpoint{},
				},
				Signature:   testhelpers.FillByteSlice(96, 82),
				CommitteeId: 83,
			}},
			expectedErrs: []string{"attestation at index 1 is invalid", "attestation's target can't be nil"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			handler := mock.NewMockHandler(ctrl)
			validatorClient := &beaconApiValidatorClient{handler: handler}
			_, err := validatorClient.ProposeAttestationElectra(t.Context(), tt.attestations)
			for _, expectedErr := range tt.expectedErrs {
				require.ErrorContains(t, expectedErr, err)
			}
		})
	}
}
