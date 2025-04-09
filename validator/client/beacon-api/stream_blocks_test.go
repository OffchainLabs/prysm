package beacon_api

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/api/server/structs"
	rpctesting "github.com/prysmaticlabs/prysm/v5/beacon-chain/rpc/eth/shared/testing"
	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/validator/client/beacon-api/mock"
	testhelpers "github.com/prysmaticlabs/prysm/v5/validator/client/beacon-api/test-helpers"
	"go.uber.org/mock/gomock"
)

func TestStreamBlocks_UnsupportedConsensusVersion(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().Get(
		gomock.Any(),
		gomock.Any(),
		&abstractSignedBlockResponseJson{},
	).SetArg(
		2,
		abstractSignedBlockResponseJson{Version: "foo"},
	).Return(
		nil,
	).Times(1)

	validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	streamBlocksClient := validatorClient.streamBlocks(ctx, &eth.StreamBlocksRequest{}, time.Millisecond*100)
	_, err := streamBlocksClient.Recv()
	assert.ErrorContains(t, "unsupported consensus version `foo`", err)
}

func TestStreamBlocks_Error(t *testing.T) {
	testSuites := []struct {
		consensusVersion             string
		generateBeaconBlockConverter func(ctrl *gomock.Controller, conversionError error) *mock.MockBeaconBlockConverter
	}{
		{
			consensusVersion: "phase0",
			generateBeaconBlockConverter: func(ctrl *gomock.Controller, conversionError error) *mock.MockBeaconBlockConverter {
				beaconBlockConverter := mock.NewMockBeaconBlockConverter(ctrl)
				beaconBlockConverter.EXPECT().ConvertRESTPhase0BlockToProto(
					gomock.Any(),
				).Return(
					nil,
					conversionError,
				).AnyTimes()

				return beaconBlockConverter
			},
		},
		{
			consensusVersion: "altair",
			generateBeaconBlockConverter: func(ctrl *gomock.Controller, conversionError error) *mock.MockBeaconBlockConverter {
				beaconBlockConverter := mock.NewMockBeaconBlockConverter(ctrl)
				beaconBlockConverter.EXPECT().ConvertRESTAltairBlockToProto(
					gomock.Any(),
				).Return(
					nil,
					conversionError,
				).AnyTimes()

				return beaconBlockConverter
			},
		},
		{
			consensusVersion: "bellatrix",
			generateBeaconBlockConverter: func(ctrl *gomock.Controller, conversionError error) *mock.MockBeaconBlockConverter {
				beaconBlockConverter := mock.NewMockBeaconBlockConverter(ctrl)
				beaconBlockConverter.EXPECT().ConvertRESTBellatrixBlockToProto(
					gomock.Any(),
				).Return(
					nil,
					conversionError,
				).AnyTimes()

				return beaconBlockConverter
			},
		},
		{
			consensusVersion: "capella",
			generateBeaconBlockConverter: func(ctrl *gomock.Controller, conversionError error) *mock.MockBeaconBlockConverter {
				beaconBlockConverter := mock.NewMockBeaconBlockConverter(ctrl)
				beaconBlockConverter.EXPECT().ConvertRESTCapellaBlockToProto(
					gomock.Any(),
				).Return(
					nil,
					conversionError,
				).AnyTimes()

				return beaconBlockConverter
			},
		},
	}

	testCases := []struct {
		name                 string
		expectedErrorMessage string
		conversionError      error
		generateData         func(consensusVersion string) []byte
	}{
		{
			name:                 "block decoding failed",
			expectedErrorMessage: "failed to decode signed %s block response json",
			generateData:         func(consensusVersion string) []byte { return []byte{} },
		},
		{
			name:                 "block conversion failed",
			expectedErrorMessage: "failed to get signed %s block",
			conversionError:      errors.New("foo"),
			generateData: func(consensusVersion string) []byte {
				blockBytes, err := json.Marshal(structs.SignedBeaconBlock{Signature: "0x01"})
				require.NoError(t, err)
				return blockBytes
			},
		},
		{
			name:                 "signature decoding failed",
			expectedErrorMessage: "failed to decode %s block signature `foo`",
			generateData: func(consensusVersion string) []byte {
				blockBytes, err := json.Marshal(structs.SignedBeaconBlock{Signature: "foo"})
				require.NoError(t, err)
				return blockBytes
			},
		},
	}

	for _, testSuite := range testSuites {
		t.Run(testSuite.consensusVersion, func(t *testing.T) {
			for _, testCase := range testCases {
				t.Run(testCase.name, func(t *testing.T) {
					ctrl := gomock.NewController(t)
					defer ctrl.Finish()

					ctx := context.Background()

					jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
					jsonRestHandler.EXPECT().Get(
						gomock.Any(),
						gomock.Any(),
						&abstractSignedBlockResponseJson{},
					).SetArg(
						2,
						abstractSignedBlockResponseJson{
							Version: testSuite.consensusVersion,
							Data:    testCase.generateData(testSuite.consensusVersion),
						},
					).Return(
						nil,
					).Times(1)

					beaconBlockConverter := testSuite.generateBeaconBlockConverter(ctrl, testCase.conversionError)
					validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler, beaconBlockConverter: beaconBlockConverter}
					streamBlocksClient := validatorClient.streamBlocks(ctx, &eth.StreamBlocksRequest{}, time.Millisecond*100)

					_, err := streamBlocksClient.Recv()
					assert.ErrorContains(t, fmt.Sprintf(testCase.expectedErrorMessage, testSuite.consensusVersion), err)
				})
			}
		})
	}

}

