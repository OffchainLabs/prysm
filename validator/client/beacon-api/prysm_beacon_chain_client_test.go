package beacon_api

import (
	"context"
	"errors"
	"testing"

	"github.com/prysmaticlabs/prysm/v5/api/server/structs"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/validator"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/validator/client/beacon-api/mock"
	"github.com/prysmaticlabs/prysm/v5/validator/client/iface"
	"go.uber.org/mock/gomock"
)

func TestGetValidatorCount(t *testing.T) {
	const nodeVersion = "prysm/v0.0.1"

	testCases := []struct {
		name                        string
		versionEndpointError        error
		validatorCountEndpointError error
		versionResponse             structs.GetVersionResponse
		validatorCountResponse      structs.GetValidatorCountResponse
		validatorCountCalled        int
		expectedResponse            []iface.ValidatorCount
		expectedError               string
	}{
		{
			name: "success",
			versionResponse: structs.GetVersionResponse{
				Data: &structs.Version{Version: nodeVersion},
			},
			validatorCountResponse: structs.GetValidatorCountResponse{
				ExecutionOptimistic: "false",
				Finalized:           "true",
				Data: []*structs.ValidatorCount{
					{
						Status: "active",
						Count:  "10",
					},
				},
			},
			validatorCountCalled: 1,
			expectedResponse: []iface.ValidatorCount{
				{
					Status: "active",
					Count:  10,
				},
			},
		},
		{
			name: "not supported beacon node",
			versionResponse: structs.GetVersionResponse{
				Data: &structs.Version{Version: "lighthouse/v0.0.1"},
			},
			expectedError: "endpoint not supported",
		},
		{
			name:                 "fails to get version",
			versionEndpointError: errors.New("foo error"),
			expectedError:        "failed to get node version",
		},
		{
			name: "fails to get validator count",
			versionResponse: structs.GetVersionResponse{
				Data: &structs.Version{Version: nodeVersion},
			},
			validatorCountEndpointError: errors.New("foo error"),
			validatorCountCalled:        1,
			expectedError:               "foo error",
		},
		{
			name: "nil validator count data",
			versionResponse: structs.GetVersionResponse{
				Data: &structs.Version{Version: nodeVersion},
			},
			validatorCountResponse: structs.GetValidatorCountResponse{
				ExecutionOptimistic: "false",
				Finalized:           "true",
				Data:                nil,
			},
			validatorCountCalled: 1,
			expectedError:        "validator count data is nil",
		},
		{
			name: "invalid validator count",
			versionResponse: structs.GetVersionResponse{
				Data: &structs.Version{Version: nodeVersion},
			},
			validatorCountResponse: structs.GetValidatorCountResponse{
				ExecutionOptimistic: "false",
				Finalized:           "true",
				Data: []*structs.ValidatorCount{
					{
						Status: "active",
						Count:  "10",
					},
					{
						Status: "exited",
						Count:  "10",
					},
				},
			},
			validatorCountCalled: 1,
			expectedError:        "mismatch between validator count data and the number of statuses provided",
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ctx := context.Background()
			jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)

			// Expect node version endpoint call.
			var nodeVersionResponse structs.GetVersionResponse
			jsonRestHandler.EXPECT().Get(
				gomock.Any(),
				"/eth/v1/node/version",
				&nodeVersionResponse,
			).Return(
				test.versionEndpointError,
			).SetArg(
				2,
				test.versionResponse,
			)

			var validatorCountResponse structs.GetValidatorCountResponse
			jsonRestHandler.EXPECT().Get(
				gomock.Any(),
				"/eth/v1/beacon/states/head/validator_count?status=active",
				&validatorCountResponse,
			).Return(
				test.validatorCountEndpointError,
			).SetArg(
				2,
				test.validatorCountResponse,
			).Times(test.validatorCountCalled)

			// Type assertion.
			var client iface.PrysmChainClient = &prysmChainClient{
				nodeClient:      &beaconApiNodeClient{jsonRestHandler: jsonRestHandler},
				jsonRestHandler: jsonRestHandler,
			}

			countResponse, err := client.ValidatorCount(ctx, "head", []validator.Status{validator.Active})

			if len(test.expectedResponse) == 0 {
				require.ErrorContains(t, test.expectedError, err)
			} else {
				require.NoError(t, err)
				require.DeepEqual(t, test.expectedResponse, countResponse)
			}
		})
	}

}
