package beacon_api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/OffchainLabs/prysm/v6/api"
	"github.com/OffchainLabs/prysm/v6/api/server/structs"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/validator/client/beacon-api/mock"
	testhelpers "github.com/OffchainLabs/prysm/v6/validator/client/beacon-api/test-helpers"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"go.uber.org/mock/gomock"
)

func TestGetBeaconBlock_RequestFailed(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := t.Context()

	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().GetSSZ(
		gomock.Any(),
		gomock.Any(),
	).Return(
		nil,
		nil,
		errors.New("foo error"),
	).Times(1)

	validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	_, err := validatorClient.beaconBlock(ctx, 1, []byte{1}, []byte{2})
	assert.ErrorContains(t, "foo error", err)
}

func TestGetBeaconBlock_Error(t *testing.T) {
	testCases := []struct {
		name                 string
		beaconBlock          interface{}
		expectedErrorMessage string
		consensusVersion     string
		blinded              bool
	}{
		{
			name:                 "phase0 block decoding failed",
			expectedErrorMessage: "failed to convert phase0 block: could not decode ",
			consensusVersion:     "phase0",
		},
		{
			name:                 "altair block decoding failed",
			expectedErrorMessage: "failed to convert altair block: could not decode ",
			consensusVersion:     "altair",
		},
		{
			name:                 "bellatrix block decoding failed",
			expectedErrorMessage: "failed to convert bellatrix block: could not decode ",
			beaconBlock:          "foo",
			consensusVersion:     "bellatrix",
			blinded:              false,
		},
		{
			name:                 "blinded bellatrix block decoding failed",
			expectedErrorMessage: "failed to convert bellatrix block: could not decode ",
			beaconBlock:          "foo",
			consensusVersion:     "bellatrix",
			blinded:              true,
		},
		{
			name:                 "capella block decoding failed",
			expectedErrorMessage: "failed to convert capella block: could not decode ",
			beaconBlock:          "foo",
			consensusVersion:     "capella",
			blinded:              false,
		},
		{
			name:                 "blinded capella block decoding failed",
			expectedErrorMessage: "failed to convert capella block: could not decode ",
			beaconBlock:          "foo",
			consensusVersion:     "capella",
			blinded:              true,
		},
		{
			name:                 "deneb block decoding failed",
			expectedErrorMessage: "failed to convert deneb block: could not decode ",
			beaconBlock:          "foo",
			consensusVersion:     "deneb",
			blinded:              false,
		},
		{
			name:                 "blinded deneb block decoding failed",
			expectedErrorMessage: "failed to convert deneb block: could not decode ",
			beaconBlock:          "foo",
			consensusVersion:     "deneb",
			blinded:              true,
		},
		{
			name:                 "electra block decoding failed",
			expectedErrorMessage: "failed to convert electra block: could not decode ",
			beaconBlock:          "foo",
			consensusVersion:     "electra",
			blinded:              false,
		},
		{
			name:                 "blinded electra block decoding failed",
			expectedErrorMessage: "failed to convert electra block: could not decode ",
			beaconBlock:          "foo",
			consensusVersion:     "electra",
			blinded:              true,
		},
		{
			name:                 "fulu block decoding failed",
			expectedErrorMessage: "failed to convert fulu block: could not decode ",
			beaconBlock:          "foo",
			consensusVersion:     "fulu",
			blinded:              false,
		},
		{
			name:                 "blinded fulu block decoding failed",
			expectedErrorMessage: "failed to convert fulu block: could not decode ",
			beaconBlock:          "foo",
			consensusVersion:     "fulu",
			blinded:              true,
		},
		{
			name:                 "unsupported consensus version",
			expectedErrorMessage: "unsupported consensus version `foo`",
			consensusVersion:     "foo",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ctx := t.Context()

			resp := structs.ProduceBlockV3Response{
				Version: testCase.consensusVersion,
				Data:    json.RawMessage(`{}`), // ← valid JSON object
			}

			b, err := json.Marshal(resp)
			require.NoError(t, err)
			jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
			jsonRestHandler.EXPECT().GetSSZ(
				gomock.Any(),
				gomock.Any(),
			).Return(
				b,
				http.Header{"Content-Type": []string{"application/json"}},
				nil,
			).Times(1)

			validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
			_, err = validatorClient.beaconBlock(ctx, 1, []byte{1}, []byte{2})
			assert.ErrorContains(t, testCase.expectedErrorMessage, err)
		})
	}
}

