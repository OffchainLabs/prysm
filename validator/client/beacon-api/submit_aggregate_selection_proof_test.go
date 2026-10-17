package beacon_api

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/validator/client/beacon-api/mock"
	testhelpers "github.com/OffchainLabs/prysm/v7/validator/client/beacon-api/test-helpers"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"go.uber.org/mock/gomock"
)

func TestSubmitAggregateSelectionProof(t *testing.T) {
	const (
		pubkeyStr                    = "0x8000091c2ae64ee414a54c1cc1fc67dec663408bc636cb86756e0200e41a75c8f86603f104f02c856983d2783116be13"
		syncingEndpoint              = "/eth/v1/node/syncing"
		attestationDataEndpoint      = "/eth/v1/validator/attestation_data"
		aggregateAttestationEndpoint = "/eth/v2/validator/aggregate_attestation"
		validatorIndex               = primitives.ValidatorIndex(55293)
		slotSignature                = "0x8776a37d6802c4797d113169c5fcfda50e68a32058eb6356a6f00d06d7da64c841a00c7c38b9b94a204751eca53707bd03523ce4797827d9bacff116a6e776a20bbccff4b683bf5201b610797ed0502557a58a65c8395f8a1649b976c3112d15"
		slot                         = primitives.Slot(123)
		committeeIndex               = primitives.CommitteeIndex(1)
		committeesAtSlot             = uint64(1)
	)

	attestationDataResponse := generateValidAttestation(uint64(slot), uint64(committeeIndex))
	attestationDataProto, err := attestationDataResponse.Data.ToConsensus()
	require.NoError(t, err)
	attestationDataRootBytes, err := attestationDataProto.HashTreeRoot()
	require.NoError(t, err)

	aggregateAttestation := &ethpb.Attestation{
		AggregationBits: testhelpers.FillByteSlice(4, 74),
		Data:            attestationDataProto,
		Signature:       testhelpers.FillByteSlice(96, 82),
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		name                       string
		isOptimistic               bool
		syncingErr                 error
		attestationDataErr         error
		aggregateAttestationErr    error
		attestationDataCalled      int
		aggregateAttestationCalled int
		expectedErrorMsg           string
		committeesAtSlot           uint64
	}{
		{
			name:                       "success",
			attestationDataCalled:      1,
			aggregateAttestationCalled: 1,
		},
		{
			name:             "head is optimistic",
			isOptimistic:     true,
			expectedErrorMsg: "the node is currently optimistic and cannot serve validators",
		},
		{
			name:             "syncing error",
			syncingErr:       errors.New("bad request"),
			expectedErrorMsg: "failed to get syncing status",
		},
		{
			name:                  "attestation data error",
			attestationDataCalled: 1,
			attestationDataErr:    errors.New("bad request"),
			expectedErrorMsg:      fmt.Sprintf("failed to get attestation data for slot=%d and committee_index=%d", slot, committeeIndex),
		},
		{
			name:                       "aggregate attestation error",
			attestationDataCalled:      1,
			aggregateAttestationCalled: 1,
			aggregateAttestationErr:    errors.New("bad request"),
			expectedErrorMsg:           "bad request",
		},
		{
			name:             "validator is not an aggregator",
			committeesAtSlot: 64,
			expectedErrorMsg: "validator is not an aggregator",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := t.Context()
			handler := mock.NewMockHandler(ctrl)

			// Call node syncing endpoint to check if head is optimistic.
			handler.EXPECT().Get(
				gomock.Any(),
				syncingEndpoint,
				&structs.SyncStatusResponse{},
			).SetArg(
				2,
				structs.SyncStatusResponse{
					Data: &structs.SyncStatusResponseData{
						IsOptimistic: test.isOptimistic,
					},
				},
			).Return(
				test.syncingErr,
			).Times(1)

			// Call attestation data to get attestation data root to query aggregate attestation.
			handler.EXPECT().Get(
				gomock.Any(),
				fmt.Sprintf("%s?committee_index=%d&slot=%d", attestationDataEndpoint, committeeIndex, slot),
				&structs.GetAttestationDataResponse{},
			).SetArg(
				2,
				attestationDataResponse,
			).Return(
				test.attestationDataErr,
			).Times(test.attestationDataCalled)

			attestationJSON, err := json.Marshal(jsonifyAttestation(aggregateAttestation))
			require.NoError(t, err)

			// Call attestation data to get attestation data root to query aggregate attestation.
			handler.EXPECT().Get(
				gomock.Any(),
				fmt.Sprintf("%s?attestation_data_root=%s&committee_index=%d&slot=%d", aggregateAttestationEndpoint, hexutil.Encode(attestationDataRootBytes[:]), committeeIndex, slot),
				&structs.AggregateAttestationResponse{},
			).SetArg(
				2,
				structs.AggregateAttestationResponse{
					Data: attestationJSON,
				},
			).Return(
				test.aggregateAttestationErr,
			).Times(test.aggregateAttestationCalled)

			pubkey, err := hexutil.Decode(pubkeyStr)
			require.NoError(t, err)

			slotSignatureBytes, err := hexutil.Decode(slotSignature)
			require.NoError(t, err)

			expectedResponse := &ethpb.AggregateSelectionResponse{
				AggregateAndProof: &ethpb.AggregateAttestationAndProof{
					AggregatorIndex: primitives.ValidatorIndex(55293),
					Aggregate:       aggregateAttestation,
					SelectionProof:  slotSignatureBytes,
				},
			}

			validatorClient := &beaconApiValidatorClient{
				handler: handler,
				stateValidatorsProvider: beaconApiStateValidatorsProvider{
					handler: handler,
				},
				dutiesProvider: beaconApiDutiesProvider{
					handler: handler,
				},
			}

			committees := committeesAtSlot
			if test.committeesAtSlot != 0 {
				committees = test.committeesAtSlot
			}
			actualResponse, err := validatorClient.submitAggregateSelectionProof(ctx, &ethpb.AggregateSelectionRequest{
				Slot:           slot,
				CommitteeIndex: committeeIndex,
				PublicKey:      pubkey,
				SlotSignature:  slotSignatureBytes,
			}, validatorIndex, committees)
			if test.expectedErrorMsg == "" {
				require.NoError(t, err)
				assert.DeepEqual(t, expectedResponse, actualResponse)
			} else {
				require.ErrorContains(t, test.expectedErrorMsg, err)
			}
		})
	}
}

