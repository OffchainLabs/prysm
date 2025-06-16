package beacon_api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"testing"

	"github.com/OffchainLabs/prysm/v6/api/server/structs"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/validator/client/beacon-api/mock"
	"github.com/ethereum/go-ethereum/common/hexutil"
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

	ctx := t.Context()

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

	dutiesProvider := &beaconApiDutiesProvider{jsonRestHandler: jsonRestHandler}
	attesterDuties, err := dutiesProvider.AttesterDuties(ctx, epoch, validatorIndices)
	require.NoError(t, err)
	assert.DeepEqual(t, expectedAttesterDuties.Data, attesterDuties.Data)
}

func TestGetAttesterDuties_HttpError(t *testing.T) {
	const epoch = primitives.Epoch(1)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := t.Context()

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

	dutiesProvider := &beaconApiDutiesProvider{jsonRestHandler: jsonRestHandler}
	_, err := dutiesProvider.AttesterDuties(ctx, epoch, nil)
	assert.ErrorContains(t, "foo error", err)
}

func TestGetAttesterDuties_NilAttesterDuty(t *testing.T) {
	const epoch = primitives.Epoch(1)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := t.Context()

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

	dutiesProvider := &beaconApiDutiesProvider{jsonRestHandler: jsonRestHandler}
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

	ctx := t.Context()

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

	dutiesProvider := &beaconApiDutiesProvider{jsonRestHandler: jsonRestHandler}
	proposerDuties, err := dutiesProvider.ProposerDuties(ctx, epoch)
	require.NoError(t, err)
	assert.DeepEqual(t, expectedProposerDuties.Data, proposerDuties.Data)
}

func TestGetProposerDuties_HttpError(t *testing.T) {
	const epoch = primitives.Epoch(1)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := t.Context()

	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().Get(
		gomock.Any(),
		fmt.Sprintf("%s/%d", getProposerDutiesTestEndpoint, epoch),
		gomock.Any(),
	).Return(
		errors.New("foo error"),
	).Times(1)

	dutiesProvider := &beaconApiDutiesProvider{jsonRestHandler: jsonRestHandler}
	_, err := dutiesProvider.ProposerDuties(ctx, epoch)
	assert.ErrorContains(t, "foo error", err)
}

func TestGetProposerDuties_NilData(t *testing.T) {
	const epoch = primitives.Epoch(1)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := t.Context()

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

	dutiesProvider := &beaconApiDutiesProvider{jsonRestHandler: jsonRestHandler}
	_, err := dutiesProvider.ProposerDuties(ctx, epoch)
	assert.ErrorContains(t, "proposer duties data is nil", err)
}

func TestGetProposerDuties_NilProposerDuty(t *testing.T) {
	const epoch = primitives.Epoch(1)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := t.Context()

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

	dutiesProvider := &beaconApiDutiesProvider{jsonRestHandler: jsonRestHandler}
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

	ctx := t.Context()

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

	dutiesProvider := &beaconApiDutiesProvider{jsonRestHandler: jsonRestHandler}
	syncDuties, err := dutiesProvider.SyncDuties(ctx, epoch, validatorIndices)
	require.NoError(t, err)
	assert.DeepEqual(t, expectedSyncDuties.Data, syncDuties)
}

func TestGetSyncDuties_HttpError(t *testing.T) {
	const epoch = primitives.Epoch(1)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := t.Context()

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

	dutiesProvider := &beaconApiDutiesProvider{jsonRestHandler: jsonRestHandler}
	_, err := dutiesProvider.SyncDuties(ctx, epoch, nil)
	assert.ErrorContains(t, "foo error", err)
}

func TestGetSyncDuties_NilData(t *testing.T) {
	const epoch = primitives.Epoch(1)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := t.Context()

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

	dutiesProvider := &beaconApiDutiesProvider{jsonRestHandler: jsonRestHandler}
	_, err := dutiesProvider.SyncDuties(ctx, epoch, nil)
	assert.ErrorContains(t, "sync duties data is nil", err)
}

func TestGetSyncDuties_NilSyncDuty(t *testing.T) {
	const epoch = primitives.Epoch(1)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := t.Context()

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

	dutiesProvider := &beaconApiDutiesProvider{jsonRestHandler: jsonRestHandler}
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

	ctx := t.Context()

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

	dutiesProvider := &beaconApiDutiesProvider{jsonRestHandler: jsonRestHandler}
	committees, err := dutiesProvider.Committees(ctx, epoch)
	require.NoError(t, err)
	assert.DeepEqual(t, expectedCommittees.Data, committees)
}

func TestGetCommittees_HttpError(t *testing.T) {
	const epoch = primitives.Epoch(1)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := t.Context()

	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().Get(
		gomock.Any(),
		fmt.Sprintf("%s?epoch=%d", getCommitteesTestEndpoint, epoch),
		gomock.Any(),
	).Return(
		errors.New("foo error"),
	).Times(1)

	dutiesProvider := &beaconApiDutiesProvider{jsonRestHandler: jsonRestHandler}
	_, err := dutiesProvider.Committees(ctx, epoch)
	assert.ErrorContains(t, "foo error", err)
}

