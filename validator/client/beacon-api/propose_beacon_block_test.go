package beacon_api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/OffchainLabs/prysm/v6/api/server/structs"
	rpctesting "github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/eth/shared/testing"
	"github.com/OffchainLabs/prysm/v6/network/httputil"
	engine "github.com/OffchainLabs/prysm/v6/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/validator/client/beacon-api/mock"
	testhelpers "github.com/OffchainLabs/prysm/v6/validator/client/beacon-api/test-helpers"
	"go.uber.org/mock/gomock"
)

func TestProposeBeaconBlock_SSZ_Error(t *testing.T) {
	testSuites := []struct {
		name                 string
		returnedError        error
		expectedErrorMessage string
	}{
		{
			name:                 "error 500",
			expectedErrorMessage: "failed to submit block ssz",
			returnedError: &httputil.DefaultJsonError{
				Code:    http.StatusInternalServerError,
				Message: "failed to submit block ssz",
			},
		},
		{
			name:                 "other error",
			expectedErrorMessage: "failed to submit block ssz",
			returnedError:        errors.New("failed to submit block ssz"),
		},
	}

	testCases := []struct {
		name             string
		consensusVersion string
		endpoint         string
		block            *ethpb.GenericSignedBeaconBlock
	}{
		{
			name:             "phase0",
			consensusVersion: "phase0",
			endpoint:         "/eth/v2/beacon/blocks",
			block: &ethpb.GenericSignedBeaconBlock{
				Block: generateSignedPhase0Block(),
			},
		},
		{
			name:             "altair",
			consensusVersion: "altair",
			endpoint:         "/eth/v2/beacon/blocks",
			block: &ethpb.GenericSignedBeaconBlock{
				Block: generateSignedAltairBlock(),
			},
		},
		{
			name:             "bellatrix",
			consensusVersion: "bellatrix",
			endpoint:         "/eth/v2/beacon/blocks",
			block: &ethpb.GenericSignedBeaconBlock{
				Block: generateSignedBellatrixBlock(),
			},
		},
		{
			name:             "blinded bellatrix",
			consensusVersion: "bellatrix",
			endpoint:         "/eth/v2/beacon/blinded_blocks",
			block: &ethpb.GenericSignedBeaconBlock{
				Block: generateSignedBlindedBellatrixBlock(),
			},
		},
		{
			name:             "capella",
			consensusVersion: "capella",
			endpoint:         "/eth/v2/beacon/blocks",
			block: &ethpb.GenericSignedBeaconBlock{
				Block: generateSignedCapellaBlock(),
			},
		},
		{
			name:             "blinded capella",
			consensusVersion: "capella",
			endpoint:         "/eth/v2/beacon/blinded_blocks",
			block: &ethpb.GenericSignedBeaconBlock{
				Block: generateSignedBlindedCapellaBlock(),
			},
		},
	}

	for _, testSuite := range testSuites {
		for _, testCase := range testCases {
			t.Run(testSuite.name+"/"+testCase.name, func(t *testing.T) {
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()

				ctx := t.Context()
				jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)

				// Expect PostSSZ to be called first with SSZ data
				headers := map[string]string{
					"Eth-Consensus-Version": testCase.consensusVersion,
				}
				jsonRestHandler.EXPECT().PostSSZ(
					gomock.Any(),
					testCase.endpoint,
					headers,
					gomock.Any(),
				).Return(
					nil, nil, testSuite.returnedError,
				).Times(1)

				// No JSON fallback expected for non-406 errors

				validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
				_, err := validatorClient.proposeBeaconBlock(ctx, testCase.block)
				assert.ErrorContains(t, testSuite.expectedErrorMessage, err)
			})
		}
	}
}

func TestProposeBeaconBlock_UnsupportedBlockType(t *testing.T) {
	validatorClient := &beaconApiValidatorClient{}
	_, err := validatorClient.proposeBeaconBlock(t.Context(), &ethpb.GenericSignedBeaconBlock{})
	assert.ErrorContains(t, "unsupported block type", err)
}