func TestStreamBlocks_Phase0Valid(t *testing.T) {
	testCases := []struct {
		name         string
		verifiedOnly bool
	}{
		{
			name:         "verified optional",
			verifiedOnly: false,
		},
		{
			name:         "verified only",
			verifiedOnly: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ctx := context.Background()

			signedBlockResponseJson := abstractSignedBlockResponseJson{}
			jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
			beaconBlockConverter := mock.NewMockBeaconBlockConverter(ctrl)

			// For the first call, return a block that satisfies the verifiedOnly condition. This block should be returned by the first Recv().
			// For the second call, return the same block as the previous one. This block shouldn't be returned by the second Recv().
			phase0BeaconBlock1 := testhelpers.GenerateJsonPhase0BeaconBlock()
			phase0BeaconBlock1.Slot = "1"
			signedBeaconBlockContainer1 := structs.SignedBeaconBlock{
				Message:   phase0BeaconBlock1,
				Signature: "0x01",
			}

			marshalledSignedBeaconBlockContainer1, err := json.Marshal(signedBeaconBlockContainer1)
			require.NoError(t, err)

			jsonRestHandler.EXPECT().Get(
				gomock.Any(),
				"/eth/v2/beacon/blocks/head",
				&signedBlockResponseJson,
			).Return(
				nil,
			).SetArg(
				2,
				abstractSignedBlockResponseJson{
					Version:             "phase0",
					ExecutionOptimistic: false,
					Data:                marshalledSignedBeaconBlockContainer1,
				},
			).Times(2)

			phase0ProtoBeaconBlock1 := testhelpers.GenerateProtoPhase0BeaconBlock()
			phase0ProtoBeaconBlock1.Slot = 1

			beaconBlockConverter.EXPECT().ConvertRESTPhase0BlockToProto(
				phase0BeaconBlock1,
			).Return(
				phase0ProtoBeaconBlock1,
				nil,
			).Times(2)

			// For the third call, return a block with a different slot than the previous one, but with the verifiedOnly condition not satisfied.
			// If verifiedOnly == false, this block will be returned by the second Recv(); otherwise, another block will be requested.
			phase0BeaconBlock2 := testhelpers.GenerateJsonPhase0BeaconBlock()
			phase0BeaconBlock2.Slot = "2"
			signedBeaconBlockContainer2 := structs.SignedBeaconBlock{
				Message:   phase0BeaconBlock2,
				Signature: "0x02",
			}

			marshalledSignedBeaconBlockContainer2, err := json.Marshal(signedBeaconBlockContainer2)
			require.NoError(t, err)

			jsonRestHandler.EXPECT().Get(
				gomock.Any(),
				"/eth/v2/beacon/blocks/head",
				&signedBlockResponseJson,
			).Return(
				nil,
			).SetArg(
				2,
				abstractSignedBlockResponseJson{
					Version:             "phase0",
					ExecutionOptimistic: true,
					Data:                marshalledSignedBeaconBlockContainer2,
				},
			).Times(1)

			phase0ProtoBeaconBlock2 := testhelpers.GenerateProtoPhase0BeaconBlock()
			phase0ProtoBeaconBlock2.Slot = 2

			beaconBlockConverter.EXPECT().ConvertRESTPhase0BlockToProto(
				phase0BeaconBlock2,
			).Return(
				phase0ProtoBeaconBlock2,
				nil,
			).Times(1)

			// The fourth call is only necessary when verifiedOnly == true since the previous block was optimistic
			if testCase.verifiedOnly {
				jsonRestHandler.EXPECT().Get(
					gomock.Any(),
					"/eth/v2/beacon/blocks/head",
					&signedBlockResponseJson,
				).Return(
					nil,
				).SetArg(
					2,
					abstractSignedBlockResponseJson{
						Version:             "phase0",
						ExecutionOptimistic: false,
						Data:                marshalledSignedBeaconBlockContainer2,
					},
				).Times(1)

				beaconBlockConverter.EXPECT().ConvertRESTPhase0BlockToProto(
					phase0BeaconBlock2,
				).Return(
					phase0ProtoBeaconBlock2,
					nil,
				).Times(1)
			}

			validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler, beaconBlockConverter: beaconBlockConverter}
			streamBlocksClient := validatorClient.streamBlocks(ctx, &eth.StreamBlocksRequest{VerifiedOnly: testCase.verifiedOnly}, time.Millisecond*100)

			// Get the first block
			streamBlocksResponse1, err := streamBlocksClient.Recv()
			require.NoError(t, err)

			expectedStreamBlocksResponse1 := &eth.StreamBlocksResponse{
				Block: &eth.StreamBlocksResponse_Phase0Block{
					Phase0Block: &eth.SignedBeaconBlock{
						Block:     phase0ProtoBeaconBlock1,
						Signature: []byte{1},
					},
				},
			}

			assert.DeepEqual(t, expectedStreamBlocksResponse1, streamBlocksResponse1)

			// Get the second block
			streamBlocksResponse2, err := streamBlocksClient.Recv()
			require.NoError(t, err)

			expectedStreamBlocksResponse2 := &eth.StreamBlocksResponse{
				Block: &eth.StreamBlocksResponse_Phase0Block{
					Phase0Block: &eth.SignedBeaconBlock{
						Block:     phase0ProtoBeaconBlock2,
						Signature: []byte{2},
					},
				},
			}

			assert.DeepEqual(t, expectedStreamBlocksResponse2, streamBlocksResponse2)
		})
	}
}