func TestGetCommittees_NilData(t *testing.T) {
	const epoch = primitives.Epoch(1)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := t.Context()

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

	dutiesProvider := &beaconApiDutiesProvider{jsonRestHandler: jsonRestHandler}
	_, err := dutiesProvider.Committees(ctx, epoch)
	assert.ErrorContains(t, "state committees data is nil", err)
}

func TestGetCommittees_NilCommittee(t *testing.T) {
	const epoch = primitives.Epoch(1)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := t.Context()

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

	dutiesProvider := &beaconApiDutiesProvider{jsonRestHandler: jsonRestHandler}
	_, err := dutiesProvider.Committees(ctx, epoch)
	assert.ErrorContains(t, "committee at index `0` is nil", err)
}

func TestGetDutiesForEpoch_Error(t *testing.T) {
	const epoch = primitives.Epoch(1)
	pubkeys := [][]byte{{1}, {2}, {3}, {4}, {5}, {6}, {7}, {8}, {9}, {10}, {11}, {12}}
	validatorIndices := []primitives.ValidatorIndex{13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24}
	committeeIndices := []primitives.CommitteeIndex{25, 26, 27}
	committeeSlots := []primitives.Slot{28, 29, 30}
	proposerSlots := []primitives.Slot{31, 32, 33, 34, 35, 36, 37, 38}

	testCases := []struct {
		name                     string
		expectedError            string
		generateAttesterDuties   func() []*structs.AttesterDuty
		fetchAttesterDutiesError error
		generateProposerDuties   func() []*structs.ProposerDuty
		fetchProposerDutiesError error
		generateSyncDuties       func() []*structs.SyncCommitteeDuty
		fetchSyncDutiesError     error
	}{
		{
			name:                     "get attester duties failed",
			expectedError:            "failed to get attester duties for epoch `1`: foo error",
			fetchAttesterDutiesError: errors.New("foo error"),
		},
		{
			name:                     "get proposer duties failed",
			expectedError:            "failed to get proposer duties for epoch `1`: foo error",
			fetchProposerDutiesError: errors.New("foo error"),
		},
		{
			name:                 "get sync duties failed",
			expectedError:        "failed to get sync duties for epoch `1`: foo error",
			fetchSyncDutiesError: errors.New("foo error"),
		},
		{
			name:          "bad attester validator index",
			expectedError: "failed to parse attester validator index `foo`",
			generateAttesterDuties: func() []*structs.AttesterDuty {
				attesterDuties := generateValidAttesterDuties(pubkeys, validatorIndices, committeeIndices, committeeSlots)
				attesterDuties[0].ValidatorIndex = "foo"
				return attesterDuties
			},
		},
		{
			name:          "bad attester slot",
			expectedError: "failed to parse attester slot `foo`",
			generateAttesterDuties: func() []*structs.AttesterDuty {
				attesterDuties := generateValidAttesterDuties(pubkeys, validatorIndices, committeeIndices, committeeSlots)
				attesterDuties[0].Slot = "foo"
				return attesterDuties
			},
		},
		{
			name:          "bad attester committee index",
			expectedError: "failed to parse attester committee index `foo`",
			generateAttesterDuties: func() []*structs.AttesterDuty {
				attesterDuties := generateValidAttesterDuties(pubkeys, validatorIndices, committeeIndices, committeeSlots)
				attesterDuties[0].CommitteeIndex = "foo"
				return attesterDuties
			},
		},
		{
			name:          "bad proposer validator index",
			expectedError: "failed to parse proposer validator index `foo`",
			generateProposerDuties: func() []*structs.ProposerDuty {
				proposerDuties := generateValidProposerDuties(pubkeys, validatorIndices, proposerSlots)
				proposerDuties[0].ValidatorIndex = "foo"
				return proposerDuties
			},
		},
		{
			name:          "bad proposer slot",
			expectedError: "failed to parse proposer slot `foo`",
			generateProposerDuties: func() []*structs.ProposerDuty {
				proposerDuties := generateValidProposerDuties(pubkeys, validatorIndices, proposerSlots)
				proposerDuties[0].Slot = "foo"
				return proposerDuties
			},
		},
		{
			name:          "bad sync validator index",
			expectedError: "failed to parse sync validator index `foo`",
			generateSyncDuties: func() []*structs.SyncCommitteeDuty {
				syncDuties := generateValidSyncDuties(pubkeys, validatorIndices)
				syncDuties[0].ValidatorIndex = "foo"
				return syncDuties
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ctx := t.Context()

			var attesterDuties []*structs.AttesterDuty
			if testCase.generateAttesterDuties == nil {
				attesterDuties = generateValidAttesterDuties(pubkeys, validatorIndices, committeeIndices, committeeSlots)
			} else {
				attesterDuties = testCase.generateAttesterDuties()
			}

			var proposerDuties []*structs.ProposerDuty
			if testCase.generateProposerDuties == nil {
				proposerDuties = generateValidProposerDuties(pubkeys, validatorIndices, proposerSlots)
			} else {
				proposerDuties = testCase.generateProposerDuties()
			}

			var syncDuties []*structs.SyncCommitteeDuty
			if testCase.generateSyncDuties == nil {
				syncDuties = generateValidSyncDuties(pubkeys, validatorIndices)
			} else {
				syncDuties = testCase.generateSyncDuties()
			}

			dutiesProvider := mock.NewMockdutiesProvider(ctrl)
			dutiesProvider.EXPECT().AttesterDuties(
				ctx,
				epoch,
				gomock.Any(),
			).Return(
				&structs.GetAttesterDutiesResponse{Data: attesterDuties},
				testCase.fetchAttesterDutiesError,
			).AnyTimes()

			dutiesProvider.EXPECT().ProposerDuties(
				ctx,
				epoch,
			).Return(
				&structs.GetProposerDutiesResponse{
					DependentRoot: "0xdeadbeef000000000000000000000000000000000000000000000000",
					Data:          proposerDuties},
				testCase.fetchProposerDutiesError,
			).AnyTimes()

			dutiesProvider.EXPECT().SyncDuties(
				ctx,
				epoch,
				gomock.Any(),
			).Return(
				syncDuties,
				testCase.fetchSyncDutiesError,
			).AnyTimes()

			vals := make([]validatorForDuty, len(pubkeys))
			for i := 0; i < len(pubkeys); i++ {
				vals[i] = validatorForDuty{
					pubkey: pubkeys[i],
					index:  validatorIndices[i],
					status: ethpb.ValidatorStatus_ACTIVE,
				}
			}

			validatorClient := &beaconApiValidatorClient{dutiesProvider: dutiesProvider}
			err := validatorClient.dutiesForEpoch(
				ctx,
				&ethpb.ValidatorDutiesContainer{},
				epoch,
				vals,
				true,
			)
			assert.ErrorContains(t, testCase.expectedError, err)
		})
	}
}

