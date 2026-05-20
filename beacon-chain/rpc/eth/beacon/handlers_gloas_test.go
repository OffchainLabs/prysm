package beacon

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/kzg"
	chainMock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	dbTest "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	executiontesting "github.com/OffchainLabs/prysm/v7/beacon-chain/execution/testing"
	mockp2p "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/lookup"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/testutil"
	mockstategen "github.com/OffchainLabs/prysm/v7/beacon-chain/state/stategen/mock"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	mock2 "github.com/OffchainLabs/prysm/v7/testing/mock"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestGetExecutionPayloadEnvelope_AcceptsSlotID(t *testing.T) {
	ctx := t.Context()
	beaconDB := dbTest.SetupDB(t)

	root := bytesutil.ToBytes32(bytesutil.PadTo([]byte("beacon-root"), 32))
	blockHash := bytesutil.ToBytes32(bytesutil.PadTo([]byte("block-hash"), 32))

	env := &ethpb.SignedExecutionPayloadEnvelope{
		Message: &ethpb.ExecutionPayloadEnvelope{
			Payload: &enginev1.ExecutionPayloadGloas{
				ParentHash:    bytesutil.PadTo([]byte("parent"), 32),
				FeeRecipient:  bytesutil.PadTo([]byte("fee"), 20),
				StateRoot:     bytesutil.PadTo([]byte("state"), 32),
				ReceiptsRoot:  bytesutil.PadTo([]byte("receipts"), 32),
				LogsBloom:     make([]byte, 256),
				PrevRandao:    bytesutil.PadTo([]byte("randao"), 32),
				BaseFeePerGas: bytesutil.PadTo([]byte{1}, 32),
				BlockHash:     blockHash[:],
				Transactions:  [][]byte{},
				Withdrawals:   []*enginev1.Withdrawal{},
				SlotNumber:    primitives.Slot(177),
			},
			ExecutionRequests:     &enginev1.ExecutionRequests{},
			BuilderIndex:          primitives.BuilderIndex(42),
			BeaconBlockRoot:       root[:],
			ParentBeaconBlockRoot: bytesutil.PadTo([]byte("parent-beacon-root"), 32),
		},
		Signature: bytesutil.PadTo([]byte("sig"), 96),
	}
	require.NoError(t, beaconDB.SaveExecutionPayloadEnvelope(ctx, env))

	reconstructor := &executiontesting.EngineClient{
		ExecutionPayloadByBlockHash: map[[32]byte]*enginev1.ExecutionPayload{
			blockHash: &enginev1.ExecutionPayload{
				ParentHash:    bytesutil.PadTo([]byte("parent"), 32),
				FeeRecipient:  bytesutil.PadTo([]byte("fee"), 20),
				StateRoot:     bytesutil.PadTo([]byte("state"), 32),
				ReceiptsRoot:  bytesutil.PadTo([]byte("receipts"), 32),
				LogsBloom:     make([]byte, 256),
				PrevRandao:    bytesutil.PadTo([]byte("randao"), 32),
				BaseFeePerGas: bytesutil.PadTo([]byte{1}, 32),
				BlockHash:     blockHash[:],
				Transactions:  [][]byte{},
			},
		},
	}

	chain := &chainMock.ChainService{
		FinalizedRoots:  map[[32]byte]bool{},
		OptimisticRoots: map[[32]byte]bool{},
	}
	s := &Server{
		BeaconDB:               beaconDB,
		Blocker:                &testutil.MockBlocker{RootToReturn: root},
		ExecutionReconstructor: reconstructor,
		OptimisticModeFetcher:  chain,
		FinalizationFetcher:    chain,
	}

	req := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v1/beacon/execution_payload_envelope/{block_id}", nil)
	req.SetPathValue("block_id", "177")
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}

	s.GetExecutionPayloadEnvelope(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, version.String(version.Gloas), w.Header().Get("Eth-Consensus-Version"))
}

func TestGetExecutionPayloadEnvelope_BlockNotFound(t *testing.T) {
	s := &Server{
		Blocker: &testutil.MockBlocker{
			ErrorToReturn: lookup.NewBlockNotFoundError("missing block"),
		},
	}

	req := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v1/beacon/execution_payload_envelope/{block_id}", nil)
	req.SetPathValue("block_id", "not-a-root")
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}

	s.GetExecutionPayloadEnvelope(w, req)
	require.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, true, bytes.Contains(w.Body.Bytes(), []byte("Block not found")))
}