func TestGetBeaconBlock_Phase0Valid(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	proto := testhelpers.GenerateProtoPhase0BeaconBlock()
	block := testhelpers.GenerateJsonPhase0BeaconBlock()
	bytes, err := json.Marshal(block)
	require.NoError(t, err)

	const slot = primitives.Slot(1)
	randaoReveal := []byte{2}
	graffiti := []byte{3}
	ctx := t.Context()

	b, err := json.Marshal(structs.ProduceBlockV3Response{
		Version: "phase0",
		Data:    bytes,
	})
	require.NoError(t, err)
	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().GetSSZ(
		gomock.Any(),
		fmt.Sprintf("/eth/v3/validator/blocks/%d?graffiti=%s&randao_reveal=%s", slot, hexutil.Encode(graffiti), hexutil.Encode(randaoReveal)),
	).Return(
		b,
		http.Header{"Content-Type": []string{"application/json"}},
		nil,
	).Times(1)

	validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	beaconBlock, err := validatorClient.beaconBlock(ctx, slot, randaoReveal, graffiti)
	require.NoError(t, err)

	expectedBeaconBlock := &ethpb.GenericBeaconBlock{
		Block: &ethpb.GenericBeaconBlock_Phase0{
			Phase0: proto,
		},
	}

	assert.DeepEqual(t, expectedBeaconBlock, beaconBlock)
}

// Add SSZ test cases below this line

func TestGetBeaconBlock_SSZ_BellatrixValid(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	proto := testhelpers.GenerateProtoBellatrixBeaconBlock()
	bytes, err := proto.MarshalSSZ()
	require.NoError(t, err)

	const slot = primitives.Slot(1)
	randaoReveal := []byte{2}
	graffiti := []byte{3}

	ctx := t.Context()

	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().GetSSZ(
		gomock.Any(),
		fmt.Sprintf("/eth/v3/validator/blocks/%d?graffiti=%s&randao_reveal=%s", slot, hexutil.Encode(graffiti), hexutil.Encode(randaoReveal)),
	).Return(
		bytes,
		http.Header{
			"Content-Type":                    []string{api.OctetStreamMediaType},
			api.VersionHeader:                 []string{"bellatrix"},
			api.ExecutionPayloadBlindedHeader: []string{"false"},
		},
		nil,
	).Times(1)

	validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	beaconBlock, err := validatorClient.beaconBlock(ctx, slot, randaoReveal, graffiti)
	require.NoError(t, err)

	expectedBeaconBlock := &ethpb.GenericBeaconBlock{
		Block: &ethpb.GenericBeaconBlock_Bellatrix{
			Bellatrix: proto,
		},
		IsBlinded: false,
	}

	assert.DeepEqual(t, expectedBeaconBlock, beaconBlock)
}

func TestGetBeaconBlock_SSZ_BlindedBellatrixValid(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	proto := testhelpers.GenerateProtoBlindedBellatrixBeaconBlock()
	bytes, err := proto.MarshalSSZ()
	require.NoError(t, err)

	const slot = primitives.Slot(1)
	randaoReveal := []byte{2}
	graffiti := []byte{3}

	ctx := t.Context()

	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().GetSSZ(
		gomock.Any(),
		fmt.Sprintf("/eth/v3/validator/blocks/%d?graffiti=%s&randao_reveal=%s", slot, hexutil.Encode(graffiti), hexutil.Encode(randaoReveal)),
	).Return(
		bytes,
		http.Header{
			"Content-Type":                    []string{api.OctetStreamMediaType},
			api.VersionHeader:                 []string{"bellatrix"},
			api.ExecutionPayloadBlindedHeader: []string{"true"},
		},
		nil,
	).Times(1)

	validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	beaconBlock, err := validatorClient.beaconBlock(ctx, slot, randaoReveal, graffiti)
	require.NoError(t, err)

	expectedBeaconBlock := &ethpb.GenericBeaconBlock{
		Block: &ethpb.GenericBeaconBlock_BlindedBellatrix{
			BlindedBellatrix: proto,
		},
		IsBlinded: true,
	}

	assert.DeepEqual(t, expectedBeaconBlock, beaconBlock)
}