func TestGetDutiesForEpoch_Valid(t *testing.T) {
	testCases := []struct {
		name            string
		fetchSyncDuties bool
	}{
		{
			name:            "fetch attester and proposer duties",
			fetchSyncDuties: false,
		},
		{
			name:            "fetch attester and sync and proposer duties",
			fetchSyncDuties: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			const epoch = primitives.Epoch(1)
			pubkeys := [][]byte{{1}, {2}, {3}, {4}, {5}, {6}, {7}, {8}, {9}, {10}, {11}, {12}}
			validatorIndices := []primitives.ValidatorIndex{13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24}
			committeeIndices := []primitives.CommitteeIndex{25, 26, 27}
			committeeSlots := []primitives.Slot{28, 29, 30}
			proposerSlots := []primitives.Slot{31, 32, 33, 34, 35, 36, 37, 38}

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ctx := t.Context()

			dutiesProvider := mock.NewMockdutiesProvider(ctrl)

			dutiesProvider.EXPECT().AttesterDuties(
				ctx,
				epoch,
				validatorIndices,
			).Return(
				&structs.GetAttesterDutiesResponse{
					DependentRoot: "0xdeadbeef000000000000000000000000000000000000000000000000",
					Data:          generateValidAttesterDuties(pubkeys, validatorIndices, committeeIndices, committeeSlots),
				},
				nil,
			).Times(1)

			dutiesProvider.EXPECT().ProposerDuties(
				ctx,
				epoch,
			).Return(
				&structs.GetProposerDutiesResponse{
					DependentRoot: "0xdeadbeef000000000000000000000000000000000000000000000000",
					Data:          generateValidProposerDuties(pubkeys, validatorIndices, proposerSlots),
				},
				nil,
			).Times(1)

			if testCase.fetchSyncDuties {
				dutiesProvider.EXPECT().SyncDuties(
					ctx,
					epoch,
					validatorIndices,
				).Return(
					generateValidSyncDuties(pubkeys, validatorIndices),
					nil,
				).Times(1)
			}

			var expectedProposerSlots1 []primitives.Slot
			var expectedProposerSlots2 []primitives.Slot
			var expectedProposerSlots3 []primitives.Slot
			var expectedProposerSlots4 []primitives.Slot

			expectedProposerSlots1 = []primitives.Slot{
				proposerSlots[0],
				proposerSlots[1],
			}

			expectedProposerSlots2 = []primitives.Slot{
				proposerSlots[2],
				proposerSlots[3],
			}

			expectedProposerSlots3 = []primitives.Slot{
				proposerSlots[4],
				proposerSlots[5],
			}

			expectedProposerSlots4 = []primitives.Slot{
				proposerSlots[6],
				proposerSlots[7],
			}

			expectedDuties := []*ethpb.DutiesResponse_Duty{
				{
					Committee: []primitives.ValidatorIndex{
						validatorIndices[0],
						validatorIndices[1],
					},
					CommitteeIndex:   committeeIndices[0],
					AttesterSlot:     committeeSlots[0],
					PublicKey:        pubkeys[0],
					Status:           ethpb.ValidatorStatus_ACTIVE,
					ValidatorIndex:   validatorIndices[0],
					CommitteesAtSlot: 1,
				},
				{
					Committee: []primitives.ValidatorIndex{
						validatorIndices[0],
						validatorIndices[1],
					},
					CommitteeIndex:   committeeIndices[0],
					AttesterSlot:     committeeSlots[0],
					PublicKey:        pubkeys[1],
					Status:           ethpb.ValidatorStatus_ACTIVE,
					ValidatorIndex:   validatorIndices[1],
					CommitteesAtSlot: 1,
				},
				{
					Committee: []primitives.ValidatorIndex{
						validatorIndices[2],
						validatorIndices[3],
					},
					CommitteeIndex:   committeeIndices[1],
					AttesterSlot:     committeeSlots[1],
					PublicKey:        pubkeys[2],
					Status:           ethpb.ValidatorStatus_ACTIVE,
					ValidatorIndex:   validatorIndices[2],
					CommitteesAtSlot: 1,
				},
				{
					Committee: []primitives.ValidatorIndex{
						validatorIndices[2],
						validatorIndices[3],
					},
					CommitteeIndex:   committeeIndices[1],
					AttesterSlot:     committeeSlots[1],
					PublicKey:        pubkeys[3],
					Status:           ethpb.ValidatorStatus_ACTIVE,
					ValidatorIndex:   validatorIndices[3],
					CommitteesAtSlot: 1,
				},
				{
					Committee: []primitives.ValidatorIndex{
						validatorIndices[4],
						validatorIndices[5],
					},
					CommitteeIndex:   committeeIndices[2],
					AttesterSlot:     committeeSlots[2],
					PublicKey:        pubkeys[4],
					Status:           ethpb.ValidatorStatus_ACTIVE,
					ValidatorIndex:   validatorIndices[4],
					ProposerSlots:    expectedProposerSlots1,
					CommitteesAtSlot: 1,
				},
				{
					Committee: []primitives.ValidatorIndex{
						validatorIndices[4],
						validatorIndices[5],
					},
					CommitteeIndex:   committeeIndices[2],
					AttesterSlot:     committeeSlots[2],
					PublicKey:        pubkeys[5],
					Status:           ethpb.ValidatorStatus_ACTIVE,
					ValidatorIndex:   validatorIndices[5],
					ProposerSlots:    expectedProposerSlots2,
					IsSyncCommittee:  testCase.fetchSyncDuties,
					CommitteesAtSlot: 1,
				},
				{
					PublicKey:       pubkeys[6],
					Status:          ethpb.ValidatorStatus_ACTIVE,
					ValidatorIndex:  validatorIndices[6],
					ProposerSlots:   expectedProposerSlots3,
					IsSyncCommittee: testCase.fetchSyncDuties,
				},
				{
					PublicKey:       pubkeys[7],
					Status:          ethpb.ValidatorStatus_ACTIVE,
					ValidatorIndex:  validatorIndices[7],
					ProposerSlots:   expectedProposerSlots4,
					IsSyncCommittee: testCase.fetchSyncDuties,
				},
				{
					PublicKey:       pubkeys[8],
					Status:          ethpb.ValidatorStatus_ACTIVE,
					ValidatorIndex:  validatorIndices[8],
					IsSyncCommittee: testCase.fetchSyncDuties,
				},
				{
					PublicKey:       pubkeys[9],
					Status:          ethpb.ValidatorStatus_ACTIVE,
					ValidatorIndex:  validatorIndices[9],
					IsSyncCommittee: testCase.fetchSyncDuties,
				},
				{
					PublicKey:      pubkeys[10],
					Status:         ethpb.ValidatorStatus_ACTIVE,
					ValidatorIndex: validatorIndices[10],
				},
				{
					PublicKey:      pubkeys[11],
					Status:         ethpb.ValidatorStatus_ACTIVE,
					ValidatorIndex: validatorIndices[11],
				},
			}

			validatorClient := &beaconApiValidatorClient{dutiesProvider: dutiesProvider}
			vals := make([]validatorForDuty, len(pubkeys))
			for i := 0; i < len(pubkeys); i++ {
				vals[i] = validatorForDuty{
					pubkey: pubkeys[i],
					index:  validatorIndices[i],
					status: ethpb.ValidatorStatus_ACTIVE,
				}
			}
			dutiesContainer := &ethpb.ValidatorDutiesContainer{}
			err := validatorClient.dutiesForEpoch(
				ctx,
				dutiesContainer,
				epoch,
				vals,
				testCase.fetchSyncDuties,
			)
			require.NoError(t, err)
			duties := dutiesContainer.CurrentEpochDuties
			require.Equal(t, len(expectedDuties), len(duties))
			for i, duty := range expectedDuties {
				assert.Equal(t, duty.CommitteeIndex, duties[i].CommitteeIndex)
				assert.DeepEqual(t, duty.ProposerSlots, duties[i].ProposerSlots)
				assert.Equal(t, duty.ValidatorIndex, duties[i].ValidatorIndex)
				assert.Equal(t, duty.IsSyncCommittee, duties[i].IsSyncCommittee)
				assert.Equal(t, duty.Status, duties[i].Status)
			}
		})
	}
}

