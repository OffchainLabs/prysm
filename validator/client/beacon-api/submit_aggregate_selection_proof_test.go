package beacon_api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/prysmaticlabs/prysm/v5/api/server/structs"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/network/httputil"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/validator/client/beacon-api/mock"
	testhelpers "github.com/prysmaticlabs/prysm/v5/validator/client/beacon-api/test-helpers"
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
			ctx := context.Background()
			jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)

			// Call node syncing endpoint to check if head is optimistic.
			jsonRestHandler.EXPECT().Get(
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
			jsonRestHandler.EXPECT().Get(
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
			jsonRestHandler.EXPECT().Get(
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
				jsonRestHandler: jsonRestHandler,
				stateValidatorsProvider: beaconApiStateValidatorsProvider{
					jsonRestHandler: jsonRestHandler,
				},
				dutiesProvider: beaconApiDutiesProvider{
					jsonRestHandler: jsonRestHandler,
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

func TestSubmitAggregateSelectionProofFallBack(t *testing.T) {
	const (
		pubkeyStr                      = "0x8000091c2ae64ee414a54c1cc1fc67dec663408bc636cb86756e0200e41a75c8f86603f104f02c856983d2783116be13"
		syncingEndpoint                = "/eth/v1/node/syncing"
		attestationDataEndpoint        = "/eth/v1/validator/attestation_data"
		aggregateAttestationEndpoint   = "/eth/v1/validator/aggregate_attestation"
		aggregateAttestationV2Endpoint = "/eth/v2/validator/aggregate_attestation"
		validatorIndex                 = primitives.ValidatorIndex(55293)
		slotSignature                  = "0x8776a37d6802c4797d113169c5fcfda50e68a32058eb6356a6f00d06d7da64c841a00c7c38b9b94a204751eca53707bd03523ce4797827d9bacff116a6e776a20bbccff4b683bf5201b610797ed0502557a58a65c8395f8a1649b976c3112d15"
		slot                           = primitives.Slot(123)
		committeeIndex                 = primitives.CommitteeIndex(1)
		committeesAtSlot               = uint64(1)
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
	ctx := context.Background()
	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)

	// Call node syncing endpoint to check if head is optimistic.
	jsonRestHandler.EXPECT().Get(
		gomock.Any(),
		syncingEndpoint,
		&structs.SyncStatusResponse{},
	).SetArg(
		2,
		structs.SyncStatusResponse{
			Data: &structs.SyncStatusResponseData{
				IsOptimistic: false,
			},
		},
	).Return(
		nil,
	).Times(1)

	// Call attestation data to get attestation data root to query aggregate attestation.
	jsonRestHandler.EXPECT().Get(
		gomock.Any(),
		fmt.Sprintf("%s?committee_index=%d&slot=%d", attestationDataEndpoint, committeeIndex, slot),
		&structs.GetAttestationDataResponse{},
	).SetArg(
		2,
		attestationDataResponse,
	).Return(
		nil,
	).Times(1)

	attestationJSON, err := json.Marshal(jsonifyAttestation(aggregateAttestation))
	require.NoError(t, err)

	// Call attestation data to get attestation data root to query aggregate attestation.
	jsonRestHandler.EXPECT().Get(
		gomock.Any(),
		fmt.Sprintf("%s?attestation_data_root=%s&committee_index=%d&slot=%d", aggregateAttestationV2Endpoint, hexutil.Encode(attestationDataRootBytes[:]), committeeIndex, slot),
		&structs.AggregateAttestationResponse{},
	).Return(
		&httputil.DefaultJsonError{
			Code: http.StatusNotFound,
		},
	).Times(1)

	// Call attestation data to get attestation data root to query aggregate attestation.
	jsonRestHandler.EXPECT().Get(
		gomock.Any(),
		fmt.Sprintf("%s?attestation_data_root=%s&slot=%d", aggregateAttestationEndpoint, hexutil.Encode(attestationDataRootBytes[:]), slot),
		&structs.AggregateAttestationResponse{},
	).SetArg(
		2,
		structs.AggregateAttestationResponse{
			Data: attestationJSON,
		},
	).Return(
		nil,
	).Times(1)

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
		jsonRestHandler: jsonRestHandler,
		stateValidatorsProvider: beaconApiStateValidatorsProvider{
			jsonRestHandler: jsonRestHandler,
		},
		dutiesProvider: beaconApiDutiesProvider{
			jsonRestHandler: jsonRestHandler,
		},
	}

	actualResponse, err := validatorClient.submitAggregateSelectionProof(ctx, &ethpb.AggregateSelectionRequest{
		Slot:           slot,
		CommitteeIndex: committeeIndex,
		PublicKey:      pubkey,
		SlotSignature:  slotSignatureBytes,
	}, validatorIndex, committeesAtSlot)

	require.NoError(t, err)
	assert.DeepEqual(t, expectedResponse, actualResponse)

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
			ctx := context.Background()
			jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)

			// Call node syncing endpoint to check if head is optimistic.
			jsonRestHandler.EXPECT().Get(
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
			jsonRestHandler.EXPECT().Get(
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
			jsonRestHandler.EXPECT().Get(
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
				jsonRestHandler: jsonRestHandler,
				stateValidatorsProvider: beaconApiStateValidatorsProvider{
					jsonRestHandler: jsonRestHandler,
				},
				dutiesProvider: beaconApiDutiesProvider{
					jsonRestHandler: jsonRestHandler,
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
