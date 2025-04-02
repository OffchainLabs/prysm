package validator_api

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/api/client/beacon"
	"github.com/prysmaticlabs/prysm/v5/api/client/beacon/mock"
	"github.com/prysmaticlabs/prysm/v5/api/server/structs"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"go.uber.org/mock/gomock"
)

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

			ctx := context.Background()

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
				attesterDuties,
				testCase.fetchAttesterDutiesError,
			).AnyTimes()

			dutiesProvider.EXPECT().ProposerDuties(
				ctx,
				epoch,
			).Return(
				proposerDuties,
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

			vals := make([]beacon.ValidatorForDuty, len(pubkeys))
			for i := 0; i < len(pubkeys); i++ {
				vals[i] = beacon.ValidatorForDuty{
					Pubkey: pubkeys[i],
					Index:  validatorIndices[i],
					Status: ethpb.ValidatorStatus_ACTIVE,
				}
			}

			validatorClient := &beaconApiValidatorClient{dutiesProvider: dutiesProvider}
			_, err := validatorClient.dutiesForEpoch(
				ctx,
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

			ctx := context.Background()

			dutiesProvider := mock.NewMockdutiesProvider(ctrl)

			dutiesProvider.EXPECT().AttesterDuties(
				ctx,
				epoch,
				validatorIndices,
			).Return(
				generateValidAttesterDuties(pubkeys, validatorIndices, committeeIndices, committeeSlots),
				nil,
			).Times(1)

			dutiesProvider.EXPECT().ProposerDuties(
				ctx,
				epoch,
			).Return(
				generateValidProposerDuties(pubkeys, validatorIndices, proposerSlots),
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
			vals := make([]beacon.ValidatorForDuty, len(pubkeys))
			for i := 0; i < len(pubkeys); i++ {
				vals[i] = beacon.ValidatorForDuty{
					Pubkey: pubkeys[i],
					Index:  validatorIndices[i],
					Status: ethpb.ValidatorStatus_ACTIVE,
				}
			}
			duties, err := validatorClient.dutiesForEpoch(
				ctx,
				epoch,
				vals,
				testCase.fetchSyncDuties,
			)
			require.NoError(t, err)
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
			vals := make([]beacon.ValidatorForDuty, valCount)
			for i := 0; i < valCount; i++ {
				pubkeys[i] = []byte(strconv.Itoa(i))
				validatorIndices[i] = primitives.ValidatorIndex(i)
				vals[i] = beacon.ValidatorForDuty{
					Pubkey: pubkeys[i],
					Index:  validatorIndices[i],
					Status: ethpb.ValidatorStatus_ACTIVE,
				}
			}

			committeeIndices := []primitives.CommitteeIndex{25, 26, 27}
			committeeSlots := []primitives.Slot{28, 29, 30}
			proposerSlots := []primitives.Slot{31, 32, 33, 34, 35, 36, 37, 38}

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ctx := context.Background()

			dutiesProvider := mock.NewMockdutiesProvider(ctrl)

			dutiesProvider.EXPECT().AttesterDuties(
				ctx,
				testCase.epoch,
				validatorIndices,
			).Return(
				generateValidAttesterDuties(pubkeys, validatorIndices, committeeIndices, committeeSlots),
				nil,
			).Times(2)

			dutiesProvider.EXPECT().ProposerDuties(
				ctx,
				testCase.epoch,
			).Return(
				generateValidProposerDuties(pubkeys, validatorIndices, proposerSlots),
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
				reverseSlice(generateValidAttesterDuties(pubkeys, validatorIndices, committeeIndices, committeeSlots)),
				nil,
			).Times(2)

			dutiesProvider.EXPECT().ProposerDuties(
				ctx,
				testCase.epoch+1,
			).Return(
				generateValidProposerDuties(pubkeys, validatorIndices, proposerSlots),
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

			expectedCurrentEpochDuties, err := validatorClient.dutiesForEpoch(
				ctx,
				testCase.epoch,
				vals,
				fetchSyncDuties,
			)
			require.NoError(t, err)

			expectedNextEpochDuties, err := validatorClient.dutiesForEpoch(
				ctx,
				testCase.epoch+1,
				vals,
				fetchSyncDuties,
			)
			require.NoError(t, err)

			expectedDuties := &ethpb.ValidatorDutiesContainer{
				CurrentEpochDuties: expectedCurrentEpochDuties,
				NextEpochDuties:    expectedNextEpochDuties,
			}

			duties, err := validatorClient.duties(ctx, &ethpb.DutiesRequest{
				Epoch:      testCase.epoch,
				PublicKeys: append(pubkeys, []byte("0xunknown")),
			})
			require.NoError(t, err)

			assert.DeepEqual(t, expectedDuties, duties)
		})
	}
}

func TestGetDuties_GetStateValidatorsFailed(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

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

	ctx := context.Background()
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
		primitives.Epoch(1),
		gomock.Any(),
	).Return(
		nil,
		errors.New("foo error"),
	).Times(1)
	dutiesProvider.EXPECT().AttesterDuties(
		ctx,
		primitives.Epoch(2),
		gomock.Any(),
	).Times(1)
	dutiesProvider.EXPECT().ProposerDuties(
		ctx,
		gomock.Any(),
	).Times(2)

	validatorClient := &beaconApiValidatorClient{
		stateValidatorsProvider: stateValidatorsProvider,
		dutiesProvider:          dutiesProvider,
	}

	_, err := validatorClient.duties(ctx, &ethpb.DutiesRequest{
		Epoch:      1,
		PublicKeys: [][]byte{pubkey},
	})
	assert.ErrorContains(t, "failed to get duties for current epoch `1`", err)
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