func testSignedEnvelope() *ethpb.SignedExecutionPayloadEnvelope {
	return &ethpb.SignedExecutionPayloadEnvelope{
		Message: &ethpb.ExecutionPayloadEnvelope{
			Payload: &enginev1.ExecutionPayloadGloas{
				ParentHash:    bytesutil.PadTo([]byte("parent"), 32),
				FeeRecipient:  bytesutil.PadTo([]byte("fee"), 20),
				StateRoot:     bytesutil.PadTo([]byte("state"), 32),
				ReceiptsRoot:  bytesutil.PadTo([]byte("receipts"), 32),
				LogsBloom:     make([]byte, 256),
				PrevRandao:    bytesutil.PadTo([]byte("randao"), 32),
				BaseFeePerGas: bytesutil.PadTo([]byte{1}, 32),
				BlockHash:     bytesutil.PadTo([]byte("blockhash"), 32),
				Transactions:  [][]byte{},
				Withdrawals:   []*enginev1.Withdrawal{},
				SlotNumber:    primitives.Slot(100),
			},
			ExecutionRequests:     &enginev1.ExecutionRequests{},
			BuilderIndex:          primitives.BuilderIndex(42),
			BeaconBlockRoot:       bytesutil.PadTo([]byte("beacon-root"), 32),
			ParentBeaconBlockRoot: bytesutil.PadTo([]byte("parent-beacon-root"), 32),
		},
		Signature: bytesutil.PadTo([]byte("sig"), 96),
	}
}