func TestSubmitAggregateSelectionProofElectra(t *testing.T) {
	const (
		pubkeyStr                    = "0x8000091c2ae64ee414a54c1cc1fc67dec663408bc636cb86756e0200e41a75c8f86603f104f02c856983d2783116be13"
		syncingEndpoint              = "/eth/v1/node/syncing"
		attestationDataEndpoint      = "/eth/v1/validator/attestation_data"
		aggregateAttestationEndpoint = "/eth/v2/validator/aggregate_attestation"
		validatorIndex               = primitives.ValidatorIndex(55293)
		slotSignature                = "0x8776a37d6802c4797d113169c5fcfda50e68a32058eb6356a6f00d06d7da64c841a00c7c38b9b94a204751eca53707bd03523ce4797827d9bacff116a6e776a20bbccff4b683bf5201b610797ed0502557a58a65c8395f8a1649b976c3112d15"
		slot                         = primitives.Slot(123)
		committeeIndex               = primitives.CommitteeIndex(1)
		committeesAtSlot             = uint64(1)
	)

	attestationDataResponse := generateValidAttestation(uint64(slot), uint64(committeeIndex))
	attestationDataProto, err := attestationDataResponse.Data.ToConsensus()
	require.NoError(t, err)
	attestationDataRootBytes, err := attestationDataProto.HashTreeRoot()
	require.NoError(t, err)

	aggregateAttestation := &ethpb.AttestationElectra{
		AggregationBits: testhelpers.FillByteSlice(4, 74),
		Data:            attestationDataProto,
		Signature:       testhelpers.FillByteSlice(96, 82),
		CommitteeBits:   testhelpers.FillByteSlice(8, 83),
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		name                       string
		isOptimistic               bool
		syncingErr                 error
		attestationDataErr         error
		aggregateAttestationErr    error
		attestationDataCalled      int
		aggregateAttestationCalled int
		expectedErrorMsg           string
		committeesAtSlot           uint64
	}{
		{
			name:                       "success",
			attestationDataCalled:      1,
			aggregateAttestationCalled: 1,
		},
		{
			name:             "head is optimistic",
			isOptimistic:     true,
			expectedErrorMsg: "the node is currently optimistic and cannot serve validators",
		},
		{
			name:             "syncing error",
			syncingErr:       errors.New("bad request"),
			expectedErrorMsg: "failed to get syncing status",
		},
		{
			name:                  "attestation data error",
			attestationDataCalled: 1,
			attestationDataErr:    errors.New("bad request"),
			expectedErrorMsg:      fmt.Sprintf("failed to get attestation data for slot=%d and committee_index=%d", slot, committeeIndex),
		},
		{
			name:                       "aggregate attestation error",
			attestationDataCalled:      1,
			aggregateAttestationCalled: 1,
			aggregateAttestationErr:    errors.New("bad request"),
			expectedErrorMsg:           "bad request",
		},
		{
			name:             "validator is not an aggregator",
			committeesAtSlot: 64,
			expectedErrorMsg: "validator is not an aggregator",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := t.Context()
			handler := mock.NewMockHandler(ctrl)

			// Call node syncing endpoint to check if head is optimistic.
			handler.EXPECT().Get(
				gomock.Any(),
				syncingEndpoint,
				&structs.SyncStatusResponse{},
			).SetArg(
				2,
				structs.SyncStatusResponse{
					Data: &structs.SyncStatusResponseData{
						IsOptimistic: test.isOptimistic,
					},
				},
			).Return(
				test.syncingErr,
			).Times(1)

			// Call attestation data to get attestation data root to query aggregate attestation.
			handler.EXPECT().Get(
				gomock.Any(),
				fmt.Sprintf("%s?committee_index=%d&slot=%d", attestationDataEndpoint, committeeIndex, slot),
				&structs.GetAttestationDataResponse{},
			).SetArg(
				2,
				attestationDataResponse,
			).Return(
				test.attestationDataErr,
			).Times(test.attestationDataCalled)

			attestationJSON, err := json.Marshal(jsonifyAttestationElectra(aggregateAttestation))
			require.NoError(t, err)

			// Call attestation data to get attestation data root to query aggregate attestation.
			handler.EXPECT().Get(
				gomock.Any(),
				fmt.Sprintf("%s?attestation_data_root=%s&committee_index=%d&slot=%d", aggregateAttestationEndpoint, hexutil.Encode(attestationDataRootBytes[:]), committeeIndex, slot),
				&structs.AggregateAttestationResponse{},
			).SetArg(
				2,
				structs.AggregateAttestationResponse{
					Data: attestationJSON,
				},
			).Return(
				test.aggregateAttestationErr,
			).Times(test.aggregateAttestationCalled)

			pubkey, err := hexutil.Decode(pubkeyStr)
			require.NoError(t, err)

			slotSignatureBytes, err := hexutil.Decode(slotSignature)
			require.NoError(t, err)

			expectedResponse := &ethpb.AggregateSelectionElectraResponse{
				AggregateAndProof: &ethpb.AggregateAttestationAndProofElectra{
					AggregatorIndex: primitives.ValidatorIndex(55293),
					Aggregate:       aggregateAttestation,
					SelectionProof:  slotSignatureBytes,
				},
			}

			validatorClient := &beaconApiValidatorClient{
				handler: handler,
				stateValidatorsProvider: beaconApiStateValidatorsProvider{
					handler: handler,
				},
				dutiesProvider: beaconApiDutiesProvider{
					handler: handler,
				},
			}

			committees := committeesAtSlot
			if test.committeesAtSlot != 0 {
				committees = test.committeesAtSlot
			}
			actualResponse, err := validatorClient.submitAggregateSelectionProofElectra(ctx, &ethpb.AggregateSelectionRequest{
				Slot:           slot,
				CommitteeIndex: committeeIndex,
				PublicKey:      pubkey,
				SlotSignature:  slotSignatureBytes,
			}, validatorIndex, committees)
			if test.expectedErrorMsg == "" {
				require.NoError(t, err)
				assert.DeepEqual(t, expectedResponse, actualResponse)
			} else {
				require.ErrorContains(t, test.expectedErrorMsg, err)
			}
		})
	}
}