func TestProposeBeaconBlock_SSZSuccess_NoFallback(t *testing.T) {
	testCases := []struct {
		name             string
		consensusVersion string
		endpoint         string
		block            *ethpb.GenericSignedBeaconBlock
	}{
		{
			name:             "phase0",
			consensusVersion: "phase0",
			endpoint:         "/eth/v2/beacon/blocks",
			block: &ethpb.GenericSignedBeaconBlock{
				Block: generateSignedPhase0Block(),
			},
		},
		{
			name:             "altair",
			consensusVersion: "altair",
			endpoint:         "/eth/v2/beacon/blocks",
			block: &ethpb.GenericSignedBeaconBlock{
				Block: generateSignedAltairBlock(),
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ctx := t.Context()
			jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)

			// Expect PostSSZ to be called and succeed
			headers := map[string]string{
				"Eth-Consensus-Version": testCase.consensusVersion,
			}
			jsonRestHandler.EXPECT().PostSSZ(
				gomock.Any(),
				testCase.endpoint,
				headers,
				gomock.Any(),
			).Return(
				nil, nil, nil,
			).Times(1)

			// Post should NOT be called when PostSSZ succeeds
			jsonRestHandler.EXPECT().Post(
				gomock.Any(),
				gomock.Any(),
				gomock.Any(),
				gomock.Any(),
				gomock.Any(),
			).Times(0)

			validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
			_, err := validatorClient.proposeBeaconBlock(ctx, testCase.block)
			assert.NoError(t, err)
		})
	}
}