func TestGetDuties_Valid(t *testing.T) {
	testCases := []struct {
		name  string
		epoch primitives.Epoch
	}{
		{
			name:  "genesis epoch",
			epoch: params.BeaconConfig().GenesisEpoch,
		},
		{
			name:  "altair epoch",
			epoch: params.BeaconConfig().AltairForkEpoch,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			valCount := 12
			pubkeys := make([][]byte, valCount)
			validatorIndices := make([]primitives.ValidatorIndex, valCount)
			vals := make([]validatorForDuty, valCount)
			for i := 0; i < valCount; i++ {
				pubkeys[i] = []byte(strconv.Itoa(i))
				validatorIndices[i] = primitives.ValidatorIndex(i)
				vals[i] = validatorForDuty{
					pubkey: pubkeys[i],
					index:  validatorIndices[i],
					status: ethpb.ValidatorStatus_ACTIVE,
				}
			}

			committeeIndices := []primitives.CommitteeIndex{25, 26, 27}
			committeeSlots := []primitives.Slot{28, 29, 30}
			proposerSlots := []primitives.Slot{31, 32, 33, 34, 35, 36, 37, 38}

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ctx := t.Context()

			dutiesProvider := mock.NewMockdutiesProvider(ctrl)

			dutiesProvider.EXPECT().AttesterDuties(
				ctx,
				testCase.epoch,
				validatorIndices,
			).Return(
				&structs.GetAttesterDutiesResponse{
					DependentRoot: "0xdeadbeef000000000000000000000000000000000000000000000000",
					Data:          generateValidAttesterDuties(pubkeys, validatorIndices, committeeIndices, committeeSlots),
				},
				nil,
			).Times(2)

			dutiesProvider.EXPECT().ProposerDuties(
				ctx,
				testCase.epoch,
			).Return(
				&structs.GetProposerDutiesResponse{
					DependentRoot: "0xdeadbeef000000000000000000000000000000000000000000000000",
					Data:          generateValidProposerDuties(pubkeys, validatorIndices, proposerSlots),
				},
				nil,
			).Times(2)

			fetchSyncDuties := testCase.epoch >= params.BeaconConfig().AltairForkEpoch
			if fetchSyncDuties {
				dutiesProvider.EXPECT().SyncDuties(
					ctx,
					testCase.epoch,
					validatorIndices,
				).Return(
					generateValidSyncDuties(pubkeys, validatorIndices),
					nil,
				).Times(2)
			}

			dutiesProvider.EXPECT().AttesterDuties(
				ctx,
				testCase.epoch+1,
				validatorIndices,
			).Return(
				&structs.GetAttesterDutiesResponse{
					DependentRoot: "0xdeadbeef000000000000000000000000000000000000000000000000",
					Data:          reverseSlice(generateValidAttesterDuties(pubkeys, validatorIndices, committeeIndices, committeeSlots)),
				},
				nil,
			).Times(2)

			dutiesProvider.EXPECT().ProposerDuties(
				ctx,
				testCase.epoch+1,
			).Return(
				&structs.GetProposerDutiesResponse{
					DependentRoot: "0xdeadbeef000000000000000000000000000000000000000000000000",
					Data:          generateValidProposerDuties(pubkeys, validatorIndices, proposerSlots),
				},
				nil,
			).Times(2)

			if fetchSyncDuties {
				dutiesProvider.EXPECT().SyncDuties(
					ctx,
					testCase.epoch+1,
					validatorIndices,
				).Return(
					reverseSlice(generateValidSyncDuties(pubkeys, validatorIndices)),
					nil,
				).Times(2)
			}

			stateValidatorsProvider := mock.NewMockStateValidatorsProvider(ctrl)
			stateValidatorsProvider.EXPECT().StateValidators(
				gomock.Any(),
				gomock.Any(),
				gomock.Any(),
				gomock.Any(),
			).Return(
				&structs.GetValidatorsResponse{
					Data: []*structs.ValidatorContainer{
						{
							Index:  strconv.FormatUint(uint64(validatorIndices[0]), 10),
							Status: "active_ongoing",
							Validator: &structs.Validator{
								Pubkey:          hexutil.Encode(pubkeys[0]),
								ActivationEpoch: strconv.FormatUint(uint64(testCase.epoch), 10),
							},
						},
						{
							Index:  strconv.FormatUint(uint64(validatorIndices[1]), 10),
							Status: "active_ongoing",
							Validator: &structs.Validator{
								Pubkey:          hexutil.Encode(pubkeys[1]),
								ActivationEpoch: strconv.FormatUint(uint64(testCase.epoch), 10),
							},
						},
						{
							Index:  strconv.FormatUint(uint64(validatorIndices[2]), 10),
							Status: "active_ongoing",
							Validator: &structs.Validator{
								Pubkey:          hexutil.Encode(pubkeys[2]),
								ActivationEpoch: strconv.FormatUint(uint64(testCase.epoch), 10),
							},
						},
						{
							Index:  strconv.FormatUint(uint64(validatorIndices[3]), 10),
							Status: "active_ongoing",
							Validator: &structs.Validator{
								Pubkey:          hexutil.Encode(pubkeys[3]),
								ActivationEpoch: strconv.FormatUint(uint64(testCase.epoch), 10),
							},
						},
						{
							Index:  strconv.FormatUint(uint64(validatorIndices[4]), 10),
							Status: "active_ongoing",
							Validator: &structs.Validator{
								Pubkey:          hexutil.Encode(pubkeys[4]),
								ActivationEpoch: strconv.FormatUint(uint64(testCase.epoch), 10),
							},
						},
						{
							Index:  strconv.FormatUint(uint64(validatorIndices[5]), 10),
							Status: "active_ongoing",
							Validator: &structs.Validator{
								Pubkey:          hexutil.Encode(pubkeys[5]),
								ActivationEpoch: strconv.FormatUint(uint64(testCase.epoch), 10),
							},
						},
						{
							Index:  strconv.FormatUint(uint64(validatorIndices[6]), 10),
							Status: "active_ongoing",
							Validator: &structs.Validator{
								Pubkey:          hexutil.Encode(pubkeys[6]),
								ActivationEpoch: strconv.FormatUint(uint64(testCase.epoch), 10),
							},
						},
						{
							Index:  strconv.FormatUint(uint64(validatorIndices[7]), 10),
							Status: "active_ongoing",
							Validator: &structs.Validator{
								Pubkey:          hexutil.Encode(pubkeys[7]),
								ActivationEpoch: strconv.FormatUint(uint64(testCase.epoch), 10),
							},
						},
						{
							Index:  strconv.FormatUint(uint64(validatorIndices[8]), 10),
							Status: "active_ongoing",
							Validator: &structs.Validator{
								Pubkey:          hexutil.Encode(pubkeys[8]),
								ActivationEpoch: strconv.FormatUint(uint64(testCase.epoch), 10),
							},
						},
						{
							Index:  strconv.FormatUint(uint64(validatorIndices[9]), 10),
							Status: "active_ongoing",
							Validator: &structs.Validator{
								Pubkey:          hexutil.Encode(pubkeys[9]),
								ActivationEpoch: strconv.FormatUint(uint64(testCase.epoch), 10),
							},
						},
						{
							Index:  strconv.FormatUint(uint64(validatorIndices[10]), 10),
							Status: "active_ongoing",
							Validator: &structs.Validator{
								Pubkey:          hexutil.Encode(pubkeys[10]),
								ActivationEpoch: strconv.FormatUint(uint64(testCase.epoch), 10),
							},
						},
						{
							Index:  strconv.FormatUint(uint64(validatorIndices[11]), 10),
							Status: "active_ongoing",
							Validator: &structs.Validator{
								Pubkey:          hexutil.Encode(pubkeys[11]),
								ActivationEpoch: strconv.FormatUint(uint64(testCase.epoch), 10),
							},
						},
					},
				},
				nil,
			).MinTimes(1)

			// Make sure that our values are equal to what would be returned by calling dutiesForEpoch individually
			validatorClient := &beaconApiValidatorClient{
				dutiesProvider:          dutiesProvider,
				stateValidatorsProvider: stateValidatorsProvider,
			}

			expectedContainer := &ethpb.ValidatorDutiesContainer{}
			err := validatorClient.dutiesForEpoch(
				ctx,
				expectedContainer,
				testCase.epoch,
				vals,
				fetchSyncDuties,
			)
			require.NoError(t, err)

			expectedCurrentEpochDuties := expectedContainer.CurrentEpochDuties
			expectedNextContainer := &ethpb.ValidatorDutiesContainer{}
			err = validatorClient.dutiesForEpoch(
				ctx,
				expectedNextContainer,
				testCase.epoch+1,
				vals,
				fetchSyncDuties,
			)
			require.NoError(t, err)

			expectedNextEpochDuties := expectedNextContainer.CurrentEpochDuties
			expectedDuties := &ethpb.ValidatorDutiesContainer{
				CurrentEpochDuties: expectedCurrentEpochDuties,
				NextEpochDuties:    expectedNextEpochDuties,
			}

			duties, err := validatorClient.duties(ctx, &ethpb.DutiesRequest{
				Epoch:      testCase.epoch,
				PublicKeys: append(pubkeys, []byte("0xunknown")),
			})
			require.NoError(t, err)

			assert.DeepEqual(t, expectedDuties.NextEpochDuties, duties.NextEpochDuties)
			assert.DeepEqual(t, expectedDuties.CurrentEpochDuties, duties.CurrentEpochDuties)
		})
	}
}

