package beacon_api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"testing"

	"github.com/OffchainLabs/prysm/v6/api/apiutil"
	"github.com/OffchainLabs/prysm/v6/api/server/structs"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/OffchainLabs/prysm/v6/validator/client/beacon-api/mock"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/pkg/errors"
	"go.uber.org/mock/gomock"
)

func TestSubmitSyncMessage_Valid(t *testing.T) {
	const beaconBlockRoot = "0x719d4f66a5f25c35d93718821aacb342194391034b11cf0a5822cc249178a274"
	const signature = "0xb459ef852bd4e0cb96e6723d67cacc8215406dd9ba663f8874a083167ebf428b28b746431bdbc1820a25289377b2610881e52b3a05c3548c5e99c08c8a36342573be5962d7510c03dcba8ddfb8ae419e59d222ddcf31cc512e704ef2cc3cf8"

	decodedBeaconBlockRoot, err := hexutil.Decode(beaconBlockRoot)
	require.NoError(t, err)

	decodedSignature, err := hexutil.Decode(signature)
	require.NoError(t, err)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	jsonSyncCommitteeMessage := &structs.SyncCommitteeMessage{
		Slot:            "42",
		BeaconBlockRoot: beaconBlockRoot,
		ValidatorIndex:  "12345",
		Signature:       signature,
	}

	marshalledJsonRegistrations, err := json.Marshal([]*structs.SyncCommitteeMessage{jsonSyncCommitteeMessage})
	require.NoError(t, err)

	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().Post(
		gomock.Any(),
		"/eth/v1/beacon/pool/sync_committees",
		nil,
		bytes.NewBuffer(marshalledJsonRegistrations),
		nil,
	).Return(
		nil,
	).Times(1)

	protoSyncCommitteeMessage := ethpb.SyncCommitteeMessage{
		Slot:           primitives.Slot(42),
		BlockRoot:      decodedBeaconBlockRoot,
		ValidatorIndex: primitives.ValidatorIndex(12345),
		Signature:      decodedSignature,
	}

	validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	res, err := validatorClient.SubmitSyncMessage(t.Context(), &protoSyncCommitteeMessage)

	assert.DeepEqual(t, new(empty.Empty), res)
	require.NoError(t, err)
}

func TestSubmitSyncMessage_BadRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().Post(
		gomock.Any(),
		"/eth/v1/beacon/pool/sync_committees",
		nil,
		gomock.Any(),
		nil,
	).Return(
		errors.New("foo error"),
	).Times(1)

	validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	_, err := validatorClient.SubmitSyncMessage(t.Context(), &ethpb.SyncCommitteeMessage{})
	assert.ErrorContains(t, "foo error", err)
}

func TestGetSyncMessageBlockRoot(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	const blockRoot = "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2"
	tests := []struct {
		name                 string
		endpointError        error
		expectedErrorMessage string
		expectedResponse     structs.BlockRootResponse
	}{
		{
			name: "valid request",
			expectedResponse: structs.BlockRootResponse{
				Data: &structs.BlockRoot{
					Root: blockRoot,
				},
			},
		},
		{
			name:                 "internal server error",
			expectedErrorMessage: "internal server error",
			endpointError:        errors.New("internal server error"),
		},
		{
			name: "execution optimistic",
			expectedResponse: structs.BlockRootResponse{
				ExecutionOptimistic: true,
			},
			expectedErrorMessage: "the node is currently optimistic and cannot serve validators",
		},
		{
			name:                 "no data",
			expectedResponse:     structs.BlockRootResponse{},
			expectedErrorMessage: "no data returned",
		},
		{
			name: "no root",
			expectedResponse: structs.BlockRootResponse{
				Data: new(structs.BlockRoot),
			},
			expectedErrorMessage: "no root returned",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := t.Context()
			jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
			jsonRestHandler.EXPECT().Get(
				gomock.Any(),
				"/eth/v1/beacon/blocks/head/root",
				&structs.BlockRootResponse{},
			).SetArg(
				2,
				test.expectedResponse,
			).Return(
				test.endpointError,
			).Times(1)

			validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
			actualResponse, err := validatorClient.syncMessageBlockRoot(ctx)
			if test.expectedErrorMessage != "" {
				require.ErrorContains(t, test.expectedErrorMessage, err)
				return
			}

			require.NoError(t, err)

			expectedRootBytes, err := hexutil.Decode(test.expectedResponse.Data.Root)
			require.NoError(t, err)

			require.NoError(t, err)
			require.DeepEqual(t, expectedRootBytes, actualResponse.Root)
		})
	}
}

