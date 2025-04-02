package shared_providers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/prysmaticlabs/prysm/v5/api/client/beacon/mock"
	"github.com/prysmaticlabs/prysm/v5/api/server/structs"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"go.uber.org/mock/gomock"
)

const getAttesterDutiesTestEndpoint = "/eth/v1/validator/duties/attester"
const getProposerDutiesTestEndpoint = "/eth/v1/validator/duties/proposer"
const getSyncDutiesTestEndpoint = "/eth/v1/validator/duties/sync"
const getCommitteesTestEndpoint = "/eth/v1/beacon/states/head/committees"

func TestGetAttesterDuties_Valid(t *testing.T) {
	stringValidatorIndices := []string{"2", "9"}
	const epoch = primitives.Epoch(1)

	validatorIndicesBytes, err := json.Marshal(stringValidatorIndices)
	require.NoError(t, err)

	expectedAttesterDuties := structs.GetAttesterDutiesResponse{
		Data: []*structs.AttesterDuty{
			{
				Pubkey:                  hexutil.Encode([]byte{1}),
				ValidatorIndex:          "2",
				CommitteeIndex:          "3",
				CommitteeLength:         "4",
				CommitteesAtSlot:        "5",
				ValidatorCommitteeIndex: "6",
				Slot:                    "7",
			},
			{
				Pubkey:                  hexutil.Encode([]byte{8}),
				ValidatorIndex:          "9",
				CommitteeIndex:          "10",
				CommitteeLength:         "11",
				CommitteesAtSlot:        "12",
				ValidatorCommitteeIndex: "13",
				Slot:                    "14",
			},
		},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	validatorIndices := []primitives.ValidatorIndex{2, 9}
	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().Post(
		gomock.Any(),
		fmt.Sprintf("%s/%d", getAttesterDutiesTestEndpoint, epoch),
		nil,
		bytes.NewBuffer(validatorIndicesBytes),
		&structs.GetAttesterDutiesResponse{},
	).Return(
		nil,
	).SetArg(
		4,
		expectedAttesterDuties,
	).Times(1)

	dutiesProvider := &dutiesProvider{jsonRestHandler: jsonRestHandler}
	attesterDuties, err := dutiesProvider.AttesterDuties(ctx, epoch, validatorIndices)
	require.NoError(t, err)
	assert.DeepEqual(t, expectedAttesterDuties.Data, attesterDuties)
}

func TestGetAttesterDuties_HttpError(t *testing.T) {
	const epoch = primitives.Epoch(1)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().Post(
		gomock.Any(),
		fmt.Sprintf("%s/%d", getAttesterDutiesTestEndpoint, epoch),
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).Return(
		errors.New("foo error"),
	).Times(1)

	dutiesProvider := &dutiesProvider{jsonRestHandler: jsonRestHandler}
	_, err := dutiesProvider.AttesterDuties(ctx, epoch, nil)
	assert.ErrorContains(t, "foo error", err)
}

func TestGetAttesterDuties_NilAttesterDuty(t *testing.T) {
	const epoch = primitives.Epoch(1)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().Post(
		gomock.Any(),
		fmt.Sprintf("%s/%d", getAttesterDutiesTestEndpoint, epoch),
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).Return(
		nil,
	).SetArg(
		4,
		structs.GetAttesterDutiesResponse{
			Data: []*structs.AttesterDuty{nil},
		},
	).Times(1)

	dutiesProvider := &dutiesProvider{jsonRestHandler: jsonRestHandler}
	_, err := dutiesProvider.AttesterDuties(ctx, epoch, nil)
	assert.ErrorContains(t, "attester duty at index `0` is nil", err)
}

func TestGetProposerDuties_Valid(t *testing.T) {
	const epoch = primitives.Epoch(1)

	expectedProposerDuties := structs.GetProposerDutiesResponse{
		Data: []*structs.ProposerDuty{
			{
				Pubkey:         hexutil.Encode([]byte{1}),
				ValidatorIndex: "2",
				Slot:           "3",
			},
			{
				Pubkey:         hexutil.Encode([]byte{4}),
				ValidatorIndex: "5",
				Slot:           "6",
			},
		},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().Get(
		gomock.Any(),
		fmt.Sprintf("%s/%d", getProposerDutiesTestEndpoint, epoch),
		&structs.GetProposerDutiesResponse{},
	).Return(
		nil,
	).SetArg(
		2,
		expectedProposerDuties,
	).Times(1)

	dutiesProvider := &dutiesProvider{jsonRestHandler: jsonRestHandler}
	proposerDuties, err := dutiesProvider.ProposerDuties(ctx, epoch)
	require.NoError(t, err)
	assert.DeepEqual(t, expectedProposerDuties.Data, proposerDuties)
}

func TestGetProposerDuties_HttpError(t *testing.T) {
	const epoch = primitives.Epoch(1)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().Get(
		gomock.Any(),
		fmt.Sprintf("%s/%d", getProposerDutiesTestEndpoint, epoch),
		gomock.Any(),
	).Return(
		errors.New("foo error"),
	).Times(1)

	dutiesProvider := &dutiesProvider{jsonRestHandler: jsonRestHandler}
	_, err := dutiesProvider.ProposerDuties(ctx, epoch)
	assert.ErrorContains(t, "foo error", err)
}

func TestGetProposerDuties_NilData(t *testing.T) {
	const epoch = primitives.Epoch(1)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().Get(
		gomock.Any(),
		fmt.Sprintf("%s/%d", getProposerDutiesTestEndpoint, epoch),
		gomock.Any(),
	).Return(
		nil,
	).SetArg(
		2,
		structs.GetProposerDutiesResponse{
			Data: nil,
		},
	).Times(1)

	dutiesProvider := &dutiesProvider{jsonRestHandler: jsonRestHandler}
	_, err := dutiesProvider.ProposerDuties(ctx, epoch)
	assert.ErrorContains(t, "proposer duties data is nil", err)
}

func TestGetProposerDuties_NilProposerDuty(t *testing.T) {
	const epoch = primitives.Epoch(1)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().Get(
		gomock.Any(),
		fmt.Sprintf("%s/%d", getProposerDutiesTestEndpoint, epoch),
		gomock.Any(),
	).Return(
		nil,
	).SetArg(
		2,
		structs.GetProposerDutiesResponse{
			Data: []*structs.ProposerDuty{nil},
		},
	).Times(1)

	dutiesProvider := &dutiesProvider{jsonRestHandler: jsonRestHandler}
	_, err := dutiesProvider.ProposerDuties(ctx, epoch)
	assert.ErrorContains(t, "proposer duty at index `0` is nil", err)
}

func TestGetSyncDuties_Valid(t *testing.T) {
	stringValidatorIndices := []string{"2", "6"}
	const epoch = primitives.Epoch(1)

	validatorIndicesBytes, err := json.Marshal(stringValidatorIndices)
	require.NoError(t, err)

	expectedSyncDuties := structs.GetSyncCommitteeDutiesResponse{
		Data: []*structs.SyncCommitteeDuty{
			{
				Pubkey:         hexutil.Encode([]byte{1}),
				ValidatorIndex: "2",
				ValidatorSyncCommitteeIndices: []string{
					"3",
					"4",
				},
			},
			{
				Pubkey:         hexutil.Encode([]byte{5}),
				ValidatorIndex: "6",
				ValidatorSyncCommitteeIndices: []string{
					"7",
					"8",
				},
			},
		},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	validatorIndices := []primitives.ValidatorIndex{2, 6}
	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().Post(
		gomock.Any(),
		fmt.Sprintf("%s/%d", getSyncDutiesTestEndpoint, epoch),
		nil,
		bytes.NewBuffer(validatorIndicesBytes),
		&structs.GetSyncCommitteeDutiesResponse{},
	).Return(
		nil,
	).SetArg(
		4,
		expectedSyncDuties,
	).Times(1)

	dutiesProvider := &dutiesProvider{jsonRestHandler: jsonRestHandler}
	syncDuties, err := dutiesProvider.SyncDuties(ctx, epoch, validatorIndices)
	require.NoError(t, err)
	assert.DeepEqual(t, expectedSyncDuties.Data, syncDuties)
}

func TestGetSyncDuties_HttpError(t *testing.T) {
	const epoch = primitives.Epoch(1)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().Post(
		gomock.Any(),
		fmt.Sprintf("%s/%d", getSyncDutiesTestEndpoint, epoch),
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).Return(
		errors.New("foo error"),
	).Times(1)

	dutiesProvider := &dutiesProvider{jsonRestHandler: jsonRestHandler}
	_, err := dutiesProvider.SyncDuties(ctx, epoch, nil)
	assert.ErrorContains(t, "foo error", err)
}

func TestGetSyncDuties_NilData(t *testing.T) {
	const epoch = primitives.Epoch(1)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().Post(
		gomock.Any(),
		fmt.Sprintf("%s/%d", getSyncDutiesTestEndpoint, epoch),
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).Return(
		nil,
	).SetArg(
		4,
		structs.GetSyncCommitteeDutiesResponse{
			Data: nil,
		},
	).Times(1)

	dutiesProvider := &dutiesProvider{jsonRestHandler: jsonRestHandler}
	_, err := dutiesProvider.SyncDuties(ctx, epoch, nil)
	assert.ErrorContains(t, "sync duties data is nil", err)
}

func TestGetSyncDuties_NilSyncDuty(t *testing.T) {
	const epoch = primitives.Epoch(1)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().Post(
		gomock.Any(),
		fmt.Sprintf("%s/%d", getSyncDutiesTestEndpoint, epoch),
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).Return(
		nil,
	).SetArg(
		4,
		structs.GetSyncCommitteeDutiesResponse{
			Data: []*structs.SyncCommitteeDuty{nil},
		},
	).Times(1)

	dutiesProvider := &dutiesProvider{jsonRestHandler: jsonRestHandler}
	_, err := dutiesProvider.SyncDuties(ctx, epoch, nil)
	assert.ErrorContains(t, "sync duty at index `0` is nil", err)
}

func TestGetCommittees_Valid(t *testing.T) {
	const epoch = primitives.Epoch(1)

	expectedCommittees := structs.GetCommitteesResponse{
		Data: []*structs.Committee{
			{
				Index: "1",
				Slot:  "2",
				Validators: []string{
					"3",
					"4",
				},
			},
			{
				Index: "5",
				Slot:  "6",
				Validators: []string{
					"7",
					"8",
				},
			},
		},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().Get(
		gomock.Any(),
		fmt.Sprintf("%s?epoch=%d", getCommitteesTestEndpoint, epoch),
		&structs.GetCommitteesResponse{},
	).Return(
		nil,
	).SetArg(
		2,
		expectedCommittees,
	).Times(1)

	dutiesProvider := &dutiesProvider{jsonRestHandler: jsonRestHandler}
	committees, err := dutiesProvider.Committees(ctx, epoch)
	require.NoError(t, err)
	assert.DeepEqual(t, expectedCommittees.Data, committees)
}

func TestGetCommittees_HttpError(t *testing.T) {
	const epoch = primitives.Epoch(1)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().Get(
		gomock.Any(),
		fmt.Sprintf("%s?epoch=%d", getCommitteesTestEndpoint, epoch),
		gomock.Any(),
	).Return(
		errors.New("foo error"),
	).Times(1)

	dutiesProvider := &dutiesProvider{jsonRestHandler: jsonRestHandler}
	_, err := dutiesProvider.Committees(ctx, epoch)
	assert.ErrorContains(t, "foo error", err)
}

func TestGetCommittees_NilData(t *testing.T) {
	const epoch = primitives.Epoch(1)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().Get(
		gomock.Any(),
		fmt.Sprintf("%s?epoch=%d", getCommitteesTestEndpoint, epoch),
		gomock.Any(),
	).Return(
		nil,
	).SetArg(
		2,
		structs.GetCommitteesResponse{
			Data: nil,
		},
	).Times(1)

	dutiesProvider := &dutiesProvider{jsonRestHandler: jsonRestHandler}
	_, err := dutiesProvider.Committees(ctx, epoch)
	assert.ErrorContains(t, "state committees data is nil", err)
}

func TestGetCommittees_NilCommittee(t *testing.T) {
	const epoch = primitives.Epoch(1)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().Get(
		gomock.Any(),
		fmt.Sprintf("%s?epoch=%d", getCommitteesTestEndpoint, epoch),
		gomock.Any(),
	).Return(
		nil,
	).SetArg(
		2,
		structs.GetCommitteesResponse{
			Data: []*structs.Committee{nil},
		},
	).Times(1)

	dutiesProvider := &dutiesProvider{jsonRestHandler: jsonRestHandler}
	_, err := dutiesProvider.Committees(ctx, epoch)
	assert.ErrorContains(t, "committee at index `0` is nil", err)
}