func TestGetDuties_GetStateValidatorsFailed(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := t.Context()

	stateValidatorsProvider := mock.NewMockStateValidatorsProvider(ctrl)
	stateValidatorsProvider.EXPECT().StateValidators(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).Return(
		nil,
		errors.New("foo error"),
	).Times(1)

	validatorClient := &beaconApiValidatorClient{
		stateValidatorsProvider: stateValidatorsProvider,
	}

	_, err := validatorClient.duties(ctx, &ethpb.DutiesRequest{
		Epoch:      1,
		PublicKeys: [][]byte{},
	})
	assert.ErrorContains(t, "failed to get state validators", err)
	assert.ErrorContains(t, "foo error", err)
}

func TestGetDuties_GetDutiesForEpochFailed(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := t.Context()
	pubkey := []byte{1, 2, 3}

	stateValidatorsProvider := mock.NewMockStateValidatorsProvider(ctrl)
	stateValidatorsProvider.EXPECT().StateValidators(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).Return(
		&structs.GetValidatorsResponse{
			Data: []*structs.ValidatorContainer{{
				Index:  "0",
				Status: "active_ongoing",
				Validator: &structs.Validator{
					Pubkey: hexutil.Encode(pubkey),
				},
			}},
		},
		nil,
	).Times(1)

	dutiesProvider := mock.NewMockdutiesProvider(ctrl)
	dutiesProvider.EXPECT().AttesterDuties(
		ctx,
		gomock.Any(),
		gomock.Any(),
	).Return(
		&structs.GetAttesterDutiesResponse{
			DependentRoot: "0xdeadbeef000000000000000000000000000000000000000000000000",
			Data:          []*structs.AttesterDuty{}},
		errors.New("foo error"),
	).Times(2)
	dutiesProvider.EXPECT().ProposerDuties(
		ctx,
		gomock.Any(),
	).Return(
		&structs.GetProposerDutiesResponse{
			DependentRoot: "0xdeadbeef000000000000000000000000000000000000000000000000",
			Data:          []*structs.ProposerDuty{},
		},
		nil,
	).Times(2)

	validatorClient := &beaconApiValidatorClient{
		stateValidatorsProvider: stateValidatorsProvider,
		dutiesProvider:          dutiesProvider,
	}

	_, err := validatorClient.duties(ctx, &ethpb.DutiesRequest{
		Epoch:      1,
		PublicKeys: [][]byte{pubkey},
	})
	assert.ErrorContains(t, "failed to get duties for", err)
	assert.ErrorContains(t, "foo error", err)
}

