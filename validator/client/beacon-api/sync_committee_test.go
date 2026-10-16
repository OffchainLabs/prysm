package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/OffchainLabs/prysm/v7/api/apiutil"
	"github.com/OffchainLabs/prysm/v7/api/rest"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/OffchainLabs/prysm/v7/validator/client/beacon-api/mock"
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

	handler := mock.NewMockHandler(ctrl)
	handler.EXPECT().Post(
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

	validatorClient := &beaconApiValidatorClient{handler: handler}
	res, err := validatorClient.SubmitSyncMessage(t.Context(), &protoSyncCommitteeMessage)

	assert.DeepEqual(t, new(empty.Empty), res)
	require.NoError(t, err)
}

func TestSubmitSyncMessage_BadRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	handler := mock.NewMockHandler(ctrl)
	handler.EXPECT().Post(
		gomock.Any(),
		"/eth/v1/beacon/pool/sync_committees",
		nil,
		gomock.Any(),
		nil,
	).Return(
		errors.New("foo error"),
	).Times(1)

	validatorClient := &beaconApiValidatorClient{handler: handler}
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
			handler := mock.NewMockHandler(ctrl)
			handler.EXPECT().Get(
				gomock.Any(),
				"/eth/v1/beacon/blocks/head/root",
				&structs.BlockRootResponse{},
			).SetArg(
				2,
				test.expectedResponse,
			).Return(
				test.endpointError,
			).Times(1)

			validatorClient := &beaconApiValidatorClient{handler: handler}
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
			handler := mock.NewMockHandler(ctrl)
			handler.EXPECT().Get(
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

			handler.EXPECT().Get(
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

			validatorClient := &beaconApiValidatorClient{handler: handler}
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

func TestGetSyncCommitteeContribution_UsesSubmittedSyncMessageRoot(t *testing.T) {
	const (
		aggregatorRoot = "0x719d4f66a5f25c35d93718821aacb342194391034b11cf0a5822cc249178a274"
		otherRoot      = "0x5ecf3bff35e39d5f75476d42950d549f81fa93038c46b6652ae89ae1f7ad834f"
		headRoot       = "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2"
		syncPoolPath   = "/eth/v1/beacon/pool/sync_committees"
	)
	pubkey := make([]byte, 48)
	decode := func(root string) []byte {
		b, err := hexutil.Decode(root)
		require.NoError(t, err)
		return b
	}
	contributionPath := func(root string, slot int) string {
		return fmt.Sprintf("/eth/v1/validator/sync_committee_contribution?beacon_block_root=%s&slot=%d&subcommittee_index=1", root, slot)
	}
	expectContribution := func(handler *mock.MockHandler, root string, slot int) {
		data := &structs.SyncCommitteeContribution{
			Slot:              strconv.Itoa(slot),
			BeaconBlockRoot:   root,
			SubcommitteeIndex: "1",
			AggregationBits:   "0x01",
			Signature:         "0x1b66ac1fb663c9bc59509846d6ec05345bd908eda73e670af888da41af171505cc411d61252fb6cb3fa0017b679f8bb2305b26a285fa2737f175668d0dff91cc1b66ac1fb663c9bc59509846d6ec05345bd908eda73e670af888da41af171505",
		}
		handler.EXPECT().Get(gomock.Any(), contributionPath(root, slot), &structs.ProduceSyncCommitteeContributionResponse{}).
			SetArg(2, structs.ProduceSyncCommitteeContributionResponse{Data: data}).Return(nil).Times(1)
	}
	expectHead := func(handler *mock.MockHandler) {
		handler.EXPECT().Get(gomock.Any(), "/eth/v1/beacon/blocks/head/root", &structs.BlockRootResponse{}).
			SetArg(2, structs.BlockRootResponse{Data: &structs.BlockRoot{Root: headRoot}}).Return(nil).Times(1)
	}
	expectIndex := func(ctrl *gomock.Controller, resp *structs.GetValidatorsResponse) *mock.MockStateValidatorsProvider {
		provider := mock.NewMockStateValidatorsProvider(ctrl)
		provider.EXPECT().StateValidators(gomock.Any(), []string{hexutil.Encode(pubkey)}, nil, nil).Return(resp, nil).Times(1)
		return provider
	}
	expectSyncing := func(handler *mock.MockHandler, optimistic bool) {
		handler.EXPECT().Get(gomock.Any(), "/eth/v1/node/syncing", &structs.SyncStatusResponse{}, gomock.Any(), gomock.Any()).
			SetArg(2, structs.SyncStatusResponse{Data: &structs.SyncStatusResponseData{IsOptimistic: optimistic}}).Return(nil).Times(1)
	}
	aggregatorIndex := &structs.GetValidatorsResponse{Data: []*structs.ValidatorContainer{{Index: "7"}}}

	t.Run("keys that signed different roots use the aggregator's own root", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		handler := mock.NewMockHandler(ctrl)
		handler.EXPECT().Post(gomock.Any(), syncPoolPath, nil, gomock.Any(), nil).Return(nil).Times(2)
		expectSyncing(handler, false)
		expectContribution(handler, aggregatorRoot, 2)

		validatorClient := &beaconApiValidatorClient{handler: handler, stateValidatorsProvider: expectIndex(ctrl, aggregatorIndex)}
		require.NoError(t, validatorClient.submitSyncMessage(t.Context(), &ethpb.SyncCommitteeMessage{Slot: 2, ValidatorIndex: 7, BlockRoot: decode(aggregatorRoot)}))
		require.NoError(t, validatorClient.submitSyncMessage(t.Context(), &ethpb.SyncCommitteeMessage{Slot: 2, ValidatorIndex: 8, BlockRoot: decode(otherRoot)}))
		contribution, err := validatorClient.syncCommitteeContribution(t.Context(), &ethpb.SyncCommitteeContributionRequest{Slot: 2, PublicKey: pubkey, SubnetId: 1})
		require.NoError(t, err)
		assert.DeepEqual(t, decode(aggregatorRoot), contribution.BlockRoot)
	})

	t.Run("keys that signed one root use it without the index lookup", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		handler := mock.NewMockHandler(ctrl)
		handler.EXPECT().Post(gomock.Any(), syncPoolPath, nil, gomock.Any(), nil).Return(nil).Times(2)
		expectSyncing(handler, false)
		expectContribution(handler, aggregatorRoot, 2)

		validatorClient := &beaconApiValidatorClient{handler: handler, stateValidatorsProvider: mock.NewMockStateValidatorsProvider(ctrl)}
		require.NoError(t, validatorClient.submitSyncMessage(t.Context(), &ethpb.SyncCommitteeMessage{Slot: 2, ValidatorIndex: 7, BlockRoot: decode(aggregatorRoot)}))
		require.NoError(t, validatorClient.submitSyncMessage(t.Context(), &ethpb.SyncCommitteeMessage{Slot: 2, ValidatorIndex: 8, BlockRoot: decode(aggregatorRoot)}))
		contribution, err := validatorClient.syncCommitteeContribution(t.Context(), &ethpb.SyncCommitteeContributionRequest{Slot: 2, PublicKey: pubkey, SubnetId: 1})
		require.NoError(t, err)
		assert.DeepEqual(t, decode(aggregatorRoot), contribution.BlockRoot)
	})

	t.Run("optimistic node is refused before the root is used", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		handler := mock.NewMockHandler(ctrl)
		handler.EXPECT().Post(gomock.Any(), syncPoolPath, nil, gomock.Any(), nil).Return(nil).Times(1)
		expectSyncing(handler, true)

		validatorClient := &beaconApiValidatorClient{handler: handler, stateValidatorsProvider: mock.NewMockStateValidatorsProvider(ctrl)}
		require.NoError(t, validatorClient.submitSyncMessage(t.Context(), &ethpb.SyncCommitteeMessage{Slot: 2, ValidatorIndex: 7, BlockRoot: decode(aggregatorRoot)}))
		_, err := validatorClient.syncCommitteeContribution(t.Context(), &ethpb.SyncCommitteeContributionRequest{Slot: 2, PublicKey: pubkey, SubnetId: 1})
		require.ErrorContains(t, "optimistic", err)
	})

	t.Run("aggregator without a sync message in a slot with different roots falls back to the head", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		handler := mock.NewMockHandler(ctrl)
		handler.EXPECT().Post(gomock.Any(), syncPoolPath, nil, gomock.Any(), nil).Return(nil).Times(2)
		expectSyncing(handler, false)
		expectHead(handler)
		expectContribution(handler, headRoot, 2)

		validatorClient := &beaconApiValidatorClient{handler: handler, stateValidatorsProvider: expectIndex(ctrl, aggregatorIndex)}
		require.NoError(t, validatorClient.submitSyncMessage(t.Context(), &ethpb.SyncCommitteeMessage{Slot: 2, ValidatorIndex: 8, BlockRoot: decode(otherRoot)}))
		require.NoError(t, validatorClient.submitSyncMessage(t.Context(), &ethpb.SyncCommitteeMessage{Slot: 2, ValidatorIndex: 9, BlockRoot: decode(aggregatorRoot)}))
		_, err := validatorClient.syncCommitteeContribution(t.Context(), &ethpb.SyncCommitteeContributionRequest{Slot: 2, PublicKey: pubkey, SubnetId: 1})
		require.NoError(t, err)
	})

	t.Run("failed submission is still recorded", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		handler := mock.NewMockHandler(ctrl)
		handler.EXPECT().Post(gomock.Any(), syncPoolPath, nil, gomock.Any(), nil).Return(errors.New("bad request")).Times(1)
		expectSyncing(handler, false)
		expectContribution(handler, aggregatorRoot, 2)

		validatorClient := &beaconApiValidatorClient{handler: handler, stateValidatorsProvider: mock.NewMockStateValidatorsProvider(ctrl)}
		require.ErrorContains(t, "bad request", validatorClient.submitSyncMessage(t.Context(), &ethpb.SyncCommitteeMessage{Slot: 2, ValidatorIndex: 7, BlockRoot: decode(aggregatorRoot)}))
		contribution, err := validatorClient.syncCommitteeContribution(t.Context(), &ethpb.SyncCommitteeContributionRequest{Slot: 2, PublicKey: pubkey, SubnetId: 1})
		require.NoError(t, err)
		assert.DeepEqual(t, decode(aggregatorRoot), contribution.BlockRoot)
	})

	t.Run("root is recorded before the POST returns", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		handler := mock.NewMockHandler(ctrl)
		posted, release := make(chan struct{}), make(chan struct{})
		handler.EXPECT().Post(gomock.Any(), syncPoolPath, nil, gomock.Any(), nil).DoAndReturn(
			func(context.Context, string, map[string]string, *bytes.Buffer, any) error {
				close(posted)
				<-release
				return nil
			}).Times(1)
		expectSyncing(handler, false)
		expectContribution(handler, aggregatorRoot, 2)

		validatorClient := &beaconApiValidatorClient{handler: handler, stateValidatorsProvider: mock.NewMockStateValidatorsProvider(ctrl)}
		submitted := make(chan error)
		go func() {
			submitted <- validatorClient.submitSyncMessage(t.Context(), &ethpb.SyncCommitteeMessage{Slot: 2, ValidatorIndex: 7, BlockRoot: decode(aggregatorRoot)})
		}()
		<-posted
		contribution, err := validatorClient.syncCommitteeContribution(t.Context(), &ethpb.SyncCommitteeContributionRequest{Slot: 2, PublicKey: pubkey, SubnetId: 1})
		close(release)
		require.NoError(t, <-submitted)
		require.NoError(t, err)
		assert.DeepEqual(t, decode(aggregatorRoot), contribution.BlockRoot)
	})

	t.Run("slot without sync messages falls back to the head", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		handler := mock.NewMockHandler(ctrl)
		handler.EXPECT().Post(gomock.Any(), syncPoolPath, nil, gomock.Any(), nil).Return(nil).Times(1)
		expectHead(handler)
		expectContribution(handler, headRoot, 3)

		validatorClient := &beaconApiValidatorClient{handler: handler}
		require.NoError(t, validatorClient.submitSyncMessage(t.Context(), &ethpb.SyncCommitteeMessage{Slot: 2, ValidatorIndex: 7, BlockRoot: decode(aggregatorRoot)}))
		_, err := validatorClient.syncCommitteeContribution(t.Context(), &ethpb.SyncCommitteeContributionRequest{Slot: 3, PublicKey: pubkey, SubnetId: 1})
		require.NoError(t, err)
	})
}