func TestGetSyncCommitteeContribution(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	const blockRoot = "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2"

	request := &ethpb.SyncCommitteeContributionRequest{
		Slot:      primitives.Slot(1),
		PublicKey: nil,
		SubnetId:  1,
	}

	contributionJson := &structs.SyncCommitteeContribution{
		Slot:              "1",
		BeaconBlockRoot:   blockRoot,
		SubcommitteeIndex: "1",
		AggregationBits:   "0x01",
		Signature:         "0x1b66ac1fb663c9bc59509846d6ec05345bd908eda73e670af888da41af171505cc411d61252fb6cb3fa0017b679f8bb2305b26a285fa2737f175668d0dff91cc1b66ac1fb663c9bc59509846d6ec05345bd908eda73e670af888da41af171505",
	}

	tests := []struct {
		name           string
		contribution   structs.ProduceSyncCommitteeContributionResponse
		endpointErr    error
		expectedErrMsg string
	}{
		{
			name:         "valid request",
			contribution: structs.ProduceSyncCommitteeContributionResponse{Data: contributionJson},
		},
		{
			name:           "bad request",
			endpointErr:    errors.New("internal server error"),
			expectedErrMsg: "internal server error",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := t.Context()
			jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
			jsonRestHandler.EXPECT().Get(
				gomock.Any(),
				"/eth/v1/beacon/blocks/head/root",
				&structs.BlockRootResponse{},
			).SetArg(
				2,
				structs.BlockRootResponse{
					Data: &structs.BlockRoot{
						Root: blockRoot,
					},
				},
			).Return(
				nil,
			).Times(1)

			jsonRestHandler.EXPECT().Get(
				gomock.Any(),
				fmt.Sprintf("/eth/v1/validator/sync_committee_contribution?beacon_block_root=%s&slot=%d&subcommittee_index=%d",
					blockRoot, uint64(request.Slot), request.SubnetId),
				&structs.ProduceSyncCommitteeContributionResponse{},
			).SetArg(
				2,
				test.contribution,
			).Return(
				test.endpointErr,
			).Times(1)

			validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
			actualResponse, err := validatorClient.syncCommitteeContribution(ctx, request)
			if test.expectedErrMsg != "" {
				require.ErrorContains(t, test.expectedErrMsg, err)
				return
			}
			require.NoError(t, err)

			expectedResponse, err := convertSyncContributionJsonToProto(test.contribution.Data)
			require.NoError(t, err)
			assert.DeepEqual(t, expectedResponse, actualResponse)
		})
	}
}