func generateValidAttesterDuties(pubkeys [][]byte, validatorIndices []primitives.ValidatorIndex, committeeIndices []primitives.CommitteeIndex, slots []primitives.Slot) []*structs.AttesterDuty {
	return []*structs.AttesterDuty{
		{
			Pubkey:                  hexutil.Encode(pubkeys[0]),
			ValidatorIndex:          strconv.FormatUint(uint64(validatorIndices[0]), 10),
			CommitteeIndex:          strconv.FormatUint(uint64(committeeIndices[0]), 10),
			CommitteeLength:         fmt.Sprintf("%d", len(committeeIndices)),
			ValidatorCommitteeIndex: strconv.FormatUint(uint64(0), 10),
			CommitteesAtSlot:        strconv.FormatUint(uint64(10), 10),
			Slot:                    strconv.FormatUint(uint64(slots[0]), 10),
		},
		{
			Pubkey:                  hexutil.Encode(pubkeys[1]),
			ValidatorIndex:          strconv.FormatUint(uint64(validatorIndices[1]), 10),
			CommitteeIndex:          strconv.FormatUint(uint64(committeeIndices[0]), 10),
			CommitteeLength:         fmt.Sprintf("%d", len(committeeIndices)),
			ValidatorCommitteeIndex: strconv.FormatUint(uint64(0), 10),
			CommitteesAtSlot:        strconv.FormatUint(uint64(10), 10),
			Slot:                    strconv.FormatUint(uint64(slots[0]), 10),
		},
		{
			Pubkey:                  hexutil.Encode(pubkeys[2]),
			ValidatorIndex:          strconv.FormatUint(uint64(validatorIndices[2]), 10),
			CommitteeIndex:          strconv.FormatUint(uint64(committeeIndices[1]), 10),
			CommitteeLength:         fmt.Sprintf("%d", len(committeeIndices)),
			ValidatorCommitteeIndex: strconv.FormatUint(uint64(0), 10),
			CommitteesAtSlot:        strconv.FormatUint(uint64(10), 10),
			Slot:                    strconv.FormatUint(uint64(slots[1]), 10),
		},
		{
			Pubkey:                  hexutil.Encode(pubkeys[3]),
			ValidatorIndex:          strconv.FormatUint(uint64(validatorIndices[3]), 10),
			CommitteeIndex:          strconv.FormatUint(uint64(committeeIndices[1]), 10),
			CommitteeLength:         fmt.Sprintf("%d", len(committeeIndices)),
			ValidatorCommitteeIndex: strconv.FormatUint(uint64(0), 10),
			CommitteesAtSlot:        strconv.FormatUint(uint64(10), 10),
			Slot:                    strconv.FormatUint(uint64(slots[1]), 10),
		},
		{
			Pubkey:                  hexutil.Encode(pubkeys[4]),
			ValidatorIndex:          strconv.FormatUint(uint64(validatorIndices[4]), 10),
			CommitteeIndex:          strconv.FormatUint(uint64(committeeIndices[2]), 10),
			CommitteeLength:         fmt.Sprintf("%d", len(committeeIndices)),
			ValidatorCommitteeIndex: strconv.FormatUint(uint64(0), 10),
			CommitteesAtSlot:        strconv.FormatUint(uint64(10), 10),
			Slot:                    strconv.FormatUint(uint64(slots[2]), 10),
		},
		{
			Pubkey:                  hexutil.Encode(pubkeys[5]),
			ValidatorIndex:          strconv.FormatUint(uint64(validatorIndices[5]), 10),
			CommitteeIndex:          strconv.FormatUint(uint64(committeeIndices[2]), 10),
			CommitteeLength:         fmt.Sprintf("%d", len(committeeIndices)),
			ValidatorCommitteeIndex: strconv.FormatUint(uint64(0), 10),
			CommitteesAtSlot:        strconv.FormatUint(uint64(10), 10),
			Slot:                    strconv.FormatUint(uint64(slots[2]), 10),
		},
	}
}

