package beacon_api

import (
	"context"
	"errors"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/prysmaticlabs/prysm/v5/api/server/structs"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/validator/client/beacon-api/mock"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestGetGenesis(t *testing.T) {
	testCases := []struct {
		name                    string
		genesisResponse         *structs.Genesis
		genesisError            error
		depositContractResponse structs.GetDepositContractResponse
		depositContractError    error
		queriesDepositContract  bool
		expectedResponse        *ethpb.Genesis
		expectedError           string
	}{
		{
			name:          "fails to get genesis",
			genesisError:  errors.New("foo error"),
			expectedError: "failed to get genesis: foo error",
		},
		{
			name: "fails to decode genesis validator root",
			genesisResponse: &structs.Genesis{
				GenesisTime:           "1",
				GenesisValidatorsRoot: "foo",
			},
			expectedError: "failed to decode genesis validator root `foo`",
		},
		{
			name: "fails to parse genesis time",
			genesisResponse: &structs.Genesis{
				GenesisTime:           "foo",
				GenesisValidatorsRoot: hexutil.Encode([]byte{1}),
			},
			expectedError: "failed to parse genesis time `foo`",
		},
		{
			name: "fails to query contract information",
			genesisResponse: &structs.Genesis{
				GenesisTime:           "1",
				GenesisValidatorsRoot: hexutil.Encode([]byte{2}),
			},
			depositContractError:   errors.New("foo error"),
			queriesDepositContract: true,
			expectedError:          "foo error",
		},
		{
			name: "fails to read nil deposit contract data",
			genesisResponse: &structs.Genesis{
				GenesisTime:           "1",
				GenesisValidatorsRoot: hexutil.Encode([]byte{2}),
			},
			queriesDepositContract: true,
			depositContractResponse: structs.GetDepositContractResponse{
				Data: nil,
			},
			expectedError: "deposit contract data is nil",
		},
		{
			name: "fails to decode deposit contract address",
			genesisResponse: &structs.Genesis{
				GenesisTime:           "1",
				GenesisValidatorsRoot: hexutil.Encode([]byte{2}),
			},
			queriesDepositContract: true,
			depositContractResponse: structs.GetDepositContractResponse{
				Data: &structs.DepositContractData{
					Address: "foo",
				},
			},
			expectedError: "failed to decode deposit contract address `foo`",
		},
		{
			name: "successfully retrieves genesis info",
			genesisResponse: &structs.Genesis{
				GenesisTime:           "654812",
				GenesisValidatorsRoot: hexutil.Encode([]byte{2}),
			},
			queriesDepositContract: true,
			depositContractResponse: structs.GetDepositContractResponse{
				Data: &structs.DepositContractData{
					Address: hexutil.Encode([]byte{3}),
				},
			},
			expectedResponse: &ethpb.Genesis{
				GenesisTime: &timestamppb.Timestamp{
					Seconds: 654812,
				},
				DepositContractAddress: []byte{3},
				GenesisValidatorsRoot:  []byte{2},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			ctx := context.Background()

			genesisProvider := mock.NewMockGenesisProvider(ctrl)
			genesisProvider.EXPECT().Genesis(
				gomock.Any(),
			).Return(
				testCase.genesisResponse,
				testCase.genesisError,
			)

			depositContractJson := structs.GetDepositContractResponse{}
			jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)

			if testCase.queriesDepositContract {
				jsonRestHandler.EXPECT().Get(
					gomock.Any(),
					"/eth/v1/config/deposit_contract",
					&depositContractJson,
				).Return(
					testCase.depositContractError,
				).SetArg(
					2,
					testCase.depositContractResponse,
				)
			}

			nodeClient := &beaconApiNodeClient{
				genesisProvider: genesisProvider,
				jsonRestHandler: jsonRestHandler,
			}
			response, err := nodeClient.Genesis(ctx, &emptypb.Empty{})

			if testCase.expectedResponse == nil {
				assert.ErrorContains(t, testCase.expectedError, err)
			} else {
				assert.DeepEqual(t, testCase.expectedResponse, response)
			}
		})
	}
}

func TestGetSyncStatus(t *testing.T) {
	const syncingEndpoint = "/eth/v1/node/syncing"

	testCases := []struct {
		name                 string
		restEndpointResponse structs.SyncStatusResponse
		restEndpointError    error
		expectedResponse     *ethpb.SyncStatus
		expectedError        string
	}{
		{
			name:              "fails to query REST endpoint",
			restEndpointError: errors.New("foo error"),
			expectedError:     "foo error",
		},
		{
			name:                 "returns nil syncing data",
			restEndpointResponse: structs.SyncStatusResponse{Data: nil},
			expectedError:        "syncing data is nil",
		},
		{
			name: "returns false syncing status",
			restEndpointResponse: structs.SyncStatusResponse{
				Data: &structs.SyncStatusResponseData{
					IsSyncing: false,
				},
			},
			expectedResponse: &ethpb.SyncStatus{
				Syncing: false,
			},
		},
		{
			name: "returns true syncing status",
			restEndpointResponse: structs.SyncStatusResponse{
				Data: &structs.SyncStatusResponseData{
					IsSyncing: true,
				},
			},
			expectedResponse: &ethpb.SyncStatus{
				Syncing: true,
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			ctx := context.Background()

			syncingResponse := structs.SyncStatusResponse{}
			jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
			jsonRestHandler.EXPECT().Get(
				gomock.Any(),
				syncingEndpoint,
				&syncingResponse,
			).Return(
				testCase.restEndpointError,
			).SetArg(
				2,
				testCase.restEndpointResponse,
			)

			nodeClient := &beaconApiNodeClient{jsonRestHandler: jsonRestHandler}
			syncStatus, err := nodeClient.SyncStatus(ctx, &emptypb.Empty{})

			if testCase.expectedResponse == nil {
				assert.ErrorContains(t, testCase.expectedError, err)
			} else {
				assert.DeepEqual(t, testCase.expectedResponse, syncStatus)
			}
		})
	}
}

func TestGetVersion(t *testing.T) {
	const versionEndpoint = "/eth/v1/node/version"

	testCases := []struct {
		name                 string
		restEndpointResponse structs.GetVersionResponse
		restEndpointError    error
		expectedResponse     *ethpb.Version
		expectedError        string
	}{
		{
			name:              "fails to query REST endpoint",
			restEndpointError: errors.New("foo error"),
			expectedError:     "foo error",
		},
		{
			name:                 "returns nil version data",
			restEndpointResponse: structs.GetVersionResponse{Data: nil},
			expectedError:        "empty version response",
		},
		{
			name: "returns proper version response",
			restEndpointResponse: structs.GetVersionResponse{
				Data: &structs.Version{
					Version: "prysm/local",
				},
			},
			expectedResponse: &ethpb.Version{
				Version: "prysm/local",
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			ctx := context.Background()

			var versionResponse structs.GetVersionResponse
			jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
			jsonRestHandler.EXPECT().Get(
				gomock.Any(),
				versionEndpoint,
				&versionResponse,
			).Return(
				testCase.restEndpointError,
			).SetArg(
				2,
				testCase.restEndpointResponse,
			)

			nodeClient := &beaconApiNodeClient{jsonRestHandler: jsonRestHandler}
			version, err := nodeClient.Version(ctx, &emptypb.Empty{})

			if testCase.expectedResponse == nil {
				assert.ErrorContains(t, testCase.expectedError, err)
			} else {
				assert.DeepEqual(t, testCase.expectedResponse, version)
			}
		})
	}
}