func TestGetBeaconBlock_SSZ_CapellaValid(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	proto := testhelpers.GenerateProtoCapellaBeaconBlock()
	bytes, err := proto.MarshalSSZ()
	require.NoError(t, err)

	const slot = primitives.Slot(1)
	randaoReveal := []byte{2}
	graffiti := []byte{3}

	ctx := t.Context()

	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().GetSSZ(
		gomock.Any(),
		fmt.Sprintf("/eth/v3/validator/blocks/%d?graffiti=%s&randao_reveal=%s", slot, hexutil.Encode(graffiti), hexutil.Encode(randaoReveal)),
	).Return(
		bytes,
		http.Header{
			"Content-Type":                    []string{api.OctetStreamMediaType},
			api.VersionHeader:                 []string{"capella"},
			api.ExecutionPayloadBlindedHeader: []string{"false"},
		},
		nil,
	).Times(1)

	validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	beaconBlock, err := validatorClient.beaconBlock(ctx, slot, randaoReveal, graffiti)
	require.NoError(t, err)

	expectedBeaconBlock := &ethpb.GenericBeaconBlock{
		Block: &ethpb.GenericBeaconBlock_Capella{
			Capella: proto,
		},
		IsBlinded: false,
	}

	assert.DeepEqual(t, expectedBeaconBlock, beaconBlock)
}

func TestGetBeaconBlock_SSZ_BlindedCapellaValid(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	proto := testhelpers.GenerateProtoBlindedCapellaBeaconBlock()
	bytes, err := proto.MarshalSSZ()
	require.NoError(t, err)

	const slot = primitives.Slot(1)
	randaoReveal := []byte{2}
	graffiti := []byte{3}

	ctx := t.Context()

	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().GetSSZ(
		gomock.Any(),
		fmt.Sprintf("/eth/v3/validator/blocks/%d?graffiti=%s&randao_reveal=%s", slot, hexutil.Encode(graffiti), hexutil.Encode(randaoReveal)),
	).Return(
		bytes,
		http.Header{
			"Content-Type":                    []string{api.OctetStreamMediaType},
			api.VersionHeader:                 []string{"capella"},
			api.ExecutionPayloadBlindedHeader: []string{"true"},
		},
		nil,
	).Times(1)

	validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	beaconBlock, err := validatorClient.beaconBlock(ctx, slot, randaoReveal, graffiti)
	require.NoError(t, err)

	expectedBeaconBlock := &ethpb.GenericBeaconBlock{
		Block: &ethpb.GenericBeaconBlock_BlindedCapella{
			BlindedCapella: proto,
		},
		IsBlinded: true,
	}

	assert.DeepEqual(t, expectedBeaconBlock, beaconBlock)
}

func TestGetBeaconBlock_SSZ_DenebValid(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	proto := testhelpers.GenerateProtoDenebBeaconBlockContents()
	bytes, err := proto.MarshalSSZ()
	require.NoError(t, err)

	const slot = primitives.Slot(1)
	randaoReveal := []byte{2}
	graffiti := []byte{3}

	ctx := t.Context()

	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().GetSSZ(
		gomock.Any(),
		fmt.Sprintf("/eth/v3/validator/blocks/%d?graffiti=%s&randao_reveal=%s", slot, hexutil.Encode(graffiti), hexutil.Encode(randaoReveal)),
	).Return(
		bytes,
		http.Header{
			"Content-Type":                    []string{api.OctetStreamMediaType},
			api.VersionHeader:                 []string{"deneb"},
			api.ExecutionPayloadBlindedHeader: []string{"false"},
		},
		nil,
	).Times(1)

	validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	beaconBlock, err := validatorClient.beaconBlock(ctx, slot, randaoReveal, graffiti)
	require.NoError(t, err)

	expectedBeaconBlock := &ethpb.GenericBeaconBlock{
		Block: &ethpb.GenericBeaconBlock_Deneb{
			Deneb: proto,
		},
		IsBlinded: false,
	}

	assert.DeepEqual(t, expectedBeaconBlock, beaconBlock)
}

func TestGetBeaconBlock_SSZ_BlindedDenebValid(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	proto := testhelpers.GenerateProtoBlindedDenebBeaconBlock()
	bytes, err := proto.MarshalSSZ()
	require.NoError(t, err)

	const slot = primitives.Slot(1)
	randaoReveal := []byte{2}
	graffiti := []byte{3}

	ctx := t.Context()

	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().GetSSZ(
		gomock.Any(),
		fmt.Sprintf("/eth/v3/validator/blocks/%d?graffiti=%s&randao_reveal=%s", slot, hexutil.Encode(graffiti), hexutil.Encode(randaoReveal)),
	).Return(
		bytes,
		http.Header{
			"Content-Type":                    []string{api.OctetStreamMediaType},
			api.VersionHeader:                 []string{"deneb"},
			api.ExecutionPayloadBlindedHeader: []string{"true"},
		},
		nil,
	).Times(1)

	validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	beaconBlock, err := validatorClient.beaconBlock(ctx, slot, randaoReveal, graffiti)
	require.NoError(t, err)

	expectedBeaconBlock := &ethpb.GenericBeaconBlock{
		Block: &ethpb.GenericBeaconBlock_BlindedDeneb{
			BlindedDeneb: proto,
		},
		IsBlinded: true,
	}

	assert.DeepEqual(t, expectedBeaconBlock, beaconBlock)
}