func generateValidProposerDuties(pubkeys [][]byte, validatorIndices []primitives.ValidatorIndex, slots []primitives.Slot) []*structs.ProposerDuty {
	return []*structs.ProposerDuty{
		{
			Pubkey:         hexutil.Encode(pubkeys[4]),
			ValidatorIndex: strconv.FormatUint(uint64(validatorIndices[4]), 10),
			Slot:           strconv.FormatUint(uint64(slots[0]), 10),
		},
		{
			Pubkey:         hexutil.Encode(pubkeys[4]),
			ValidatorIndex: strconv.FormatUint(uint64(validatorIndices[4]), 10),
			Slot:           strconv.FormatUint(uint64(slots[1]), 10),
		},
		{
			Pubkey:         hexutil.Encode(pubkeys[5]),
			ValidatorIndex: strconv.FormatUint(uint64(validatorIndices[5]), 10),
			Slot:           strconv.FormatUint(uint64(slots[2]), 10),
		},
		{
			Pubkey:         hexutil.Encode(pubkeys[5]),
			ValidatorIndex: strconv.FormatUint(uint64(validatorIndices[5]), 10),
			Slot:           strconv.FormatUint(uint64(slots[3]), 10),
		},
		{
			Pubkey:         hexutil.Encode(pubkeys[6]),
			ValidatorIndex: strconv.FormatUint(uint64(validatorIndices[6]), 10),
			Slot:           strconv.FormatUint(uint64(slots[4]), 10),
		},
		{
			Pubkey:         hexutil.Encode(pubkeys[6]),
			ValidatorIndex: strconv.FormatUint(uint64(validatorIndices[6]), 10),
			Slot:           strconv.FormatUint(uint64(slots[5]), 10),
		},
		{
			Pubkey:         hexutil.Encode(pubkeys[7]),
			ValidatorIndex: strconv.FormatUint(uint64(validatorIndices[7]), 10),
			Slot:           strconv.FormatUint(uint64(slots[6]), 10),
		},
		{
			Pubkey:         hexutil.Encode(pubkeys[7]),
			ValidatorIndex: strconv.FormatUint(uint64(validatorIndices[7]), 10),
			Slot:           strconv.FormatUint(uint64(slots[7]), 10),
		},
	}
}

