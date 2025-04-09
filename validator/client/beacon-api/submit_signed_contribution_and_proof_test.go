package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/prysmaticlabs/prysm/v5/api/server/structs"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/validator/client/beacon-api/mock"
	"go.uber.org/mock/gomock"
)

const submitSignedContributionAndProofTestEndpoint = "/eth/v1/validator/contribution_and_proofs"

func TestSubmitSignedContributionAndProof_Valid(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	jsonContributionAndProofs := []structs.SignedContributionAndProof{
		{
			Message: &structs.ContributionAndProof{
				AggregatorIndex: "1",
				Contribution: &structs.SyncCommitteeContribution{
					Slot:              "2",
					BeaconBlockRoot:   hexutil.Encode([]byte{3}),
					SubcommitteeIndex: "4",
					AggregationBits:   hexutil.Encode([]byte{5}),
					Signature:         hexutil.Encode([]byte{6}),
				},
				SelectionProof: hexutil.Encode([]byte{7}),
			},
			Signature: hexutil.Encode([]byte{8}),
		},
	}

	marshalledContributionAndProofs, err := json.Marshal(jsonContributionAndProofs)
	require.NoError(t, err)

	ctx := context.Background()

	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().Post(
		gomock.Any(),
		submitSignedContributionAndProofTestEndpoint,
		nil,
		bytes.NewBuffer(marshalledContributionAndProofs),
		nil,
	).Return(
		nil,
	).Times(1)

	contributionAndProof := &ethpb.SignedContributionAndProof{
		Message: &ethpb.ContributionAndProof{
			AggregatorIndex: 1,
			Contribution: &ethpb.SyncCommitteeContribution{
				Slot:              2,
				BlockRoot:         []byte{3},
				SubcommitteeIndex: 4,
				AggregationBits:   []byte{5},
				Signature:         []byte{6},
			},
			SelectionProof: []byte{7},
		},
		Signature: []byte{8},
	}

	validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	err = validatorClient.submitSignedContributionAndProof(ctx, contributionAndProof)
	require.NoError(t, err)
}

func TestSubmitSignedContributionAndProof_Error(t *testing.T) {
	testCases := []struct {
		name                 string
		data                 *ethpb.SignedContributionAndProof
		expectedErrorMessage string
		httpRequestExpected  bool
	}{
		{
			name:                 "nil signed contribution and proof",
			data:                 nil,
			expectedErrorMessage: "signed contribution and proof is nil",
		},
		{
			name:                 "nil message",
			data:                 &ethpb.SignedContributionAndProof{},
			expectedErrorMessage: "signed contribution and proof message is nil",
		},
		{
			name: "nil contribution",
			data: &ethpb.SignedContributionAndProof{
				Message: &ethpb.ContributionAndProof{},
			},
			expectedErrorMessage: "signed contribution and proof contribution is nil",
		},
		{
			name: "bad request",
			data: &ethpb.SignedContributionAndProof{
				Message: &ethpb.ContributionAndProof{
					Contribution: &ethpb.SyncCommitteeContribution{},
				},
			},
			httpRequestExpected:  true,
			expectedErrorMessage: "foo error",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ctx := context.Background()

			jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
			if testCase.httpRequestExpected {
				jsonRestHandler.EXPECT().Post(
					gomock.Any(),
					submitSignedContributionAndProofTestEndpoint,
					gomock.Any(),
					gomock.Any(),
					gomock.Any(),
				).Return(
					errors.New("foo error"),
				).Times(1)
			}

			validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
			err := validatorClient.submitSignedContributionAndProof(ctx, testCase.data)
			assert.ErrorContains(t, testCase.expectedErrorMessage, err)
		})
	}
}