func TestProposeBeaconBlock_NewerTypes_SSZMarshal(t *testing.T) {
	t.Run("deneb", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)

		var blockContents structs.SignedBeaconBlockContentsDeneb
		err := json.Unmarshal([]byte(rpctesting.DenebBlockContents), &blockContents)
		require.NoError(t, err)
		genericSignedBlock, err := blockContents.ToGeneric()
		require.NoError(t, err)

		denebBytes, err := genericSignedBlock.GetDeneb().MarshalSSZ()
		require.NoError(t, err)

		jsonRestHandler.EXPECT().PostSSZ(
			gomock.Any(),
			"/eth/v2/beacon/blocks",
			gomock.Any(),
			bytes.NewBuffer(denebBytes),
		)

		validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
		proposeResponse, err := validatorClient.proposeBeaconBlock(t.Context(), genericSignedBlock)
		assert.NoError(t, err)
		require.NotNil(t, proposeResponse)

		expectedBlockRoot, err := genericSignedBlock.GetDeneb().Block.HashTreeRoot()
		require.NoError(t, err)
		assert.DeepEqual(t, expectedBlockRoot[:], proposeResponse.BlockRoot)
	})

	t.Run("blinded_deneb", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)

		var blindedBlock structs.SignedBlindedBeaconBlockDeneb
		err := json.Unmarshal([]byte(rpctesting.BlindedDenebBlock), &blindedBlock)
		require.NoError(t, err)
		genericSignedBlock, err := blindedBlock.ToGeneric()
		require.NoError(t, err)

		blindedDenebBytes, err := genericSignedBlock.GetBlindedDeneb().MarshalSSZ()
		require.NoError(t, err)

		jsonRestHandler.EXPECT().PostSSZ(
			gomock.Any(),
			"/eth/v2/beacon/blinded_blocks",
			gomock.Any(),
			bytes.NewBuffer(blindedDenebBytes),
		)

		validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
		proposeResponse, err := validatorClient.proposeBeaconBlock(t.Context(), genericSignedBlock)
		assert.NoError(t, err)
		require.NotNil(t, proposeResponse)

		expectedBlockRoot, err := genericSignedBlock.GetBlindedDeneb().HashTreeRoot()
		require.NoError(t, err)
		assert.DeepEqual(t, expectedBlockRoot[:], proposeResponse.BlockRoot)
	})

	t.Run("electra", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)

		var blockContents structs.SignedBeaconBlockContentsElectra
		err := json.Unmarshal([]byte(rpctesting.ElectraBlockContents), &blockContents)
		require.NoError(t, err)
		genericSignedBlock, err := blockContents.ToGeneric()
		require.NoError(t, err)

		electraBytes, err := genericSignedBlock.GetElectra().MarshalSSZ()
		require.NoError(t, err)

		jsonRestHandler.EXPECT().PostSSZ(
			gomock.Any(),
			"/eth/v2/beacon/blocks",
			gomock.Any(),
			bytes.NewBuffer(electraBytes),
		)

		validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
		proposeResponse, err := validatorClient.proposeBeaconBlock(t.Context(), genericSignedBlock)
		assert.NoError(t, err)
		require.NotNil(t, proposeResponse)

		expectedBlockRoot, err := genericSignedBlock.GetElectra().Block.HashTreeRoot()
		require.NoError(t, err)
		assert.DeepEqual(t, expectedBlockRoot[:], proposeResponse.BlockRoot)
	})

	t.Run("blinded_electra", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)

		var blindedBlock structs.SignedBlindedBeaconBlockElectra
		err := json.Unmarshal([]byte(rpctesting.BlindedElectraBlock), &blindedBlock)
		require.NoError(t, err)
		genericSignedBlock, err := blindedBlock.ToGeneric()
		require.NoError(t, err)

		blindedElectraBytes, err := genericSignedBlock.GetBlindedElectra().MarshalSSZ()
		require.NoError(t, err)

		jsonRestHandler.EXPECT().PostSSZ(
			gomock.Any(),
			"/eth/v2/beacon/blinded_blocks",
			gomock.Any(),
			bytes.NewBuffer(blindedElectraBytes),
		)

		validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
		proposeResponse, err := validatorClient.proposeBeaconBlock(t.Context(), genericSignedBlock)
		assert.NoError(t, err)
		require.NotNil(t, proposeResponse)

		expectedBlockRoot, err := genericSignedBlock.GetBlindedElectra().HashTreeRoot()
		require.NoError(t, err)
		assert.DeepEqual(t, expectedBlockRoot[:], proposeResponse.BlockRoot)
	})

	t.Run("fulu", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)

		var blockContents structs.SignedBeaconBlockContentsFulu
		err := json.Unmarshal([]byte(rpctesting.FuluBlockContents), &blockContents)
		require.NoError(t, err)
		genericSignedBlock, err := blockContents.ToGeneric()
		require.NoError(t, err)

		fuluBytes, err := genericSignedBlock.GetFulu().MarshalSSZ()
		require.NoError(t, err)

		jsonRestHandler.EXPECT().PostSSZ(
			gomock.Any(),
			"/eth/v2/beacon/blocks",
			gomock.Any(),
			bytes.NewBuffer(fuluBytes),
		)

		validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
		proposeResponse, err := validatorClient.proposeBeaconBlock(t.Context(), genericSignedBlock)
		assert.NoError(t, err)
		require.NotNil(t, proposeResponse)

		expectedBlockRoot, err := genericSignedBlock.GetFulu().Block.HashTreeRoot()
		require.NoError(t, err)
		assert.DeepEqual(t, expectedBlockRoot[:], proposeResponse.BlockRoot)
	})

	t.Run("blinded_fulu", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)

		var blindedBlock structs.SignedBlindedBeaconBlockFulu
		err := json.Unmarshal([]byte(rpctesting.BlindedFuluBlock), &blindedBlock)
		require.NoError(t, err)
		genericSignedBlock, err := blindedBlock.ToGeneric()
		require.NoError(t, err)

		blindedFuluBytes, err := genericSignedBlock.GetBlindedFulu().MarshalSSZ()
		require.NoError(t, err)

		jsonRestHandler.EXPECT().PostSSZ(
			gomock.Any(),
			"/eth/v2/beacon/blinded_blocks",
			gomock.Any(),
			bytes.NewBuffer(blindedFuluBytes),
		)

		validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
		proposeResponse, err := validatorClient.proposeBeaconBlock(t.Context(), genericSignedBlock)
		assert.NoError(t, err)
		require.NotNil(t, proposeResponse)

		expectedBlockRoot, err := genericSignedBlock.GetBlindedFulu().HashTreeRoot()
		require.NoError(t, err)
		assert.DeepEqual(t, expectedBlockRoot[:], proposeResponse.BlockRoot)
	})
}