// TestGetSyncCommitteeContribution_ChecksTheContributionHost runs one optimistic and one validated host.
// An optimistic host returns 503 for the contribution, so the contribution GET moves to the validated host.
func TestGetSyncCommitteeContribution_ChecksTheContributionHost(t *testing.T) {
	root, err := hexutil.Decode("0x719d4f66a5f25c35d93718821aacb342194391034b11cf0a5822cc249178a274")
	require.NoError(t, err)
	otherRoot, err := hexutil.Decode("0x5ecf3bff35e39d5f75476d42950d549f81fa93038c46b6652ae89ae1f7ad834f")
	require.NoError(t, err)
	pubkey := make([]byte, 48)
	newHost := func(t *testing.T, optimistic bool, release <-chan struct{}) *httptest.Server {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var body any
			switch r.URL.Path {
			case "/eth/v1/beacon/pool/sync_committees":
				return
			case "/eth/v1/node/syncing":
				body = structs.SyncStatusResponse{Data: &structs.SyncStatusResponseData{IsOptimistic: optimistic}}
			case "/eth/v1/beacon/states/head/validators":
				if !optimistic {
					<-release
				}
				body = structs.GetValidatorsResponse{ExecutionOptimistic: optimistic, Data: []*structs.ValidatorContainer{{Index: "7"}}}
			case "/eth/v1/validator/sync_committee_contribution":
				if optimistic {
					http.Error(w, "optimistic", http.StatusServiceUnavailable)
					return
				}
				body = structs.ProduceSyncCommitteeContributionResponse{Data: &structs.SyncCommitteeContribution{
					Slot:              "2",
					BeaconBlockRoot:   r.URL.Query().Get("beacon_block_root"),
					SubcommitteeIndex: "1",
					AggregationBits:   "0x01",
					Signature:         "0x1b66ac1fb663c9bc59509846d6ec05345bd908eda73e670af888da41af171505cc411d61252fb6cb3fa0017b679f8bb2305b26a285fa2737f175668d0dff91cc1b66ac1fb663c9bc59509846d6ec05345bd908eda73e670af888da41af171505",
				}}
			default:
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			assert.NoError(t, json.NewEncoder(w).Encode(body))
		}))
		t.Cleanup(server.Close)
		return server
	}
	contribute := func(t *testing.T, hosts string, messages ...*ethpb.SyncCommitteeMessage) {
		provider, err := rest.NewRestConnectionProvider(hosts)
		require.NoError(t, err)
		handler := provider.Handler()
		validatorClient := &beaconApiValidatorClient{handler: handler, stateValidatorsProvider: beaconApiStateValidatorsProvider{handler: handler}}
		for _, m := range messages {
			require.NoError(t, validatorClient.submitSyncMessage(t.Context(), m))
		}
		contribution, err := validatorClient.syncCommitteeContribution(t.Context(), &ethpb.SyncCommitteeContributionRequest{Slot: 2, PublicKey: pubkey, SubnetId: 1})
		require.NoError(t, err)
		assert.DeepEqual(t, root, contribution.BlockRoot)
	}

	t.Run("optimistic second host answers the index POST first", func(t *testing.T) {
		// release unblocks the validated host's index POST after the test, so the optimistic host always answers it first.
		release := make(chan struct{})
		validated, optimistic := newHost(t, false, release), newHost(t, true, release)
		t.Cleanup(func() { close(release) })
		contribute(t, validated.URL+","+optimistic.URL,
			&ethpb.SyncCommitteeMessage{Slot: 2, ValidatorIndex: 7, BlockRoot: root},
			&ethpb.SyncCommitteeMessage{Slot: 2, ValidatorIndex: 8, BlockRoot: otherRoot})
	})

	t.Run("optimistic first host does not stop the validated second host", func(t *testing.T) {
		optimistic, validated := newHost(t, true, nil), newHost(t, false, nil)
		contribute(t, optimistic.URL+","+validated.URL, &ethpb.SyncCommitteeMessage{Slot: 2, ValidatorIndex: 7, BlockRoot: root})
	})
}