func TestPublishExecutionPayloadEnvelope_OK(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	ctrl := gomock.NewController(t)
	signed := testSignedEnvelope()

	v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
	v1alpha1Server.EXPECT().PublishExecutionPayloadEnvelope(
		gomock.Any(), gomock.Any(),
	).Return(&emptypb.Empty{}, nil)

	jsonEnvelope, err := structs.SignedExecutionPayloadEnvelopeFromConsensus(signed)
	require.NoError(t, err)
	body, err := json.Marshal(jsonEnvelope)
	require.NoError(t, err)

	s := &Server{V1Alpha1ValidatorServer: v1alpha1Server}
	req := httptest.NewRequest(http.MethodPost, "/eth/v1/beacon/execution_payload_envelope", bytes.NewReader(body))
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}

	s.PublishExecutionPayloadEnvelope(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestPublishExecutionPayloadEnvelope_InvalidBody(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/eth/v1/beacon/execution_payload_envelope", bytes.NewReader([]byte("not json")))
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}

	s.PublishExecutionPayloadEnvelope(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPublishExecutionPayloadEnvelope_StatelessContents_NoBlobs(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	ctrl := gomock.NewController(t)
	signed := testSignedEnvelope()
	contents, err := structs.SignedExecutionPayloadEnvelopeContentsFromConsensus(signed, nil, nil)
	require.NoError(t, err)
	body, err := json.Marshal(contents)
	require.NoError(t, err)

	v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
	v1alpha1Server.EXPECT().PublishExecutionPayloadEnvelope(
		gomock.Any(), gomock.Any(),
	).Return(&emptypb.Empty{}, nil)

	// With no blobs in the request, the sidecar broadcast/receive branch is
	// skipped, so the handler does not need a Broadcaster or DataColumnReceiver.
	s := &Server{V1Alpha1ValidatorServer: v1alpha1Server}
	req := httptest.NewRequest(http.MethodPost, "/eth/v1/beacon/execution_payload_envelope", bytes.NewReader(body))
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}

	s.PublishExecutionPayloadEnvelope(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

// statelessContentsBody builds a SignedExecutionPayloadEnvelopeContents JSON
// body with real blobs+proofs, returning the body bytes and the signed
// envelope used to construct it. blobMutator runs against the flat proofs
// after they're built so callers can inject corruption.
func statelessContentsBody(t *testing.T, blobCount int, mutateProofs func([][]byte)) ([]byte, *ethpb.SignedExecutionPayloadEnvelope) {
	t.Helper()
	require.NoError(t, kzg.Start())

	rawBlobs := make([]kzg.Blob, blobCount)
	for i := range rawBlobs {
		rawBlobs[i] = kzg.Blob{uint8(i + 1)}
	}
	_, proofsPerBlob := util.GenerateCellsAndProofs(t, rawBlobs)

	flatBlobs := make([][]byte, blobCount)
	for i, b := range rawBlobs {
		flatBlobs[i] = b[:]
	}
	flatProofs := make([][]byte, 0, blobCount*fieldparams.NumberOfColumns)
	for _, proofs := range proofsPerBlob {
		for _, p := range proofs {
			flatProofs = append(flatProofs, p[:])
		}
	}
	if mutateProofs != nil {
		mutateProofs(flatProofs)
	}

	signed := testSignedEnvelope()
	contents, err := structs.SignedExecutionPayloadEnvelopeContentsFromConsensus(signed, flatProofs, flatBlobs)
	require.NoError(t, err)
	body, err := json.Marshal(contents)
	require.NoError(t, err)
	return body, signed
}

func TestPublishExecutionPayloadEnvelope_StatelessContents_WithBlobs(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	body, _ := statelessContentsBody(t, 2, nil)

	ctrl := gomock.NewController(t)
	v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
	v1alpha1Server.EXPECT().PublishExecutionPayloadEnvelope(
		gomock.Any(), gomock.Any(),
	).Return(&emptypb.Empty{}, nil)

	s := &Server{
		V1Alpha1ValidatorServer: v1alpha1Server,
		Broadcaster:             &mockp2p.MockBroadcaster{},
		DataColumnReceiver:      &chainMock.ChainService{},
	}
	req := httptest.NewRequest(http.MethodPost, "/eth/v1/beacon/execution_payload_envelope", bytes.NewReader(body))
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}

	s.PublishExecutionPayloadEnvelope(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestPublishExecutionPayloadEnvelope_StatelessContents_RejectsBadProofs(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	body, _ := statelessContentsBody(t, 2, func(flatProofs [][]byte) {
		// Corrupt the first proof — verification must reject.
		flatProofs[0] = bytes.Repeat([]byte{0xff}, 48)
	})

	s := &Server{
		Broadcaster:        &mockp2p.MockBroadcaster{},
		DataColumnReceiver: &chainMock.ChainService{},
	}
	req := httptest.NewRequest(http.MethodPost, "/eth/v1/beacon/execution_payload_envelope", bytes.NewReader(body))
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}

	s.PublishExecutionPayloadEnvelope(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, true, bytes.Contains(w.Body.Bytes(), []byte("kzg verification failed")))
}

func TestPublishExecutionPayloadEnvelope_ServerError(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	ctrl := gomock.NewController(t)

	v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
	v1alpha1Server.EXPECT().PublishExecutionPayloadEnvelope(
		gomock.Any(), gomock.Any(),
	).Return(nil, status.Error(codes.Internal, "broadcast failed"))

	signed := testSignedEnvelope()
	jsonEnvelope, err := structs.SignedExecutionPayloadEnvelopeFromConsensus(signed)
	require.NoError(t, err)
	body, err := json.Marshal(jsonEnvelope)
	require.NoError(t, err)

	s := &Server{V1Alpha1ValidatorServer: v1alpha1Server}
	req := httptest.NewRequest(http.MethodPost, "/eth/v1/beacon/execution_payload_envelope", bytes.NewReader(body))
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}

	s.PublishExecutionPayloadEnvelope(w, req)
	require.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestPublishExecutionPayloadEnvelope_BroadcastValidation(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	signed := testSignedEnvelope()
	envRoot := bytesutil.ToBytes32(signed.Message.BeaconBlockRoot)
	envSlot := primitives.Slot(signed.Message.Payload.SlotNumber)
	jsonEnvelope, err := structs.SignedExecutionPayloadEnvelopeFromConsensus(signed)
	require.NoError(t, err)
	body, err := json.Marshal(jsonEnvelope)
	require.NoError(t, err)

	// State that fails gloas.VerifyExecutionPayloadEnvelope (slot mismatch is
	// enough). Lets us exercise the consensus path and assert it actually runs.
	failingState, err := util.NewBeaconStateGloas()
	require.NoError(t, err)

	cases := []struct {
		name           string
		query          string
		highestRoot    [32]byte
		highestSlot    primitives.Slot
		addState       bool
		expectPublish  bool
		expectedStatus int
		expectedBody   string
	}{
		{name: "default (gossip)", query: "", expectPublish: true, expectedStatus: http.StatusOK},
		{name: "explicit gossip", query: "?broadcast_validation=gossip", expectPublish: true, expectedStatus: http.StatusOK},
		{
			name:           "consensus state missing",
			query:          "?broadcast_validation=consensus",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "could not get state for envelope beacon block root",
		},
		{
			name:           "consensus verification fails",
			query:          "?broadcast_validation=consensus",
			addState:       true,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "consensus validation failed",
		},
		{
			name:           "consensus_and_equivocation equivocation detected",
			query:          "?broadcast_validation=consensus_and_equivocation",
			highestRoot:    bytesutil.ToBytes32(bytesutil.PadTo([]byte("other-root"), 32)),
			highestSlot:    envSlot,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "block is equivocated",
		},
		{
			name:           "consensus_and_equivocation no equivocation runs consensus check",
			query:          "?broadcast_validation=consensus_and_equivocation",
			highestRoot:    envRoot,
			highestSlot:    envSlot,
			addState:       true,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "consensus validation failed",
		},
		{
			name:           "invalid value",
			query:          "?broadcast_validation=bogus",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "invalid broadcast_validation value",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
			if tc.expectPublish {
				v1alpha1Server.EXPECT().PublishExecutionPayloadEnvelope(
					gomock.Any(), gomock.Any(),
				).Return(&emptypb.Empty{}, nil)
			}

			stateGen := mockstategen.NewService()
			if tc.addState {
				stateGen.AddStateForRoot(failingState, envRoot)
			}
			s := &Server{
				V1Alpha1ValidatorServer: v1alpha1Server,
				ForkchoiceFetcher: &chainMock.ChainService{
					BlockSlot: tc.highestSlot,
					Root:      tc.highestRoot[:],
				},
				StateGenService: stateGen,
			}
			req := httptest.NewRequest(http.MethodPost, "/eth/v1/beacon/execution_payload_envelope"+tc.query, bytes.NewReader(body))
			w := httptest.NewRecorder()
			w.Body = &bytes.Buffer{}

			s.PublishExecutionPayloadEnvelope(w, req)
			require.Equal(t, tc.expectedStatus, w.Code)
			if tc.expectedBody != "" {
				assert.Equal(t, true, bytes.Contains(w.Body.Bytes(), []byte(tc.expectedBody)))
			}
		})
	}
}