func TestGetBeaconBlock_SSZ_ElectraValid(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	proto := testhelpers.GenerateProtoElectraBeaconBlockContents()
	bytes, err := proto.MarshalSSZ()
	require.NoError(t, err)

	const slot = primitives.Slot(1)
	randaoReveal := []byte{2}
	graffiti := []byte{3}

	ctx := t.Context()

	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().GetSSZ(
		gomock.Any(),
		fmt.Sprintf("/eth/v3/validator/blocks/%d?graffiti=%s&randao_reveal=%s", slot, hexutil.Encode(graffiti), hexutil.Encode(randaoReveal)),
	).Return(
		bytes,
		http.Header{
			"Content-Type":                    []string{api.OctetStreamMediaType},
			api.VersionHeader:                 []string{"electra"},
			api.ExecutionPayloadBlindedHeader: []string{"false"},
		},
		nil,
	).Times(1)

	validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	beaconBlock, err := validatorClient.beaconBlock(ctx, slot, randaoReveal, graffiti)
	require.NoError(t, err)

	expectedBeaconBlock := &ethpb.GenericBeaconBlock{
		Block: &ethpb.GenericBeaconBlock_Electra{
			Electra: proto,
		},
		IsBlinded: false,
	}

	assert.DeepEqual(t, expectedBeaconBlock, beaconBlock)
}

func TestGetBeaconBlock_SSZ_BlindedElectraValid(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	proto := testhelpers.GenerateProtoBlindedElectraBeaconBlock()
	bytes, err := proto.MarshalSSZ()
	require.NoError(t, err)

	const slot = primitives.Slot(1)
	randaoReveal := []byte{2}
	graffiti := []byte{3}

	ctx := t.Context()

	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().GetSSZ(
		gomock.Any(),
		fmt.Sprintf("/eth/v3/validator/blocks/%d?graffiti=%s&randao_reveal=%s", slot, hexutil.Encode(graffiti), hexutil.Encode(randaoReveal)),
	).Return(
		bytes,
		http.Header{
			"Content-Type":                    []string{api.OctetStreamMediaType},
			api.VersionHeader:                 []string{"electra"},
			api.ExecutionPayloadBlindedHeader: []string{"true"},
		},
		nil,
	).Times(1)

	validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	beaconBlock, err := validatorClient.beaconBlock(ctx, slot, randaoReveal, graffiti)
	require.NoError(t, err)

	expectedBeaconBlock := &ethpb.GenericBeaconBlock{
		Block: &ethpb.GenericBeaconBlock_BlindedElectra{
			BlindedElectra: proto,
		},
		IsBlinded: true,
	}

	assert.DeepEqual(t, expectedBeaconBlock, beaconBlock)
}

func TestGetBeaconBlock_SSZ_UnsupportedVersion(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	const slot = primitives.Slot(1)
	randaoReveal := []byte{2}
	graffiti := []byte{3}

	ctx := t.Context()

	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().GetSSZ(
		gomock.Any(),
		fmt.Sprintf("/eth/v3/validator/blocks/%d?graffiti=%s&randao_reveal=%s", slot, hexutil.Encode(graffiti), hexutil.Encode(randaoReveal)),
	).Return(
		[]byte{},
		http.Header{
			"Content-Type":                    []string{api.OctetStreamMediaType},
			api.VersionHeader:                 []string{"unsupported"},
			api.ExecutionPayloadBlindedHeader: []string{"false"},
		},
		nil,
	).Times(1)

	validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	_, err := validatorClient.beaconBlock(ctx, slot, randaoReveal, graffiti)
	assert.ErrorContains(t, "version name doesn't map to a known value in the enum", err)
}