func TestSubmitAggregateSelectionProof_UsesSubmittedAttestationData(t *testing.T) {
	const (
		syncingEndpoint              = "/eth/v1/node/syncing"
		aggregateAttestationEndpoint = "/eth/v2/validator/aggregate_attestation"
		slot                         = primitives.Slot(123)
		committee                    = primitives.CommitteeIndex(7)
	)
	// attestationData builds the signed data: before Electra the index is the committee, in Gloas it is the payload status.
	attestationData := func(t *testing.T, index uint64) *ethpb.AttestationData {
		data, err := generateValidAttestation(uint64(slot), index).Data.ToConsensus()
		require.NoError(t, err)
		return data
	}

	// expectAggregation fails the test on any attestation_data call, so the aggregate must use the submitted root.
	expectAggregation := func(t *testing.T, handler *mock.MockHandler, postErr error, signed *ethpb.AttestationData, aggregate any) {
		signedRoot, err := signed.HashTreeRoot()
		require.NoError(t, err)
		handler.EXPECT().Post(gomock.Any(), "/eth/v2/beacon/pool/attestations", gomock.Any(), gomock.Any(), nil).Return(postErr).Times(1)
		handler.EXPECT().Get(gomock.Any(), syncingEndpoint, &structs.SyncStatusResponse{}).
			SetArg(2, structs.SyncStatusResponse{Data: &structs.SyncStatusResponseData{}}).Return(nil).Times(1)
		aggregateJSON, err := json.Marshal(aggregate)
		require.NoError(t, err)
		handler.EXPECT().Get(
			gomock.Any(),
			fmt.Sprintf("%s?attestation_data_root=%s&committee_index=%d&slot=%d", aggregateAttestationEndpoint, hexutil.Encode(signedRoot[:]), committee, slot),
			&structs.AggregateAttestationResponse{},
		).SetArg(2, structs.AggregateAttestationResponse{Data: aggregateJSON}).Return(nil).Times(1)
	}

	// runPhase0 submits an Attestation, whose data index is the committee.
	runPhase0 := func(t *testing.T, postErr error) {
		ctrl := gomock.NewController(t)
		handler := mock.NewMockHandler(ctrl)
		signed := attestationData(t, uint64(committee))
		att := &ethpb.Attestation{AggregationBits: testhelpers.FillByteSlice(4, 74), Data: signed, Signature: testhelpers.FillByteSlice(96, 82)}
		expectAggregation(t, handler, postErr, signed, jsonifyAttestation(att))

		validatorClient := &beaconApiValidatorClient{handler: handler}
		_, err := validatorClient.proposeAttestation(t.Context(), att)
		require.Equal(t, postErr, err)
		resp, err := validatorClient.submitAggregateSelectionProof(t.Context(), &ethpb.AggregateSelectionRequest{Slot: slot, CommitteeIndex: committee, SlotSignature: testhelpers.FillByteSlice(96, 1)}, 0, 1)
		require.NoError(t, err)
		assert.DeepEqual(t, signed, resp.AggregateAndProof.Aggregate.Data)
	}

	// runSingle submits a SingleAttestation whose data index differs from the committee id, so a memo keyed by the data index misses.
	runSingle := func(t *testing.T, postErr error, signed *ethpb.AttestationData) {
		ctrl := gomock.NewController(t)
		handler := mock.NewMockHandler(ctrl)
		single := &ethpb.SingleAttestation{CommitteeId: committee, AttesterIndex: 1, Data: signed, Signature: testhelpers.FillByteSlice(96, 82)}
		aggregate := &ethpb.AttestationElectra{
			AggregationBits: testhelpers.FillByteSlice(4, 74),
			Data:            signed,
			Signature:       testhelpers.FillByteSlice(96, 82),
			CommitteeBits:   testhelpers.FillByteSlice(8, 83),
		}
		expectAggregation(t, handler, postErr, signed, jsonifyAttestationElectra(aggregate))

		validatorClient := &beaconApiValidatorClient{handler: handler}
		_, err := validatorClient.proposeAttestationElectra(t.Context(), single)
		require.Equal(t, postErr, err)
		resp, err := validatorClient.submitAggregateSelectionProofElectra(t.Context(), &ethpb.AggregateSelectionRequest{Slot: slot, CommitteeIndex: committee, SlotSignature: testhelpers.FillByteSlice(96, 1)}, 0, 1)
		require.NoError(t, err)
		assert.DeepEqual(t, signed, resp.AggregateAndProof.Aggregate.Data)
	}

	// The validator signed the data before the POST, so a refused submission must not change the aggregate root.
	submissions := []struct {
		name    string
		postErr error
	}{
		{name: "accepted"},
		{name: "refused", postErr: errors.New("bad request")},
	}

	t.Run("phase0", func(t *testing.T) {
		for _, submission := range submissions {
			t.Run(submission.name, func(t *testing.T) {
				runPhase0(t, submission.postErr)
			})
		}
	})

	t.Run("electra", func(t *testing.T) {
		for _, submission := range submissions {
			t.Run(submission.name, func(t *testing.T) {
				runSingle(t, submission.postErr, attestationData(t, 0))
			})
		}
	})

	t.Run("gloas", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig().Copy()
		cfg.GloasForkEpoch = 0
		params.OverrideBeaconConfig(cfg)
		for _, submission := range submissions {
			t.Run(submission.name, func(t *testing.T) {
				// In Gloas the data index is the payload status.
				runSingle(t, submission.postErr, attestationData(t, 1))
			})
		}
	})
}