func TestGetSyncSubCommitteeIndex(t *testing.T) {
	const (
		pubkeyStr          = "0x8000091c2ae64ee414a54c1cc1fc67dec663408bc636cb86756e0200e41a75c8f86603f104f02c856983d2783116be13"
		syncDutiesEndpoint = "/eth/v1/validator/duties/sync"
		validatorsEndpoint = "/eth/v1/beacon/states/head/validators"
		validatorIndex     = "55293"
		slot               = primitives.Slot(123)
	)

	expectedResponse := &ethpb.SyncSubcommitteeIndexResponse{
		Indices: []primitives.CommitteeIndex{123, 456},
	}

	syncDuties := []*structs.SyncCommitteeDuty{
		{
			Pubkey:         hexutil.Encode([]byte{1}),
			ValidatorIndex: validatorIndex,
			ValidatorSyncCommitteeIndices: []string{
				"123",
				"456",
			},
		},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		name             string
		duties           []*structs.SyncCommitteeDuty
		validatorsErr    error
		dutiesErr        error
		expectedErrorMsg string
	}{
		{
			name:   "success",
			duties: syncDuties,
		},
		{
			name:             "no sync duties",
			duties:           []*structs.SyncCommitteeDuty{},
			expectedErrorMsg: fmt.Sprintf("no sync committee duty for the given slot %d", slot),
		},
		{
			name:             "duties endpoint error",
			dutiesErr:        errors.New("bad request"),
			expectedErrorMsg: "bad request",
		},
		{
			name:             "validator index endpoint error",
			validatorsErr:    errors.New("bad request"),
			expectedErrorMsg: "bad request",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := t.Context()
			valsReq := &structs.GetValidatorsRequest{
				Ids:      []string{pubkeyStr},
				Statuses: []string{},
			}
			valsReqBytes, err := json.Marshal(valsReq)
			require.NoError(t, err)
			jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
			jsonRestHandler.EXPECT().Post(
				gomock.Any(),
				validatorsEndpoint,
				nil,
				bytes.NewBuffer(valsReqBytes),
				&structs.GetValidatorsResponse{},
			).SetArg(
				4,
				structs.GetValidatorsResponse{
					Data: []*structs.ValidatorContainer{
						{
							Index:  validatorIndex,
							Status: "active_ongoing",
							Validator: &structs.Validator{
								Pubkey: stringPubKey,
							},
						},
					},
				},
			).Return(
				test.validatorsErr,
			).Times(1)

			if test.validatorsErr != nil {
				// Then try the GET call which will also return error.
				queryParams := url.Values{}
				for _, id := range valsReq.Ids {
					queryParams.Add("id", id)
				}
				for _, st := range valsReq.Statuses {
					queryParams.Add("status", st)
				}

				query := apiutil.BuildURL("/eth/v1/beacon/states/head/validators", queryParams)

				jsonRestHandler.EXPECT().Get(
					gomock.Any(),
					query,
					&structs.GetValidatorsResponse{},
				).Return(
					test.validatorsErr,
				).Times(1)
			}

			validatorIndicesBytes, err := json.Marshal([]string{validatorIndex})
			require.NoError(t, err)

			var syncDutiesCalled int
			if test.validatorsErr == nil {
				syncDutiesCalled = 1
			}

			jsonRestHandler.EXPECT().Post(
				gomock.Any(),
				fmt.Sprintf("%s/%d", syncDutiesEndpoint, slots.ToEpoch(slot)),
				nil,
				bytes.NewBuffer(validatorIndicesBytes),
				&structs.GetSyncCommitteeDutiesResponse{},
			).SetArg(
				4,
				structs.GetSyncCommitteeDutiesResponse{
					Data: test.duties,
				},
			).Return(
				test.dutiesErr,
			).Times(syncDutiesCalled)

			pubkey, err := hexutil.Decode(pubkeyStr)
			require.NoError(t, err)

			validatorClient := &beaconApiValidatorClient{
				jsonRestHandler: jsonRestHandler,
				stateValidatorsProvider: beaconApiStateValidatorsProvider{
					jsonRestHandler: jsonRestHandler,
				},
				dutiesProvider: beaconApiDutiesProvider{
					jsonRestHandler: jsonRestHandler,
				},
			}
			actualResponse, err := validatorClient.syncSubcommitteeIndex(ctx, &ethpb.SyncSubcommitteeIndexRequest{
				PublicKey: pubkey,
				Slot:      slot,
			})
			if test.expectedErrorMsg == "" {
				require.NoError(t, err)
				assert.DeepEqual(t, expectedResponse, actualResponse)
			} else {
				require.ErrorContains(t, test.expectedErrorMsg, err)
			}
		})
	}
}