func TestGetBeaconBlock_SSZ_InvalidBlindedHeader(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	proto := testhelpers.GenerateProtoBellatrixBeaconBlock()
	bytes, err := proto.MarshalSSZ()
	require.NoError(t, err)

	const slot = primitives.Slot(1)
	randaoReveal := []byte{2}
	graffiti := []byte{3}

	ctx := t.Context()

	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().GetSSZ(
		gomock.Any(),
		fmt.Sprintf("/eth/v3/validator/blocks/%d?graffiti=%s&randao_reveal=%s", slot, hexutil.Encode(graffiti), hexutil.Encode(randaoReveal)),
	).Return(
		bytes,
		http.Header{
			"Content-Type":                    []string{api.OctetStreamMediaType},
			api.VersionHeader:                 []string{"bellatrix"},
			api.ExecutionPayloadBlindedHeader: []string{"invalid"},
		},
		nil,
	).Times(1)

	validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	_, err = validatorClient.beaconBlock(ctx, slot, randaoReveal, graffiti)
	assert.ErrorContains(t, "strconv.ParseBool: parsing \"invalid\": invalid syntax", err)
}

func TestGetBeaconBlock_SSZ_InvalidVersionHeader(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	proto := testhelpers.GenerateProtoBellatrixBeaconBlock()
	bytes, err := proto.MarshalSSZ()
	require.NoError(t, err)

	const slot = primitives.Slot(1)
	randaoReveal := []byte{2}
	graffiti := []byte{3}

	ctx := t.Context()

	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().GetSSZ(
		gomock.Any(),
		fmt.Sprintf("/eth/v3/validator/blocks/%d?graffiti=%s&randao_reveal=%s", slot, hexutil.Encode(graffiti), hexutil.Encode(randaoReveal)),
	).Return(
		bytes,
		http.Header{
			"Content-Type":                    []string{api.OctetStreamMediaType},
			api.VersionHeader:                 []string{"invalid"},
			api.ExecutionPayloadBlindedHeader: []string{"false"},
		},
		nil,
	).Times(1)

	validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	_, err = validatorClient.beaconBlock(ctx, slot, randaoReveal, graffiti)
	assert.ErrorContains(t, "unsupported header version invalid", err)
}

func TestGetBeaconBlock_SSZ_GetSSZError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	const slot = primitives.Slot(1)
	randaoReveal := []byte{2}
	graffiti := []byte{3}

	ctx := t.Context()

	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().GetSSZ(
		gomock.Any(),
		fmt.Sprintf("/eth/v3/validator/blocks/%d?graffiti=%s&randao_reveal=%s", slot, hexutil.Encode(graffiti), hexutil.Encode(randaoReveal)),
	).Return(
		nil,
		nil,
		errors.New("get ssz error"),
	).Times(1)

	validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	_, err := validatorClient.beaconBlock(ctx, slot, randaoReveal, graffiti)
	assert.ErrorContains(t, "get ssz error", err)
}

func TestGetBeaconBlock_SSZ_Phase0Valid(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	proto := testhelpers.GenerateProtoPhase0BeaconBlock()
	bytes, err := proto.MarshalSSZ()
	require.NoError(t, err)

	const slot = primitives.Slot(1)
	randaoReveal := []byte{2}
	graffiti := []byte{3}

	ctx := t.Context()

	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().GetSSZ(
		gomock.Any(),
		fmt.Sprintf("/eth/v3/validator/blocks/%d?graffiti=%s&randao_reveal=%s", slot, hexutil.Encode(graffiti), hexutil.Encode(randaoReveal)),
	).Return(
		bytes,
		http.Header{
			"Content-Type":                    []string{api.OctetStreamMediaType},
			api.VersionHeader:                 []string{"phase0"},
			api.ExecutionPayloadBlindedHeader: []string{"false"},
		},
		nil,
	).Times(1)

	validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	beaconBlock, err := validatorClient.beaconBlock(ctx, slot, randaoReveal, graffiti)
	require.NoError(t, err)

	expectedBeaconBlock := &ethpb.GenericBeaconBlock{
		Block: &ethpb.GenericBeaconBlock_Phase0{
			Phase0: proto,
		},
		IsBlinded: false,
	}

	assert.DeepEqual(t, expectedBeaconBlock, beaconBlock)
}

func TestGetBeaconBlock_SSZ_AltairValid(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	proto := testhelpers.GenerateProtoAltairBeaconBlock()
	bytes, err := proto.MarshalSSZ()
	require.NoError(t, err)

	const slot = primitives.Slot(1)
	randaoReveal := []byte{2}
	graffiti := []byte{3}

	ctx := t.Context()

	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().GetSSZ(
		gomock.Any(),
		fmt.Sprintf("/eth/v3/validator/blocks/%d?graffiti=%s&randao_reveal=%s", slot, hexutil.Encode(graffiti), hexutil.Encode(randaoReveal)),
	).Return(
		bytes,
		http.Header{
			"Content-Type":                    []string{api.OctetStreamMediaType},
			api.VersionHeader:                 []string{"altair"},
			api.ExecutionPayloadBlindedHeader: []string{"false"},
		},
		nil,
	).Times(1)

	validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	beaconBlock, err := validatorClient.beaconBlock(ctx, slot, randaoReveal, graffiti)
	require.NoError(t, err)

	expectedBeaconBlock := &ethpb.GenericBeaconBlock{
		Block: &ethpb.GenericBeaconBlock_Altair{
			Altair: proto,
		},
		IsBlinded: false,
	}

	assert.DeepEqual(t, expectedBeaconBlock, beaconBlock)
}

