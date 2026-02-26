package beacon_api

import (
	"bytes"
	"encoding/json"
	"errors"
	"strconv"
	"testing"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/validator/client/beacon-api/mock"
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

	handler := mock.NewMockJsonRestHandler(ctrl)
	handler.EXPECT().Post(
		gomock.Any(),
		subscribeCommitteeSubnetsTestEndpoint,
		nil,
		bytes.NewBuffer(committeeSubscriptionsBytes),
		nil,
	).Return(
		nil,
	).Times(1)

	validatorClient := &beaconApiValidatorClient{
		handler: handler,
	}
	err = validatorClient.subscribeCommitteeSubnets(
		ctx,
		&ethpb.CommitteeSubnetsSubscribeRequest{
			Slots:        subscribeSlots,
			CommitteeIds: committeeIndices,
			IsAggregator: isAggregator,
		},
		validatorIndices,
		committeesAtSlot,
	)
	require.NoError(t, err)
}

func TestSubscribeCommitteeSubnets_Error(t *testing.T) {
	const arraySizeMismatchErrorMessage = "arrays `in.CommitteeIds`, `in.Slots`, `in.IsAggregator`, `validatorIndices` and `committeesAtSlot` don't have the same length"

	testCases := []struct {
		name                    string
		subscribeRequest        *ethpb.CommitteeSubnetsSubscribeRequest
		validatorIndices        []primitives.ValidatorIndex
		committeesAtSlot        []uint64
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
			validatorIndices:     []primitives.ValidatorIndex{1, 2},
			committeesAtSlot:     []uint64{1, 2},
			expectedErrorMessage: arraySizeMismatchErrorMessage,
		},
		{
			name: "Slots size mismatch",
			subscribeRequest: &ethpb.CommitteeSubnetsSubscribeRequest{
				CommitteeIds: []primitives.CommitteeIndex{1, 2},
				Slots:        []primitives.Slot{1},
				IsAggregator: []bool{false, true},
			},
			validatorIndices:     []primitives.ValidatorIndex{1, 2},
			committeesAtSlot:     []uint64{1, 2},
			expectedErrorMessage: arraySizeMismatchErrorMessage,
		},
		{
			name: "IsAggregator size mismatch",
			subscribeRequest: &ethpb.CommitteeSubnetsSubscribeRequest{
				CommitteeIds: []primitives.CommitteeIndex{1, 2},
				Slots:        []primitives.Slot{1, 2},
				IsAggregator: []bool{false},
			},
			validatorIndices:     []primitives.ValidatorIndex{1, 2},
			committeesAtSlot:     []uint64{1, 2},
			expectedErrorMessage: arraySizeMismatchErrorMessage,
		},
		{
			name: "validatorIndices size mismatch",
			subscribeRequest: &ethpb.CommitteeSubnetsSubscribeRequest{
				CommitteeIds: []primitives.CommitteeIndex{1, 2},
				Slots:        []primitives.Slot{1, 2},
				IsAggregator: []bool{false, true},
			},
			validatorIndices:     []primitives.ValidatorIndex{1},
			committeesAtSlot:     []uint64{1, 2},
			expectedErrorMessage: arraySizeMismatchErrorMessage,
		},
		{
			name: "bad POST request",
			subscribeRequest: &ethpb.CommitteeSubnetsSubscribeRequest{
				Slots:        []primitives.Slot{1},
				CommitteeIds: []primitives.CommitteeIndex{2},
				IsAggregator: []bool{false},
			},
			validatorIndices:        []primitives.ValidatorIndex{1},
			committeesAtSlot:        []uint64{1},
			expectSubscribeRestCall: true,
			expectedErrorMessage:    "foo error",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ctx := t.Context()

			handler := mock.NewMockJsonRestHandler(ctrl)
			if testCase.expectSubscribeRestCall {
				handler.EXPECT().Post(
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
				handler: handler,
			}
			err := validatorClient.subscribeCommitteeSubnets(ctx, testCase.subscribeRequest, testCase.validatorIndices, testCase.committeesAtSlot)
			assert.ErrorContains(t, testCase.expectedErrorMessage, err)
		})
	}
}
