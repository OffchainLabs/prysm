package beacon_api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
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
	beaconBlockRoot := hexutil.Encode(testhelpers.FillByteSlice(32, 0xab))

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	handler := mock.NewMockJsonRestHandler(ctrl)

	resp := structs.GetPayloadAttestationDataResponse{}
	handler.EXPECT().Get(
		gomock.Any(),
		fmt.Sprintf("/eth/v1/validator/payload_attestation_data/%d", slot),
		&resp,
	).Return(nil).SetArg(2, structs.GetPayloadAttestationDataResponse{
		Version: version.String(version.Gloas),
		Data: &structs.PayloadAttestationData{
			BeaconBlockRoot:   beaconBlockRoot,
			Slot:              fmt.Sprintf("%d", slot),
			PayloadPresent:    true,
			BlobDataAvailable: false,
		},
	}).Times(1)

	client := &beaconApiValidatorClient{handler: handler}
	data, err := client.payloadAttestationData(ctx, primitives.Slot(slot))
	require.NoError(t, err)
	require.NotNil(t, data)
	assert.Equal(t, primitives.Slot(slot), data.Slot)
	assert.Equal(t, beaconBlockRoot, hexutil.Encode(data.BeaconBlockRoot))
	assert.Equal(t, true, data.PayloadPresent)
	assert.Equal(t, false, data.BlobDataAvailable)
}

func TestPayloadAttestationData_NilData(t *testing.T) {
	ctx := t.Context()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	handler := mock.NewMockJsonRestHandler(ctrl)

	handler.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)

	client := &beaconApiValidatorClient{handler: handler}
	_, err := client.payloadAttestationData(ctx, 1)
	require.ErrorContains(t, "payload attestation data is nil", err)
}

func TestPayloadAttestationData_EndpointError(t *testing.T) {
	ctx := t.Context()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	handler := mock.NewMockJsonRestHandler(ctrl)

	handler.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("boom")).Times(1)

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

	validBody, err := json.Marshal([]*structs.PayloadAttestationMessage{{
		ValidatorIndex: "7",
		Data: &structs.PayloadAttestationData{
			BeaconBlockRoot:   hexutil.Encode(testhelpers.FillByteSlice(32, 0x11)),
			Slot:              "99",
			PayloadPresent:    true,
			BlobDataAvailable: true,
		},
		Signature: hexutil.Encode(testhelpers.FillByteSlice(96, 0x22)),
	}})
	require.NoError(t, err)

	tests := []struct {
		name          string
		msg           *ethpb.PayloadAttestationMessage
		endpointError error
		endpointCall  int
		expectErr     string
	}{
		{name: "valid", msg: msg, endpointCall: 1},
		{name: "nil message", msg: nil, expectErr: "payload attestation message is nil"},
		{name: "nil data", msg: &ethpb.PayloadAttestationMessage{ValidatorIndex: 1}, expectErr: "payload attestation message is nil"},
		{name: "endpoint error", msg: msg, endpointError: errors.New("bad request"), endpointCall: 1, expectErr: "bad request"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			handler := mock.NewMockJsonRestHandler(ctrl)

			var body []byte
			if tt.msg != nil && tt.msg.Data != nil {
				body = validBody
			}

			headers := map[string]string{api.VersionHeader: version.String(version.Gloas)}
			handler.EXPECT().Post(
				gomock.Any(),
				"/eth/v1/beacon/pool/payload_attestations",
				headers,
				bytes.NewBuffer(body),
				nil,
			).Return(tt.endpointError).Times(tt.endpointCall)

			client := &beaconApiValidatorClient{handler: handler}
			err := client.submitPayloadAttestation(t.Context(), tt.msg)
			if tt.expectErr != "" {
				require.ErrorContains(t, tt.expectErr, err)
				return
			}
			require.NoError(t, err)
		})
	}
}