func TestGetBeaconBlock_AltairValid(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	proto := testhelpers.GenerateProtoAltairBeaconBlock()
	block := testhelpers.GenerateJsonAltairBeaconBlock()
	bytes, err := json.Marshal(block)
	require.NoError(t, err)

	const slot = primitives.Slot(1)
	randaoReveal := []byte{2}
	graffiti := []byte{3}

	ctx := t.Context()

	b, err := json.Marshal(structs.ProduceBlockV3Response{
		Version: "altair",
		Data:    bytes,
	})
	require.NoError(t, err)
	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().GetSSZ(
		gomock.Any(),
		fmt.Sprintf("/eth/v3/validator/blocks/%d?graffiti=%s&randao_reveal=%s", slot, hexutil.Encode(graffiti), hexutil.Encode(randaoReveal)),
	).Return(
		b,
		http.Header{"Content-Type": []string{"application/json"}},
		nil,
	).Times(1)

	validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	beaconBlock, err := validatorClient.beaconBlock(ctx, slot, randaoReveal, graffiti)
	require.NoError(t, err)

	expectedBeaconBlock := &ethpb.GenericBeaconBlock{
		Block: &ethpb.GenericBeaconBlock_Altair{
			Altair: proto,
		},
	}

	assert.DeepEqual(t, expectedBeaconBlock, beaconBlock)
}

func TestGetBeaconBlock_BellatrixValid(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	proto := testhelpers.GenerateProtoBellatrixBeaconBlock()
	block := testhelpers.GenerateJsonBellatrixBeaconBlock()
	bytes, err := json.Marshal(block)
	require.NoError(t, err)

	const slot = primitives.Slot(1)
	randaoReveal := []byte{2}
	graffiti := []byte{3}

	ctx := t.Context()

	b, err := json.Marshal(structs.ProduceBlockV3Response{
		Version:                 "bellatrix",
		ExecutionPayloadBlinded: false,
		Data:                    bytes,
	})
	require.NoError(t, err)
	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().GetSSZ(
		gomock.Any(),
		fmt.Sprintf("/eth/v3/validator/blocks/%d?graffiti=%s&randao_reveal=%s", slot, hexutil.Encode(graffiti), hexutil.Encode(randaoReveal)),
	).Return(
		b,
		http.Header{"Content-Type": []string{"application/json"}},
		nil,
	).Times(1)

	validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	beaconBlock, err := validatorClient.beaconBlock(ctx, slot, randaoReveal, graffiti)
	require.NoError(t, err)

	expectedBeaconBlock := &ethpb.GenericBeaconBlock{
		Block: &ethpb.GenericBeaconBlock_Bellatrix{
			Bellatrix: proto,
		},
		IsBlinded: false,
	}

	assert.DeepEqual(t, expectedBeaconBlock, beaconBlock)
}

func TestGetBeaconBlock_BlindedBellatrixValid(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	proto := testhelpers.GenerateProtoBlindedBellatrixBeaconBlock()
	block := testhelpers.GenerateJsonBlindedBellatrixBeaconBlock()
	bytes, err := json.Marshal(block)
	require.NoError(t, err)

	const slot = primitives.Slot(1)
	randaoReveal := []byte{2}
	graffiti := []byte{3}

	ctx := t.Context()

	b, err := json.Marshal(structs.ProduceBlockV3Response{
		Version:                 "bellatrix",
		ExecutionPayloadBlinded: true,
		Data:                    bytes,
	})
	require.NoError(t, err)
	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().GetSSZ(
		gomock.Any(),
		fmt.Sprintf("/eth/v3/validator/blocks/%d?graffiti=%s&randao_reveal=%s", slot, hexutil.Encode(graffiti), hexutil.Encode(randaoReveal)),
	).Return(
		b,
		http.Header{"Content-Type": []string{"application/json"}},
		nil,
	).Times(1)

	validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	beaconBlock, err := validatorClient.beaconBlock(ctx, slot, randaoReveal, graffiti)
	require.NoError(t, err)

	expectedBeaconBlock := &ethpb.GenericBeaconBlock{
		Block: &ethpb.GenericBeaconBlock_BlindedBellatrix{
			BlindedBellatrix: proto,
		},
		IsBlinded: true,
	}

	assert.DeepEqual(t, expectedBeaconBlock, beaconBlock)
}