func TestStreamBlocks_AltairValid(t *testing.T) {
	testCases := []struct {
		name         string
		verifiedOnly bool
	}{
		{
			name:         "verified optional",
			verifiedOnly: false,
		},
		{
			name:         "verified only",
			verifiedOnly: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ctx := context.Background()

			signedBlockResponseJson := abstractSignedBlockResponseJson{}
			jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
			beaconBlockConverter := mock.NewMockBeaconBlockConverter(ctrl)

			// For the first call, return a block that satisfies the verifiedOnly condition. This block should be returned by the first Recv().
			// For the second call, return the same block as the previous one. This block shouldn't be returned by the second Recv().
			altairBeaconBlock1 := testhelpers.GenerateJsonAltairBeaconBlock()
			altairBeaconBlock1.Slot = "1"
			signedBeaconBlockContainer1 := structs.SignedBeaconBlockAltair{
				Message:   altairBeaconBlock1,
				Signature: "0x01",
			}

			marshalledSignedBeaconBlockContainer1, err := json.Marshal(signedBeaconBlockContainer1)
			require.NoError(t, err)

			jsonRestHandler.EXPECT().Get(
				gomock.Any(),
				"/eth/v2/beacon/blocks/head",
				&signedBlockResponseJson,
			).Return(
				nil,
			).SetArg(
				2,
				abstractSignedBlockResponseJson{
					Version:             "altair",
					ExecutionOptimistic: false,
					Data:                marshalledSignedBeaconBlockContainer1,
				},
			).Times(2)

			altairProtoBeaconBlock1 := testhelpers.GenerateProtoAltairBeaconBlock()
			altairProtoBeaconBlock1.Slot = 1

			beaconBlockConverter.EXPECT().ConvertRESTAltairBlockToProto(
				altairBeaconBlock1,
			).Return(
				altairProtoBeaconBlock1,
				nil,
			).Times(2)

			// For the third call, return a block with a different slot than the previous one, but with the verifiedOnly condition not satisfied.
			// If verifiedOnly == false, this block will be returned by the second Recv(); otherwise, another block will be requested.
			altairBeaconBlock2 := testhelpers.GenerateJsonAltairBeaconBlock()
			altairBeaconBlock2.Slot = "2"
			signedBeaconBlockContainer2 := structs.SignedBeaconBlockAltair{
				Message:   altairBeaconBlock2,
				Signature: "0x02",
			}

			marshalledSignedBeaconBlockContainer2, err := json.Marshal(signedBeaconBlockContainer2)
			require.NoError(t, err)

			jsonRestHandler.EXPECT().Get(
				gomock.Any(),
				"/eth/v2/beacon/blocks/head",
				&signedBlockResponseJson,
			).Return(
				nil,
			).SetArg(
				2,
				abstractSignedBlockResponseJson{
					Version:             "altair",
					ExecutionOptimistic: true,
					Data:                marshalledSignedBeaconBlockContainer2,
				},
			).Times(1)

			altairProtoBeaconBlock2 := testhelpers.GenerateProtoAltairBeaconBlock()
			altairProtoBeaconBlock2.Slot = 2

			beaconBlockConverter.EXPECT().ConvertRESTAltairBlockToProto(
				altairBeaconBlock2,
			).Return(
				altairProtoBeaconBlock2,
				nil,
			).Times(1)

			// The fourth call is only necessary when verifiedOnly == true since the previous block was optimistic
			if testCase.verifiedOnly {
				jsonRestHandler.EXPECT().Get(
					gomock.Any(),
					"/eth/v2/beacon/blocks/head",
					&signedBlockResponseJson,
				).Return(
					nil,
				).SetArg(
					2,
					abstractSignedBlockResponseJson{
						Version:             "altair",
						ExecutionOptimistic: false,
						Data:                marshalledSignedBeaconBlockContainer2,
					},
				).Times(1)

				beaconBlockConverter.EXPECT().ConvertRESTAltairBlockToProto(
					altairBeaconBlock2,
				).Return(
					altairProtoBeaconBlock2,
					nil,
				).Times(1)
			}

			validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler, beaconBlockConverter: beaconBlockConverter}
			streamBlocksClient := validatorClient.streamBlocks(ctx, &eth.StreamBlocksRequest{VerifiedOnly: testCase.verifiedOnly}, time.Millisecond*100)

			// Get the first block
			streamBlocksResponse1, err := streamBlocksClient.Recv()
			require.NoError(t, err)

			expectedStreamBlocksResponse1 := &eth.StreamBlocksResponse{
				Block: &eth.StreamBlocksResponse_AltairBlock{
					AltairBlock: &eth.SignedBeaconBlockAltair{
						Block:     altairProtoBeaconBlock1,
						Signature: []byte{1},
					},
				},
			}

			assert.DeepEqual(t, expectedStreamBlocksResponse1, streamBlocksResponse1)

			// Get the second block
			streamBlocksResponse2, err := streamBlocksClient.Recv()
			require.NoError(t, err)

			expectedStreamBlocksResponse2 := &eth.StreamBlocksResponse{
				Block: &eth.StreamBlocksResponse_AltairBlock{
					AltairBlock: &eth.SignedBeaconBlockAltair{
						Block:     altairProtoBeaconBlock2,
						Signature: []byte{2},
					},
				},
			}

			assert.DeepEqual(t, expectedStreamBlocksResponse2, streamBlocksResponse2)
		})
	}
}

