package beacon_api

import (
	"bytes"
	"encoding/json"
	"errors"
	"strconv"
	"testing"

	"github.com/OffchainLabs/prysm/v6/api/server/structs"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/validator/client/beacon-api/mock"
	"go.uber.org/mock/gomock"
)

const subscribeCommitteeSubnetsTestEndpoint = "/eth/v1/validator/beacon_committee_subscriptions"

func TestSubscribeCommitteeSubnets_Valid(t *testing.T) {
	subscribeSlots := []primitives.Slot{0, 1, 100}
	validatorIndices := []primitives.ValidatorIndex{2, 3, 4}
	committeesAtSlot := []uint64{5, 6, 7}
	isAggregator := []bool{false, true, false}
	committeeIndices := []primitives.CommitteeIndex{8, 9, 10}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	jsonCommitteeSubscriptions := make([]*structs.BeaconCommitteeSubscription, len(subscribeSlots))
	for index := range jsonCommitteeSubscriptions {
		jsonCommitteeSubscriptions[index] = &structs.BeaconCommitteeSubscription{
			ValidatorIndex:   strconv.FormatUint(uint64(validatorIndices[index]), 10),
			CommitteeIndex:   strconv.FormatUint(uint64(committeeIndices[index]), 10),
			CommitteesAtSlot: strconv.FormatUint(committeesAtSlot[index], 10),
			Slot:             strconv.FormatUint(uint64(subscribeSlots[index]), 10),
			IsAggregator:     isAggregator[index],
		}
	}

	committeeSubscriptionsBytes, err := json.Marshal(jsonCommitteeSubscriptions)
	require.NoError(t, err)

	ctx := t.Context()

	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().Post(
		gomock.Any(),
		subscribeCommitteeSubnetsTestEndpoint,
		nil,
		bytes.NewBuffer(committeeSubscriptionsBytes),
		nil,
	).Return(
		nil,
	).Times(1)

	duties := make([]*structs.AttesterDuty, len(subscribeSlots))
	for index := range duties {
		duties[index] = &structs.AttesterDuty{
			ValidatorIndex:   strconv.FormatUint(uint64(validatorIndices[index]), 10),
			CommitteeIndex:   strconv.FormatUint(uint64(committeeIndices[index]), 10),
			CommitteesAtSlot: strconv.FormatUint(committeesAtSlot[index], 10),
			Slot:             strconv.FormatUint(uint64(subscribeSlots[index]), 10),
		}
	}

	validatorClient := &beaconApiValidatorClient{
		jsonRestHandler: jsonRestHandler,
	}
	err = validatorClient.subscribeCommitteeSubnets(
		ctx,
		&ethpb.CommitteeSubnetsSubscribeRequest{
			Slots:        subscribeSlots,
			CommitteeIds: committeeIndices,
			IsAggregator: isAggregator,
		},
		[]*ethpb.ValidatorDuty{
			{
				ValidatorIndex:   validatorIndices[0],
				CommitteesAtSlot: committeesAtSlot[0],
			},
			{
				ValidatorIndex:   validatorIndices[1],
				CommitteesAtSlot: committeesAtSlot[1],
			},
			{
				ValidatorIndex:   validatorIndices[2],
				CommitteesAtSlot: committeesAtSlot[2],
			},
		},
	)
	require.NoError(t, err)
}

func TestSubscribeCommitteeSubnets_Error(t *testing.T) {
	const arraySizeMismatchErrorMessage = "arrays `in.CommitteeIds`, `in.Slots`, `in.IsAggregator` and `duties` don't have the same length"

	testCases := []struct {
		name                    string
		subscribeRequest        *ethpb.CommitteeSubnetsSubscribeRequest
		duties                  []*ethpb.ValidatorDuty
		expectSubscribeRestCall bool
		expectedErrorMessage    string
	}{
		{
			name:                 "nil subscribe request",
			subscribeRequest:     nil,
			expectedErrorMessage: "committee subnets subscribe request is nil",
		},
		{
			name: "CommitteeIds size mismatch",
			subscribeRequest: &ethpb.CommitteeSubnetsSubscribeRequest{
				CommitteeIds: []primitives.CommitteeIndex{1},
				Slots:        []primitives.Slot{1, 2},
				IsAggregator: []bool{false, true},
			},
			duties: []*ethpb.ValidatorDuty{
				{
					ValidatorIndex:   1,
					CommitteesAtSlot: 1,
				},
				{
					ValidatorIndex:   2,
					CommitteesAtSlot: 2,
				},
			},
			expectedErrorMessage: arraySizeMismatchErrorMessage,
		},
		{
			name: "Slots size mismatch",
			subscribeRequest: &ethpb.CommitteeSubnetsSubscribeRequest{
				CommitteeIds: []primitives.CommitteeIndex{1, 2},
				Slots:        []primitives.Slot{1},
				IsAggregator: []bool{false, true},
			},
			duties: []*ethpb.ValidatorDuty{
				{
					ValidatorIndex:   1,
					CommitteesAtSlot: 1,
				},
				{
					ValidatorIndex:   2,
					CommitteesAtSlot: 2,
				},
			},
			expectedErrorMessage: arraySizeMismatchErrorMessage,
		},
		{
			name: "IsAggregator size mismatch",
			subscribeRequest: &ethpb.CommitteeSubnetsSubscribeRequest{
				CommitteeIds: []primitives.CommitteeIndex{1, 2},
				Slots:        []primitives.Slot{1, 2},
				IsAggregator: []bool{false},
			},
			duties: []*ethpb.ValidatorDuty{
				{
					ValidatorIndex:   1,
					CommitteesAtSlot: 1,
				},
				{
					ValidatorIndex:   2,
					CommitteesAtSlot: 2,
				},
			},
			expectedErrorMessage: arraySizeMismatchErrorMessage,
		},
		{
			name: "duties size mismatch",
			subscribeRequest: &ethpb.CommitteeSubnetsSubscribeRequest{
				CommitteeIds: []primitives.CommitteeIndex{1, 2},
				Slots:        []primitives.Slot{1, 2},
				IsAggregator: []bool{false, true},
			},
			duties: []*ethpb.ValidatorDuty{
				{
					ValidatorIndex:   1,
					CommitteesAtSlot: 1,
				},
			},
			expectedErrorMessage: arraySizeMismatchErrorMessage,
		},
		{
			name: "bad POST request",
			subscribeRequest: &ethpb.CommitteeSubnetsSubscribeRequest{
				Slots:        []primitives.Slot{1},
				CommitteeIds: []primitives.CommitteeIndex{2},
				IsAggregator: []bool{false},
			},
			duties: []*ethpb.ValidatorDuty{
				{
					ValidatorIndex:   1,
					CommitteesAtSlot: 1,
				},
			},
			expectSubscribeRestCall: true,
			expectedErrorMessage:    "foo error",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ctx := t.Context()

			jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
			if testCase.expectSubscribeRestCall {
				jsonRestHandler.EXPECT().Post(
					gomock.Any(),
					subscribeCommitteeSubnetsTestEndpoint,
					gomock.Any(),
					gomock.Any(),
					gomock.Any(),
				).Return(
					errors.New("foo error"),
				).Times(1)
			}

			validatorClient := &beaconApiValidatorClient{
				jsonRestHandler: jsonRestHandler,
			}
			err := validatorClient.subscribeCommitteeSubnets(ctx, testCase.subscribeRequest, testCase.duties)
			assert.ErrorContains(t, testCase.expectedErrorMessage, err)
		})
	}
}