func TestGetBeaconBlock_CapellaValid(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	proto := testhelpers.GenerateProtoCapellaBeaconBlock()
	block := testhelpers.GenerateJsonCapellaBeaconBlock()
	bytes, err := json.Marshal(block)
	require.NoError(t, err)

	const slot = primitives.Slot(1)
	randaoReveal := []byte{2}
	graffiti := []byte{3}

	ctx := t.Context()

	b, err := json.Marshal(structs.ProduceBlockV3Response{
		Version:                 "capella",
		ExecutionPayloadBlinded: false,
		Data:                    bytes,
	})
	require.NoError(t, err)
	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().GetSSZ(
		gomock.Any(),
		fmt.Sprintf("/eth/v3/validator/blocks/%d?graffiti=%s&randao_reveal=%s", slot, hexutil.Encode(graffiti), hexutil.Encode(randaoReveal)),
	).Return(
		b,
		http.Header{"Content-Type": []string{"application/json"}},
		nil,
	).Times(1)

	validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	beaconBlock, err := validatorClient.beaconBlock(ctx, slot, randaoReveal, graffiti)
	require.NoError(t, err)

	expectedBeaconBlock := &ethpb.GenericBeaconBlock{
		Block: &ethpb.GenericBeaconBlock_Capella{
			Capella: proto,
		},
		IsBlinded: false,
	}

	assert.DeepEqual(t, expectedBeaconBlock, beaconBlock)
}

func TestGetBeaconBlock_BlindedCapellaValid(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	proto := testhelpers.GenerateProtoBlindedCapellaBeaconBlock()
	block := testhelpers.GenerateJsonBlindedCapellaBeaconBlock()
	bytes, err := json.Marshal(block)
	require.NoError(t, err)

	const slot = primitives.Slot(1)
	randaoReveal := []byte{2}
	graffiti := []byte{3}

	ctx := t.Context()

	b, err := json.Marshal(structs.ProduceBlockV3Response{
		Version:                 "capella",
		ExecutionPayloadBlinded: true,
		Data:                    bytes,
	})
	require.NoError(t, err)
	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().GetSSZ(
		gomock.Any(),
		fmt.Sprintf("/eth/v3/validator/blocks/%d?graffiti=%s&randao_reveal=%s", slot, hexutil.Encode(graffiti), hexutil.Encode(randaoReveal)),
	).Return(
		b,
		http.Header{"Content-Type": []string{"application/json"}},
		nil,
	).Times(1)

	validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	beaconBlock, err := validatorClient.beaconBlock(ctx, slot, randaoReveal, graffiti)
	require.NoError(t, err)

	expectedBeaconBlock := &ethpb.GenericBeaconBlock{
		Block: &ethpb.GenericBeaconBlock_BlindedCapella{
			BlindedCapella: proto,
		},
		IsBlinded: true,
	}

	assert.DeepEqual(t, expectedBeaconBlock, beaconBlock)
}

func TestGetBeaconBlock_DenebValid(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	proto := testhelpers.GenerateProtoDenebBeaconBlockContents()
	block := testhelpers.GenerateJsonDenebBeaconBlockContents()
	bytes, err := json.Marshal(block)
	require.NoError(t, err)

	const slot = primitives.Slot(1)
	randaoReveal := []byte{2}
	graffiti := []byte{3}

	ctx := t.Context()

	b, err := json.Marshal(structs.ProduceBlockV3Response{
		Version:                 "deneb",
		ExecutionPayloadBlinded: false,
		Data:                    bytes,
	})
	require.NoError(t, err)
	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().GetSSZ(
		gomock.Any(),
		fmt.Sprintf("/eth/v3/validator/blocks/%d?graffiti=%s&randao_reveal=%s", slot, hexutil.Encode(graffiti), hexutil.Encode(randaoReveal)),
	).Return(
		b,
		http.Header{"Content-Type": []string{"application/json"}},
		nil,
	).Times(1)

	validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	beaconBlock, err := validatorClient.beaconBlock(ctx, slot, randaoReveal, graffiti)
	require.NoError(t, err)

	expectedBeaconBlock := &ethpb.GenericBeaconBlock{
		Block: &ethpb.GenericBeaconBlock_Deneb{
			Deneb: proto,
		},
		IsBlinded: false,
	}

	assert.DeepEqual(t, expectedBeaconBlock, beaconBlock)
}