func TestStreamBlocks_BellatrixValid(t *testing.T) {
	testCases := []struct {
		name         string
		verifiedOnly bool
	}{
		{
			name:         "verified optional",
			verifiedOnly: false,
		},
		{
			name:         "verified only",
			verifiedOnly: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ctx := context.Background()

			signedBlockResponseJson := abstractSignedBlockResponseJson{}
			jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
			beaconBlockConverter := mock.NewMockBeaconBlockConverter(ctrl)

			// For the first call, return a block that satisfies the verifiedOnly condition. This block should be returned by the first Recv().
			// For the second call, return the same block as the previous one. This block shouldn't be returned by the second Recv().
			bellatrixBeaconBlock1 := testhelpers.GenerateJsonBellatrixBeaconBlock()
			bellatrixBeaconBlock1.Slot = "1"
			signedBeaconBlockContainer1 := structs.SignedBeaconBlockBellatrix{
				Message:   bellatrixBeaconBlock1,
				Signature: "0x01",
			}

			marshalledSignedBeaconBlockContainer1, err := json.Marshal(signedBeaconBlockContainer1)
			require.NoError(t, err)

			jsonRestHandler.EXPECT().Get(
				gomock.Any(),
				"/eth/v2/beacon/blocks/head",
				&signedBlockResponseJson,
			).Return(
				nil,
			).SetArg(
				2,
				abstractSignedBlockResponseJson{
					Version:             "bellatrix",
					ExecutionOptimistic: false,
					Data:                marshalledSignedBeaconBlockContainer1,
				},
			).Times(2)

			bellatrixProtoBeaconBlock1 := testhelpers.GenerateProtoBellatrixBeaconBlock()
			bellatrixProtoBeaconBlock1.Slot = 1

			beaconBlockConverter.EXPECT().ConvertRESTBellatrixBlockToProto(
				bellatrixBeaconBlock1,
			).Return(
				bellatrixProtoBeaconBlock1,
				nil,
			).Times(2)

			// For the third call, return a block with a different slot than the previous one, but with the verifiedOnly condition not satisfied.
			// If verifiedOnly == false, this block will be returned by the second Recv(); otherwise, another block will be requested.
			bellatrixBeaconBlock2 := testhelpers.GenerateJsonBellatrixBeaconBlock()
			bellatrixBeaconBlock2.Slot = "2"
			signedBeaconBlockContainer2 := structs.SignedBeaconBlockBellatrix{
				Message:   bellatrixBeaconBlock2,
				Signature: "0x02",
			}

			marshalledSignedBeaconBlockContainer2, err := json.Marshal(signedBeaconBlockContainer2)
			require.NoError(t, err)

			jsonRestHandler.EXPECT().Get(
				gomock.Any(),
				"/eth/v2/beacon/blocks/head",
				&signedBlockResponseJson,
			).Return(
				nil,
			).SetArg(
				2,
				abstractSignedBlockResponseJson{
					Version:             "bellatrix",
					ExecutionOptimistic: true,
					Data:                marshalledSignedBeaconBlockContainer2,
				},
			).Times(1)

			bellatrixProtoBeaconBlock2 := testhelpers.GenerateProtoBellatrixBeaconBlock()
			bellatrixProtoBeaconBlock2.Slot = 2

			beaconBlockConverter.EXPECT().ConvertRESTBellatrixBlockToProto(
				bellatrixBeaconBlock2,
			).Return(
				bellatrixProtoBeaconBlock2,
				nil,
			).Times(1)

			// The fourth call is only necessary when verifiedOnly == true since the previous block was optimistic
			if testCase.verifiedOnly {
				jsonRestHandler.EXPECT().Get(
					gomock.Any(),
					"/eth/v2/beacon/blocks/head",
					&signedBlockResponseJson,
				).Return(
					nil,
				).SetArg(
					2,
					abstractSignedBlockResponseJson{
						Version:             "bellatrix",
						ExecutionOptimistic: false,
						Data:                marshalledSignedBeaconBlockContainer2,
					},
				).Times(1)

				beaconBlockConverter.EXPECT().ConvertRESTBellatrixBlockToProto(
					bellatrixBeaconBlock2,
				).Return(
					bellatrixProtoBeaconBlock2,
					nil,
				).Times(1)
			}

			validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler, beaconBlockConverter: beaconBlockConverter}
			streamBlocksClient := validatorClient.streamBlocks(ctx, &eth.StreamBlocksRequest{VerifiedOnly: testCase.verifiedOnly}, time.Millisecond*100)

			// Get the first block
			streamBlocksResponse1, err := streamBlocksClient.Recv()
			require.NoError(t, err)

			expectedStreamBlocksResponse1 := &eth.StreamBlocksResponse{
				Block: &eth.StreamBlocksResponse_BellatrixBlock{
					BellatrixBlock: &eth.SignedBeaconBlockBellatrix{
						Block:     bellatrixProtoBeaconBlock1,
						Signature: []byte{1},
					},
				},
			}

			assert.DeepEqual(t, expectedStreamBlocksResponse1, streamBlocksResponse1)

			// Get the second block
			streamBlocksResponse2, err := streamBlocksClient.Recv()
			require.NoError(t, err)

			expectedStreamBlocksResponse2 := &eth.StreamBlocksResponse{
				Block: &eth.StreamBlocksResponse_BellatrixBlock{
					BellatrixBlock: &eth.SignedBeaconBlockBellatrix{
						Block:     bellatrixProtoBeaconBlock2,
						Signature: []byte{2},
					},
				},
			}

			assert.DeepEqual(t, expectedStreamBlocksResponse2, streamBlocksResponse2)
		})
	}
}