// Generator functions for test blocks
func generateSignedPhase0Block() *ethpb.GenericSignedBeaconBlock_Phase0 {
	return &ethpb.GenericSignedBeaconBlock_Phase0{
		Phase0: &ethpb.SignedBeaconBlock{
			Block:     testhelpers.GenerateProtoPhase0BeaconBlock(),
			Signature: testhelpers.FillByteSlice(96, 110),
		},
	}
}

func generateSignedAltairBlock() *ethpb.GenericSignedBeaconBlock_Altair {
	return &ethpb.GenericSignedBeaconBlock_Altair{
		Altair: &ethpb.SignedBeaconBlockAltair{
			Block:     testhelpers.GenerateProtoAltairBeaconBlock(),
			Signature: testhelpers.FillByteSlice(96, 112),
		},
	}
}

func generateSignedBellatrixBlock() *ethpb.GenericSignedBeaconBlock_Bellatrix {
	return &ethpb.GenericSignedBeaconBlock_Bellatrix{
		Bellatrix: &ethpb.SignedBeaconBlockBellatrix{
			Block:     testhelpers.GenerateProtoBellatrixBeaconBlock(),
			Signature: testhelpers.FillByteSlice(96, 127),
		},
	}
}

func generateSignedBlindedBellatrixBlock() *ethpb.GenericSignedBeaconBlock_BlindedBellatrix {
	return &ethpb.GenericSignedBeaconBlock_BlindedBellatrix{
		BlindedBellatrix: &ethpb.SignedBlindedBeaconBlockBellatrix{
			Block: &ethpb.BlindedBeaconBlockBellatrix{
				Slot:          1,
				ProposerIndex: 2,
				ParentRoot:    testhelpers.FillByteSlice(32, 3),
				StateRoot:     testhelpers.FillByteSlice(32, 4),
				Body: &ethpb.BlindedBeaconBlockBodyBellatrix{
					RandaoReveal: testhelpers.FillByteSlice(96, 5),
					Eth1Data: &ethpb.Eth1Data{
						DepositRoot:  testhelpers.FillByteSlice(32, 6),
						DepositCount: 7,
						BlockHash:    testhelpers.FillByteSlice(32, 8),
					},
					Graffiti: testhelpers.FillByteSlice(32, 9),
					ProposerSlashings: []*ethpb.ProposerSlashing{
						{
							Header_1: &ethpb.SignedBeaconBlockHeader{
								Header: &ethpb.BeaconBlockHeader{
									Slot:          10,
									ProposerIndex: 11,
									ParentRoot:    testhelpers.FillByteSlice(32, 12),
									StateRoot:     testhelpers.FillByteSlice(32, 13),
									BodyRoot:      testhelpers.FillByteSlice(32, 14),
								},
								Signature: testhelpers.FillByteSlice(96, 15),
							},
							Header_2: &ethpb.SignedBeaconBlockHeader{
								Header: &ethpb.BeaconBlockHeader{
									Slot:          16,
									ProposerIndex: 17,
									ParentRoot:    testhelpers.FillByteSlice(32, 18),
									StateRoot:     testhelpers.FillByteSlice(32, 19),
									BodyRoot:      testhelpers.FillByteSlice(32, 20),
								},
								Signature: testhelpers.FillByteSlice(96, 21),
							},
						},
					},
					AttesterSlashings: []*ethpb.AttesterSlashing{},
					Attestations:      []*ethpb.Attestation{},
					Deposits:          []*ethpb.Deposit{},
					VoluntaryExits:    []*ethpb.SignedVoluntaryExit{},
					SyncAggregate: &ethpb.SyncAggregate{
						SyncCommitteeBits:      testhelpers.FillByteSlice(64, 100),
						SyncCommitteeSignature: testhelpers.FillByteSlice(96, 101),
					},
					ExecutionPayloadHeader: &engine.ExecutionPayloadHeader{
						ParentHash:       testhelpers.FillByteSlice(32, 102),
						FeeRecipient:     testhelpers.FillByteSlice(20, 103),
						StateRoot:        testhelpers.FillByteSlice(32, 104),
						ReceiptsRoot:     testhelpers.FillByteSlice(32, 105),
						LogsBloom:        testhelpers.FillByteSlice(256, 106),
						PrevRandao:       testhelpers.FillByteSlice(32, 107),
						BlockNumber:      108,
						GasLimit:         109,
						GasUsed:          110,
						Timestamp:        111,
						ExtraData:        testhelpers.FillByteSlice(32, 112),
						BaseFeePerGas:    testhelpers.FillByteSlice(32, 113),
						BlockHash:        testhelpers.FillByteSlice(32, 114),
						TransactionsRoot: testhelpers.FillByteSlice(32, 115),
					},
				},
			},
			Signature: testhelpers.FillByteSlice(96, 116),
		},
	}
}