func TestGetBeaconBlock_BlindedDenebValid(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	proto := testhelpers.GenerateProtoBlindedDenebBeaconBlock()
	block := testhelpers.GenerateJsonBlindedDenebBeaconBlock()
	bytes, err := json.Marshal(block)
	require.NoError(t, err)

	const slot = primitives.Slot(1)
	randaoReveal := []byte{2}
	graffiti := []byte{3}

	ctx := t.Context()

	b, err := json.Marshal(structs.ProduceBlockV3Response{
		Version:                 "deneb",
		ExecutionPayloadBlinded: true,
		Data:                    bytes,
	})
	require.NoError(t, err)
	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().GetSSZ(
		gomock.Any(),
		fmt.Sprintf("/eth/v3/validator/blocks/%d?graffiti=%s&randao_reveal=%s", slot, hexutil.Encode(graffiti), hexutil.Encode(randaoReveal)),
	).Return(
		b,
		http.Header{"Content-Type": []string{"application/json"}},
		nil,
	).Times(1)

	validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	beaconBlock, err := validatorClient.beaconBlock(ctx, slot, randaoReveal, graffiti)
	require.NoError(t, err)

	expectedBeaconBlock := &ethpb.GenericBeaconBlock{
		Block: &ethpb.GenericBeaconBlock_BlindedDeneb{
			BlindedDeneb: proto,
		},
		IsBlinded: true,
	}

	assert.DeepEqual(t, expectedBeaconBlock, beaconBlock)
}

func TestGetBeaconBlock_ElectraValid(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	proto := testhelpers.GenerateProtoElectraBeaconBlockContents()
	block := testhelpers.GenerateJsonElectraBeaconBlockContents()
	bytes, err := json.Marshal(block)
	require.NoError(t, err)

	const slot = primitives.Slot(1)
	randaoReveal := []byte{2}
	graffiti := []byte{3}

	ctx := t.Context()

	b, err := json.Marshal(structs.ProduceBlockV3Response{
		Version:                 "electra",
		ExecutionPayloadBlinded: false,
		Data:                    bytes,
	})
	require.NoError(t, err)
	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().GetSSZ(
		gomock.Any(),
		fmt.Sprintf("/eth/v3/validator/blocks/%d?graffiti=%s&randao_reveal=%s", slot, hexutil.Encode(graffiti), hexutil.Encode(randaoReveal)),
	).Return(
		b,
		http.Header{"Content-Type": []string{"application/json"}},
		nil,
	).Times(1)

	validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	beaconBlock, err := validatorClient.beaconBlock(ctx, slot, randaoReveal, graffiti)
	require.NoError(t, err)

	expectedBeaconBlock := &ethpb.GenericBeaconBlock{
		Block: &ethpb.GenericBeaconBlock_Electra{
			Electra: proto,
		},
		IsBlinded: false,
	}

	assert.DeepEqual(t, expectedBeaconBlock, beaconBlock)
}

func TestGetBeaconBlock_BlindedElectraValid(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	proto := testhelpers.GenerateProtoBlindedElectraBeaconBlock()
	block := testhelpers.GenerateJsonBlindedElectraBeaconBlock()
	bytes, err := json.Marshal(block)
	require.NoError(t, err)

	const slot = primitives.Slot(1)
	randaoReveal := []byte{2}
	graffiti := []byte{3}

	ctx := t.Context()

	b, err := json.Marshal(structs.ProduceBlockV3Response{
		Version:                 "electra",
		ExecutionPayloadBlinded: true,
		Data:                    bytes,
	})
	require.NoError(t, err)
	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().GetSSZ(
		gomock.Any(),
		fmt.Sprintf("/eth/v3/validator/blocks/%d?graffiti=%s&randao_reveal=%s", slot, hexutil.Encode(graffiti), hexutil.Encode(randaoReveal)),
	).Return(
		b,
		http.Header{"Content-Type": []string{"application/json"}},
		nil,
	).Times(1)

	validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	beaconBlock, err := validatorClient.beaconBlock(ctx, slot, randaoReveal, graffiti)
	require.NoError(t, err)

	expectedBeaconBlock := &ethpb.GenericBeaconBlock{
		Block: &ethpb.GenericBeaconBlock_BlindedElectra{
			BlindedElectra: proto,
		},
		IsBlinded: true,
	}

	assert.DeepEqual(t, expectedBeaconBlock, beaconBlock)
}