func TestStreamBlocks_CapellaValid(t *testing.T) {
	testCases := []struct {
		name         string
		verifiedOnly bool
	}{
		{
			name:         "verified optional",
			verifiedOnly: false,
		},
		{
			name:         "verified only",
			verifiedOnly: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ctx := context.Background()

			signedBlockResponseJson := abstractSignedBlockResponseJson{}
			jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
			beaconBlockConverter := mock.NewMockBeaconBlockConverter(ctrl)

			// For the first call, return a block that satisfies the verifiedOnly condition. This block should be returned by the first Recv().
			// For the second call, return the same block as the previous one. This block shouldn't be returned by the second Recv().
			capellaBeaconBlock1 := testhelpers.GenerateJsonCapellaBeaconBlock()
			capellaBeaconBlock1.Slot = "1"
			signedBeaconBlockContainer1 := structs.SignedBeaconBlockCapella{
				Message:   capellaBeaconBlock1,
				Signature: "0x01",
			}

			marshalledSignedBeaconBlockContainer1, err := json.Marshal(signedBeaconBlockContainer1)
			require.NoError(t, err)

			jsonRestHandler.EXPECT().Get(
				gomock.Any(),
				"/eth/v2/beacon/blocks/head",
				&signedBlockResponseJson,
			).Return(
				nil,
			).SetArg(
				2,
				abstractSignedBlockResponseJson{
					Version:             "capella",
					ExecutionOptimistic: false,
					Data:                marshalledSignedBeaconBlockContainer1,
				},
			).Times(2)

			capellaProtoBeaconBlock1 := testhelpers.GenerateProtoCapellaBeaconBlock()
			capellaProtoBeaconBlock1.Slot = 1

			beaconBlockConverter.EXPECT().ConvertRESTCapellaBlockToProto(
				capellaBeaconBlock1,
			).Return(
				capellaProtoBeaconBlock1,
				nil,
			).Times(2)

			// For the third call, return a block with a different slot than the previous one, but with the verifiedOnly condition not satisfied.
			// If verifiedOnly == false, this block will be returned by the second Recv(); otherwise, another block will be requested.
			capellaBeaconBlock2 := testhelpers.GenerateJsonCapellaBeaconBlock()
			capellaBeaconBlock2.Slot = "2"
			signedBeaconBlockContainer2 := structs.SignedBeaconBlockCapella{
				Message:   capellaBeaconBlock2,
				Signature: "0x02",
			}

			marshalledSignedBeaconBlockContainer2, err := json.Marshal(signedBeaconBlockContainer2)
			require.NoError(t, err)

			jsonRestHandler.EXPECT().Get(
				gomock.Any(),
				"/eth/v2/beacon/blocks/head",
				&signedBlockResponseJson,
			).Return(
				nil,
			).SetArg(
				2,
				abstractSignedBlockResponseJson{
					Version:             "capella",
					ExecutionOptimistic: true,
					Data:                marshalledSignedBeaconBlockContainer2,
				},
			).Times(1)

			capellaProtoBeaconBlock2 := testhelpers.GenerateProtoCapellaBeaconBlock()
			capellaProtoBeaconBlock2.Slot = 2

			beaconBlockConverter.EXPECT().ConvertRESTCapellaBlockToProto(
				capellaBeaconBlock2,
			).Return(
				capellaProtoBeaconBlock2,
				nil,
			).Times(1)

			// The fourth call is only necessary when verifiedOnly == true since the previous block was optimistic
			if testCase.verifiedOnly {
				jsonRestHandler.EXPECT().Get(
					gomock.Any(),
					"/eth/v2/beacon/blocks/head",
					&signedBlockResponseJson,
				).Return(
					nil,
				).SetArg(
					2,
					abstractSignedBlockResponseJson{
						Version:             "capella",
						ExecutionOptimistic: false,
						Data:                marshalledSignedBeaconBlockContainer2,
					},
				).Times(1)

				beaconBlockConverter.EXPECT().ConvertRESTCapellaBlockToProto(
					capellaBeaconBlock2,
				).Return(
					capellaProtoBeaconBlock2,
					nil,
				).Times(1)
			}

			validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler, beaconBlockConverter: beaconBlockConverter}
			streamBlocksClient := validatorClient.streamBlocks(ctx, &eth.StreamBlocksRequest{VerifiedOnly: testCase.verifiedOnly}, time.Millisecond*100)

			// Get the first block
			streamBlocksResponse1, err := streamBlocksClient.Recv()
			require.NoError(t, err)

			expectedStreamBlocksResponse1 := &eth.StreamBlocksResponse{
				Block: &eth.StreamBlocksResponse_CapellaBlock{
					CapellaBlock: &eth.SignedBeaconBlockCapella{
						Block:     capellaProtoBeaconBlock1,
						Signature: []byte{1},
					},
				},
			}

			assert.DeepEqual(t, expectedStreamBlocksResponse1, streamBlocksResponse1)

			// Get the second block
			streamBlocksResponse2, err := streamBlocksClient.Recv()
			require.NoError(t, err)

			expectedStreamBlocksResponse2 := &eth.StreamBlocksResponse{
				Block: &eth.StreamBlocksResponse_CapellaBlock{
					CapellaBlock: &eth.SignedBeaconBlockCapella{
						Block:     capellaProtoBeaconBlock2,
						Signature: []byte{2},
					},
				},
			}

			assert.DeepEqual(t, expectedStreamBlocksResponse2, streamBlocksResponse2)
		})
	}
}