func TestGetSyncSubCommitteeIndex(t *testing.T) {
	const (
		pubkeyStr          = "0x8000091c2ae64ee414a54c1cc1fc67dec663408bc636cb86756e0200e41a75c8f86603f104f02c856983d2783116be13"
		syncDutiesEndpoint = "/eth/v1/validator/duties/sync"
		validatorsEndpoint = "/eth/v1/beacon/states/head/validators"
		validatorIndex     = "55293"
		slot               = primitives.Slot(123)
	)
	periodEpochs := params.BeaconConfig().EpochsPerSyncCommitteePeriod
	lastSlotOfPeriod := primitives.Slot(periodEpochs)*params.BeaconConfig().SlotsPerEpoch - 1

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
		slot             primitives.Slot
		dutiesEpoch      primitives.Epoch
		duties           []*structs.SyncCommitteeDuty
		validatorsErr    error
		dutiesErr        error
		expectedErrorMsg string
	}{
		{
			name:        "success",
			slot:        slot,
			dutiesEpoch: slots.ToEpoch(slot),
			duties:      syncDuties,
		},
		{
			name:        "last slot of sync committee period uses next period",
			slot:        lastSlotOfPeriod,
			dutiesEpoch: periodEpochs,
			duties:      syncDuties,
		},
		{
			name:             "no sync duties",
			slot:             slot,
			dutiesEpoch:      slots.ToEpoch(slot),
			duties:           []*structs.SyncCommitteeDuty{},
			expectedErrorMsg: fmt.Sprintf("no sync committee duty for the given slot %d", slot),
		},
		{
			name:             "duties endpoint error",
			slot:             slot,
			dutiesEpoch:      slots.ToEpoch(slot),
			dutiesErr:        errors.New("bad request"),
			expectedErrorMsg: "bad request",
		},
		{
			name:             "validator index endpoint error",
			slot:             slot,
			dutiesEpoch:      slots.ToEpoch(slot),
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
			handler := mock.NewMockHandler(ctrl)
			handler.EXPECT().Post(
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

				handler.EXPECT().Get(
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

			handler.EXPECT().Post(
				gomock.Any(),
				fmt.Sprintf("%s/%d", syncDutiesEndpoint, test.dutiesEpoch),
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
				handler: handler,
				stateValidatorsProvider: beaconApiStateValidatorsProvider{
					handler: handler,
				},
				dutiesProvider: beaconApiDutiesProvider{
					handler: handler,
				},
			}
			actualResponse, err := validatorClient.syncSubcommitteeIndex(ctx, &ethpb.SyncSubcommitteeIndexRequest{
				PublicKey: pubkey,
				Slot:      test.slot,
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
