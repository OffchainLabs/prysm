package beacon_api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/validator/client/beacon-api/mock"
	testhelpers "github.com/OffchainLabs/prysm/v7/validator/client/beacon-api/test-helpers"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"go.uber.org/mock/gomock"
)

func TestPayloadAttestationData(t *testing.T) {
	ctx := t.Context()
	slot := uint64(42)
	beaconBlockRoot := testhelpers.FillByteSlice(32, 0xab)
	endpoint := fmt.Sprintf("/eth/v1/validator/payload_attestation_data/%d", slot)

	jsonHeader := http.Header{"Content-Type": []string{api.JsonMediaType}}
	sszHeader := http.Header{"Content-Type": []string{api.OctetStreamMediaType}}

	jsonResp, err := json.Marshal(structs.GetPayloadAttestationDataResponse{
		Version: version.String(version.Gloas),
		Data: &structs.PayloadAttestationData{
			BeaconBlockRoot:   hexutil.Encode(beaconBlockRoot),
			Slot:              fmt.Sprintf("%d", slot),
			PayloadPresent:    true,
			BlobDataAvailable: false,
		},
	})
	require.NoError(t, err)

	sszResp, err := (&ethpb.PayloadAttestationData{
		BeaconBlockRoot:   beaconBlockRoot,
		Slot:              primitives.Slot(slot),
		PayloadPresent:    true,
		BlobDataAvailable: false,
	}).MarshalSSZ()
	require.NoError(t, err)

	for _, tt := range []struct {
		name   string
		body   []byte
		header http.Header
	}{
		{name: "json response", body: jsonResp, header: jsonHeader},
		{name: "ssz response", body: sszResp, header: sszHeader},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			handler := mock.NewMockHandler(ctrl)
			handler.EXPECT().GetSSZ(gomock.Any(), endpoint).Return(tt.body, tt.header, nil).Times(1)

			client := &beaconApiValidatorClient{handler: handler}
			data, err := client.payloadAttestationData(ctx, primitives.Slot(slot))
			require.NoError(t, err)
			require.NotNil(t, data)
			assert.Equal(t, primitives.Slot(slot), data.Slot)
			assert.Equal(t, hexutil.Encode(beaconBlockRoot), hexutil.Encode(data.BeaconBlockRoot))
			assert.Equal(t, true, data.PayloadPresent)
			assert.Equal(t, false, data.BlobDataAvailable)
		})
	}
}

func TestPayloadAttestationData_NilData(t *testing.T) {
	ctx := t.Context()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	handler := mock.NewMockHandler(ctrl)

	jsonHeader := http.Header{"Content-Type": []string{api.JsonMediaType}}
	handler.EXPECT().GetSSZ(gomock.Any(), gomock.Any()).Return([]byte("{}"), jsonHeader, nil).Times(1)

	client := &beaconApiValidatorClient{handler: handler}
	_, err := client.payloadAttestationData(ctx, 1)
	require.ErrorContains(t, "payload attestation data is nil", err)
}

func TestPayloadAttestationData_EndpointError(t *testing.T) {
	ctx := t.Context()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	handler := mock.NewMockHandler(ctrl)

	handler.EXPECT().GetSSZ(gomock.Any(), gomock.Any()).Return(nil, nil, errors.New("boom")).Times(1)

	client := &beaconApiValidatorClient{handler: handler}
	_, err := client.payloadAttestationData(ctx, 1)
	require.ErrorContains(t, "boom", err)
}

func TestSubmitPayloadAttestation(t *testing.T) {
	msg := &ethpb.PayloadAttestationMessage{
		ValidatorIndex: 7,
		Data: &ethpb.PayloadAttestationData{
			BeaconBlockRoot:   testhelpers.FillByteSlice(32, 0x11),
			Slot:              99,
			PayloadPresent:    true,
			BlobDataAvailable: true,
		},
		Signature: testhelpers.FillByteSlice(96, 0x22),
	}
	sszBody, err := msg.MarshalSSZ()
	require.NoError(t, err)
	jsonBody, err := json.Marshal([]*structs.PayloadAttestationMessage{structs.PayloadAttestationMessageFromConsensus(msg)})
	require.NoError(t, err)
	headers := map[string]string{api.VersionHeader: version.String(version.Gloas)}

	t.Run("valid sends SSZ", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		handler := mock.NewMockHandler(ctrl)
		handler.EXPECT().PostSSZ(
			gomock.Any(),
			payloadAttestationsEndpoint,
			headers,
			bytes.NewBuffer(sszBody),
		).Return(nil, nil, nil).Times(1)

		client := &beaconApiValidatorClient{handler: handler}
		require.NoError(t, client.submitPayloadAttestation(t.Context(), msg))
	})

	t.Run("falls back to JSON on 415", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		handler := mock.NewMockHandler(ctrl)
		handler.EXPECT().PostSSZ(gomock.Any(), payloadAttestationsEndpoint, gomock.Any(), gomock.Any()).
			Return(nil, nil, &httputil.DefaultJsonError{Code: http.StatusUnsupportedMediaType, Message: "unsupported media type"}).Times(1)
		handler.EXPECT().Post(
			gomock.Any(),
			payloadAttestationsEndpoint,
			headers,
			bytes.NewBuffer(jsonBody),
			nil,
		).Return(nil).Times(1)

		client := &beaconApiValidatorClient{handler: handler}
		require.NoError(t, client.submitPayloadAttestation(t.Context(), msg))
	})

	t.Run("non-415 error does not fall back", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		handler := mock.NewMockHandler(ctrl)
		handler.EXPECT().PostSSZ(gomock.Any(), payloadAttestationsEndpoint, gomock.Any(), gomock.Any()).
			Return(nil, nil, errors.New("bad request")).Times(1)

		client := &beaconApiValidatorClient{handler: handler}
		require.ErrorContains(t, "bad request", client.submitPayloadAttestation(t.Context(), msg))
	})

	t.Run("nil message", func(t *testing.T) {
		client := &beaconApiValidatorClient{}
		require.ErrorContains(t, "payload attestation message is nil", client.submitPayloadAttestation(t.Context(), nil))
	})

	t.Run("nil data", func(t *testing.T) {
		client := &beaconApiValidatorClient{}
		require.ErrorContains(t, "payload attestation message is nil", client.submitPayloadAttestation(t.Context(), &ethpb.PayloadAttestationMessage{ValidatorIndex: 1}))
	})
}