func TestStreamBlocks_DenebValid(t *testing.T) {
	testCases := []struct {
		name         string
		verifiedOnly bool
	}{
		{
			name:         "verified optional",
			verifiedOnly: false,
		},
		{
			name:         "verified only",
			verifiedOnly: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ctx := context.Background()

			signedBlockResponseJson := abstractSignedBlockResponseJson{}
			jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
			beaconBlockConverter := mock.NewMockBeaconBlockConverter(ctrl)

			// For the first call, return a block that satisfies the verifiedOnly condition. This block should be returned by the first Recv().
			// For the second call, return the same block as the previous one. This block shouldn't be returned by the second Recv().
			var blockContents structs.SignedBeaconBlockContentsDeneb
			err := json.Unmarshal([]byte(rpctesting.DenebBlockContents), &blockContents)
			require.NoError(t, err)
			sig := "0x1b66ac1fb663c9bc59509846d6ec05345bd908eda73e670af888da41af171505cc411d61252fb6cb3fa0017b679f8bb2305b26a285fa2737f175668d0dff91cc1b66ac1fb663c9bc59509846d6ec05345bd908eda73e670af888da41af171505"
			denebBlock := blockContents.SignedBlock
			denebBlock.Message.Slot = "1"
			denebBlock.Signature = sig

			marshalledSignedBeaconBlockContainer1, err := json.Marshal(denebBlock)
			require.NoError(t, err)
			jsonRestHandler.EXPECT().Get(
				gomock.Any(),
				"/eth/v2/beacon/blocks/head",
				&signedBlockResponseJson,
			).Return(
				nil,
			).SetArg(
				2,
				abstractSignedBlockResponseJson{
					Version:             "deneb",
					ExecutionOptimistic: false,
					Data:                marshalledSignedBeaconBlockContainer1,
				},
			).Times(2)

			// For the third call, return a block with a different slot than the previous one, but with the verifiedOnly condition not satisfied.
			// If verifiedOnly == false, this block will be returned by the second Recv(); otherwise, another block will be requested.

			var blockContents2 structs.SignedBeaconBlockContentsDeneb
			err = json.Unmarshal([]byte(rpctesting.DenebBlockContents), &blockContents2)
			require.NoError(t, err)
			sig2 := "0x2b66ac1fb663c9bc59509846d6ec05345bd908eda73e670af888da41af171505cc411d61252fb6cb3fa0017b679f8bb2305b26a285fa2737f175668d0dff91cc1b66ac1fb663c9bc59509846d6ec05345bd908eda73e670af888da41af171505"
			denebBlock2 := blockContents.SignedBlock
			denebBlock2.Message.Slot = "2"
			denebBlock2.Signature = sig2

			marshalledSignedBeaconBlockContainer2, err := json.Marshal(denebBlock2)
			require.NoError(t, err)

			jsonRestHandler.EXPECT().Get(
				gomock.Any(),
				"/eth/v2/beacon/blocks/head",
				&signedBlockResponseJson,
			).Return(
				nil,
			).SetArg(
				2,
				abstractSignedBlockResponseJson{
					Version:             "deneb",
					ExecutionOptimistic: true,
					Data:                marshalledSignedBeaconBlockContainer2,
				},
			).Times(1)

			// The fourth call is only necessary when verifiedOnly == true since the previous block was optimistic
			if testCase.verifiedOnly {
				jsonRestHandler.EXPECT().Get(
					gomock.Any(),
					"/eth/v2/beacon/blocks/head",
					&signedBlockResponseJson,
				).Return(
					nil,
				).SetArg(
					2,
					abstractSignedBlockResponseJson{
						Version:             "deneb",
						ExecutionOptimistic: false,
						Data:                marshalledSignedBeaconBlockContainer2,
					},
				).Times(1)
			}

			validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler, beaconBlockConverter: beaconBlockConverter}
			streamBlocksClient := validatorClient.streamBlocks(ctx, &eth.StreamBlocksRequest{VerifiedOnly: testCase.verifiedOnly}, time.Millisecond*100)

			// Get the first block
			streamBlocksResponse1, err := streamBlocksClient.Recv()
			require.NoError(t, err)
			consensusBlock, err := denebBlock.Message.ToConsensus()
			consensusBlock.Slot = 1
			require.NoError(t, err)
			sigBytes, err := hexutil.Decode(sig)
			require.NoError(t, err)
			expectedStreamBlocksResponse1 := &eth.StreamBlocksResponse{
				Block: &eth.StreamBlocksResponse_DenebBlock{
					DenebBlock: &eth.SignedBeaconBlockDeneb{
						Block:     consensusBlock,
						Signature: sigBytes,
					},
				},
			}

			assert.DeepEqual(t, expectedStreamBlocksResponse1, streamBlocksResponse1)

			// Get the second block
			streamBlocksResponse2, err := streamBlocksClient.Recv()
			require.NoError(t, err)
			consensusBlock.Slot = 2
			sig2Bytes, err := hexutil.Decode(sig2)
			require.NoError(t, err)
			expectedStreamBlocksResponse2 := &eth.StreamBlocksResponse{
				Block: &eth.StreamBlocksResponse_DenebBlock{
					DenebBlock: &eth.SignedBeaconBlockDeneb{
						Block:     consensusBlock,
						Signature: sig2Bytes,
					},
				},
			}

			assert.DeepEqual(t, expectedStreamBlocksResponse2, streamBlocksResponse2)
		})
	}
}
