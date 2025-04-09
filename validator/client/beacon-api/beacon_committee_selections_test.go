package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/validator/client/beacon-api/mock"
	testhelpers "github.com/prysmaticlabs/prysm/v5/validator/client/beacon-api/test-helpers"
	"github.com/prysmaticlabs/prysm/v5/validator/client/iface"
	"go.uber.org/mock/gomock"
)

func TestGetAggregatedSelections(t *testing.T) {
	testcases := []struct {
		name                 string
		req                  []iface.BeaconCommitteeSelection
		res                  []iface.BeaconCommitteeSelection
		endpointError        error
		expectedErrorMessage string
	}{
		{
			name: "valid",
			req: []iface.BeaconCommitteeSelection{
				{
					SelectionProof: testhelpers.FillByteSlice(96, 82),
					Slot:           75,
					ValidatorIndex: 76,
				},
			},
			res: []iface.BeaconCommitteeSelection{
				{
					SelectionProof: testhelpers.FillByteSlice(96, 100),
					Slot:           75,
					ValidatorIndex: 76,
				},
			},
		},
		{
			name: "endpoint error",
			req: []iface.BeaconCommitteeSelection{
				{
					SelectionProof: testhelpers.FillByteSlice(96, 82),
					Slot:           75,
					ValidatorIndex: 76,
				},
			},
			endpointError:        errors.New("bad request"),
			expectedErrorMessage: "bad request",
		},
		{
			name: "no response error",
			req: []iface.BeaconCommitteeSelection{
				{
					SelectionProof: testhelpers.FillByteSlice(96, 82),
					Slot:           75,
					ValidatorIndex: 76,
				},
			},
			expectedErrorMessage: "no aggregated selection returned",
		},
		{
			name: "mismatch response",
			req: []iface.BeaconCommitteeSelection{
				{
					SelectionProof: testhelpers.FillByteSlice(96, 82),
					Slot:           75,
					ValidatorIndex: 76,
				},
				{
					SelectionProof: testhelpers.FillByteSlice(96, 102),
					Slot:           75,
					ValidatorIndex: 79,
				},
			},
			res: []iface.BeaconCommitteeSelection{
				{
					SelectionProof: testhelpers.FillByteSlice(96, 100),
					Slot:           75,
					ValidatorIndex: 76,
				},
			},
			expectedErrorMessage: "mismatching number of selections",
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
				"/eth/v1/validator/beacon_committee_selections",
				nil,
				bytes.NewBuffer(reqBody),
				&aggregatedSelectionResponse{},
			).SetArg(
				4,
				aggregatedSelectionResponse{Data: test.res},
			).Return(
				test.endpointError,
			).Times(1)

			validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
			res, err := validatorClient.AggregatedSelections(ctx, test.req)
			if test.expectedErrorMessage != "" {
				require.ErrorContains(t, test.expectedErrorMessage, err)
				return
			}

			require.NoError(t, err)
			require.DeepEqual(t, test.res, res)
		})
	}
}