func generateValidSyncDuties(pubkeys [][]byte, validatorIndices []primitives.ValidatorIndex) []*structs.SyncCommitteeDuty {
	return []*structs.SyncCommitteeDuty{
		{
			Pubkey:         hexutil.Encode(pubkeys[5]),
			ValidatorIndex: strconv.FormatUint(uint64(validatorIndices[5]), 10),
		},
		{
			Pubkey:         hexutil.Encode(pubkeys[6]),
			ValidatorIndex: strconv.FormatUint(uint64(validatorIndices[6]), 10),
		},
		{
			Pubkey:         hexutil.Encode(pubkeys[7]),
			ValidatorIndex: strconv.FormatUint(uint64(validatorIndices[7]), 10),
		},
		{
			Pubkey:         hexutil.Encode(pubkeys[8]),
			ValidatorIndex: strconv.FormatUint(uint64(validatorIndices[8]), 10),
		},
		{
			Pubkey:         hexutil.Encode(pubkeys[9]),
			ValidatorIndex: strconv.FormatUint(uint64(validatorIndices[9]), 10),
		},
	}
}

// We will use a reverse function to easily make sure that the current epoch and next epoch data returned by dutiesForEpoch
// are not the same
func reverseSlice[T interface{}](slice []T) []T {
	reversedSlice := make([]T, len(slice))
	for i := range slice {
		reversedSlice[len(reversedSlice)-1-i] = slice[i]
	}
	return reversedSlice
}
