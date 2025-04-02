package validator_api

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/api/client/beacon"
	"github.com/prysmaticlabs/prysm/v5/api/client/beacon/mock"
	"github.com/prysmaticlabs/prysm/v5/api/client/beacon/validator_api/test_helpers"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"go.uber.org/mock/gomock"
)

func TestGetAggregatedSyncSelections(t *testing.T) {
	testcases := []struct {
		name                 string
		req                  []beacon.SyncCommitteeSelection
		res                  []beacon.SyncCommitteeSelection
		endpointError        error
		expectedErrorMessage string
	}{
		{
			name: "valid",
			req: []beacon.SyncCommitteeSelection{
				{
					SelectionProof:    test_helpers.FillByteSlice(96, 82),
					Slot:              75,
					ValidatorIndex:    76,
					SubcommitteeIndex: 77,
				},
			},
			res: []beacon.SyncCommitteeSelection{
				{
					SelectionProof:    test_helpers.FillByteSlice(96, 100),
					Slot:              75,
					ValidatorIndex:    76,
					SubcommitteeIndex: 77,
				},
			},
		},
		{
			name: "endpoint error",
			req: []beacon.SyncCommitteeSelection{
				{
					SelectionProof:    test_helpers.FillByteSlice(96, 82),
					Slot:              75,
					ValidatorIndex:    76,
					SubcommitteeIndex: 77,
				},
			},
			endpointError:        errors.New("bad request"),
			expectedErrorMessage: "bad request",
		},
		{
			name: "no response error",
			req: []beacon.SyncCommitteeSelection{
				{
					SelectionProof:    test_helpers.FillByteSlice(96, 82),
					Slot:              75,
					ValidatorIndex:    76,
					SubcommitteeIndex: 77,
				},
			},
			expectedErrorMessage: "no aggregated sync selections returned",
		},
		{
			name: "mismatch response",
			req: []beacon.SyncCommitteeSelection{
				{
					SelectionProof:    test_helpers.FillByteSlice(96, 82),
					Slot:              75,
					ValidatorIndex:    76,
					SubcommitteeIndex: 77,
				},
				{
					SelectionProof:    test_helpers.FillByteSlice(96, 100),
					Slot:              75,
					ValidatorIndex:    76,
					SubcommitteeIndex: 78,
				},
			},
			res: []beacon.SyncCommitteeSelection{
				{
					SelectionProof:    test_helpers.FillByteSlice(96, 100),
					Slot:              75,
					ValidatorIndex:    76,
					SubcommitteeIndex: 77,
				},
			},
			expectedErrorMessage: "mismatching number of sync selections",
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)

			reqBody, err := json.Marshal(test.req)
			require.NoError(t, err)

			ctx := context.Background()
			jsonRestHandler.EXPECT().Post(
				gomock.Any(),
				"/eth/v1/validator/sync_committee_selections",
				nil,
				bytes.NewBuffer(reqBody),
				&beacon.AggregatedSyncSelectionResponse{},
			).SetArg(
				4,
				beacon.AggregatedSyncSelectionResponse{Data: test.res},
			).Return(
				test.endpointError,
			).Times(1)

			validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
			res, err := validatorClient.AggregatedSyncSelections(ctx, test.req)
			if test.expectedErrorMessage != "" {
				require.ErrorContains(t, test.expectedErrorMessage, err)
				return
			}

			require.NoError(t, err)
			require.DeepEqual(t, test.res, res)
		})
	}
}