func generateSignedCapellaBlock() *ethpb.GenericSignedBeaconBlock_Capella {
	return &ethpb.GenericSignedBeaconBlock_Capella{
		Capella: &ethpb.SignedBeaconBlockCapella{
			Block:     testhelpers.GenerateProtoCapellaBeaconBlock(),
			Signature: testhelpers.FillByteSlice(96, 127),
		},
	}
}

func generateSignedBlindedCapellaBlock() *ethpb.GenericSignedBeaconBlock_BlindedCapella {
	return &ethpb.GenericSignedBeaconBlock_BlindedCapella{
		BlindedCapella: &ethpb.SignedBlindedBeaconBlockCapella{
			Block: &ethpb.BlindedBeaconBlockCapella{
				Slot:          1,
				ProposerIndex: 2,
				ParentRoot:    testhelpers.FillByteSlice(32, 3),
				StateRoot:     testhelpers.FillByteSlice(32, 4),
				Body: &ethpb.BlindedBeaconBlockBodyCapella{
					RandaoReveal: testhelpers.FillByteSlice(96, 5),
					Eth1Data: &ethpb.Eth1Data{
						DepositRoot:  testhelpers.FillByteSlice(32, 6),
						DepositCount: 7,
						BlockHash:    testhelpers.FillByteSlice(32, 8),
					},
					Graffiti: testhelpers.FillByteSlice(32, 9),
					ProposerSlashings: []*ethpb.ProposerSlashing{
						{
							Header_1: &ethpb.SignedBeaconBlockHeader{
								Header: &ethpb.BeaconBlockHeader{
									Slot:          10,
									ProposerIndex: 11,
									ParentRoot:    testhelpers.FillByteSlice(32, 12),
									StateRoot:     testhelpers.FillByteSlice(32, 13),
									BodyRoot:      testhelpers.FillByteSlice(32, 14),
								},
								Signature: testhelpers.FillByteSlice(96, 15),
							},
							Header_2: &ethpb.SignedBeaconBlockHeader{
								Header: &ethpb.BeaconBlockHeader{
									Slot:          16,
									ProposerIndex: 17,
									ParentRoot:    testhelpers.FillByteSlice(32, 18),
									StateRoot:     testhelpers.FillByteSlice(32, 19),
									BodyRoot:      testhelpers.FillByteSlice(32, 20),
								},
								Signature: testhelpers.FillByteSlice(96, 21),
							},
						},
					},
					AttesterSlashings: []*ethpb.AttesterSlashing{},
					Attestations:      []*ethpb.Attestation{},
					Deposits:          []*ethpb.Deposit{},
					VoluntaryExits:    []*ethpb.SignedVoluntaryExit{},
					SyncAggregate: &ethpb.SyncAggregate{
						SyncCommitteeBits:      testhelpers.FillByteSlice(64, 37),
						SyncCommitteeSignature: testhelpers.FillByteSlice(96, 38),
					},
					ExecutionPayloadHeader: &engine.ExecutionPayloadHeaderCapella{
						ParentHash:       testhelpers.FillByteSlice(32, 39),
						FeeRecipient:     testhelpers.FillByteSlice(20, 40),
						StateRoot:        testhelpers.FillByteSlice(32, 41),
						ReceiptsRoot:     testhelpers.FillByteSlice(32, 42),
						LogsBloom:        testhelpers.FillByteSlice(256, 43),
						PrevRandao:       testhelpers.FillByteSlice(32, 44),
						BlockNumber:      45,
						GasLimit:         46,
						GasUsed:          47,
						Timestamp:        48,
						ExtraData:        testhelpers.FillByteSlice(32, 49),
						BaseFeePerGas:    testhelpers.FillByteSlice(32, 50),
						BlockHash:        testhelpers.FillByteSlice(32, 51),
						TransactionsRoot: testhelpers.FillByteSlice(32, 52),
						WithdrawalsRoot:  testhelpers.FillByteSlice(32, 53),
					},
					BlsToExecutionChanges: []*ethpb.SignedBLSToExecutionChange{},
				},
			},
			Signature: testhelpers.FillByteSlice(96, 54),
		},
	}
}

func TestProposeBeaconBlock_SSZFails_406_FallbackToJSON(t *testing.T) {
	testCases := []struct {
		name             string
		consensusVersion string
		endpoint         string
		block            *ethpb.GenericSignedBeaconBlock
	}{
		{
			name:             "phase0",
			consensusVersion: "phase0",
			endpoint:         "/eth/v2/beacon/blocks",
			block: &ethpb.GenericSignedBeaconBlock{
				Block: generateSignedPhase0Block(),
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ctx := t.Context()
			jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)

			// Expect PostSSZ to be called first and fail
			jsonRestHandler.EXPECT().PostSSZ(
				gomock.Any(),
				testCase.endpoint,
				gomock.Any(),
				gomock.Any(),
			).Return(
				nil, nil, &httputil.DefaultJsonError{
					Code:    http.StatusNotAcceptable,
					Message: "SSZ not supported",
				},
			).Times(1)

			jsonRestHandler.EXPECT().Post(
				gomock.Any(),
				testCase.endpoint,
				gomock.Any(),
				gomock.Any(),
				nil,
			).Return(
				nil,
			).Times(1)

			validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
			_, err := validatorClient.proposeBeaconBlock(ctx, testCase.block)
			assert.NoError(t, err)
		})
	}
}

func TestProposeBeaconBlock_SSZFails_Non406_NoFallback(t *testing.T) {
	testCases := []struct {
		name             string
		consensusVersion string
		endpoint         string
		block            *ethpb.GenericSignedBeaconBlock
	}{
		{
			name:             "phase0",
			consensusVersion: "phase0",
			endpoint:         "/eth/v2/beacon/blocks",
			block: &ethpb.GenericSignedBeaconBlock{
				Block: generateSignedPhase0Block(),
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ctx := t.Context()
			jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)

			// Expect PostSSZ to be called first and fail with non-406 error
			sszHeaders := map[string]string{
				"Eth-Consensus-Version": testCase.consensusVersion,
			}
			jsonRestHandler.EXPECT().PostSSZ(
				gomock.Any(),
				testCase.endpoint,
				sszHeaders,
				gomock.Any(),
			).Return(
				nil, nil, &httputil.DefaultJsonError{
					Code:    http.StatusInternalServerError,
					Message: "Internal server error",
				},
			).Times(1)

			// Post should NOT be called for non-406 errors
			jsonRestHandler.EXPECT().Post(
				gomock.Any(),
				gomock.Any(),
				gomock.Any(),
				gomock.Any(),
				gomock.Any(),
			).Times(0)

			validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
			_, err := validatorClient.proposeBeaconBlock(ctx, testCase.block)
			require.ErrorContains(t, "Internal server error", err)
		})
	}
}
