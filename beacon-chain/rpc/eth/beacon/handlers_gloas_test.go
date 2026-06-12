package beacon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/kzg"
	chainMock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	dbTest "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	executiontesting "github.com/OffchainLabs/prysm/v7/beacon-chain/execution/testing"
	mockp2p "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/lookup"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/testutil"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	consensusblocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	mock2 "github.com/OffchainLabs/prysm/v7/testing/mock"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

func envelopeCacheFor(signed *ethpb.SignedExecutionPayloadEnvelope) *cache.ExecutionPayloadEnvelopeCache {
	c := cache.NewExecutionPayloadEnvelopeCache()
	c.Set(&cache.ExecutionPayloadContents{Envelope: signed.Message})
	return c
}

type mockEnvelopeVerifier struct {
	errSlotAboveFinalized error
	errSlotMatchesBlock   error
	errBuilderValid       error
	errPayloadHash        error
	errExecutionRequests  error
	errSignature          error
}

var _ verification.ExecutionPayloadEnvelopeVerifier = &mockEnvelopeVerifier{}

func (*mockEnvelopeVerifier) VerifyBlockRootSeen(_ func([32]byte) bool) error  { return nil }
func (*mockEnvelopeVerifier) VerifyBlockRootValid(_ func([32]byte) bool) error { return nil }
func (m *mockEnvelopeVerifier) VerifySlotAboveFinalized(_ primitives.Epoch) error {
	return m.errSlotAboveFinalized
}
func (m *mockEnvelopeVerifier) VerifySlotMatchesBlock(_ primitives.Slot) error {
	return m.errSlotMatchesBlock
}
func (m *mockEnvelopeVerifier) VerifyBuilderValid(_ interfaces.ROExecutionPayloadBid) error {
	return m.errBuilderValid
}
func (m *mockEnvelopeVerifier) VerifyPayloadHash(_ interfaces.ROExecutionPayloadBid) error {
	return m.errPayloadHash
}
func (m *mockEnvelopeVerifier) VerifyExecutionRequestsRoot(_ interfaces.ROExecutionPayloadBid) error {
	return m.errExecutionRequests
}
func (m *mockEnvelopeVerifier) VerifySignature(_ context.Context, _ state.ReadOnlyBeaconState) error {
	return m.errSignature
}
func (*mockEnvelopeVerifier) SatisfyRequirement(_ verification.Requirement) {}

func (*mockEnvelopeVerifier) SatisfiedRequirements() error { return nil }

func gloasBlockWithBid(t *testing.T, slot primitives.Slot, bid *ethpb.SignedExecutionPayloadBid) interfaces.ReadOnlySignedBeaconBlock {
	t.Helper()
	sb := util.NewBeaconBlockGloas()
	sb.Block.Slot = slot
	sb.Block.Body.SignedExecutionPayloadBid = bid
	signed, err := consensusblocks.NewSignedBeaconBlock(sb)
	require.NoError(t, err)
	return signed
}

// wireEnvelopeGossipDeps fills nil validateEnvelopeGossip deps with an always-passing verifier.
func wireEnvelopeGossipDeps(t *testing.T, s *Server) {
	t.Helper()
	s.Blocker = &testutil.MockBlocker{BlockToReturn: gloasBlockWithBid(t, 100, util.GenerateTestSignedExecutionPayloadBid(100))}
	chain := &chainMock.ChainService{FinalizedCheckPoint: &ethpb.Checkpoint{}}
	if s.FinalizationFetcher == nil {
		s.FinalizationFetcher = chain
	}
	if s.HeadFetcher == nil {
		s.HeadFetcher = chain
	}
	s.PayloadEnvelopeVerifier = func(_ interfaces.ROSignedExecutionPayloadEnvelope, _ []verification.Requirement) verification.ExecutionPayloadEnvelopeVerifier {
		return &mockEnvelopeVerifier{}
	}
}

func blindedJSONBody(t *testing.T, signed *ethpb.SignedExecutionPayloadEnvelope) []byte {
	t.Helper()
	blinded, err := structs.SignedWireBlindedFromFull(signed)
	require.NoError(t, err)
	msg, err := structs.BlindedExecutionPayloadEnvelopeFromConsensus(blinded.Message)
	require.NoError(t, err)
	body, err := json.Marshal(&structs.SignedBlindedExecutionPayloadEnvelope{
		Message:   msg,
		Signature: hexutil.Encode(blinded.Signature),
	})
	require.NoError(t, err)
	return body
}

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

// Stateful: body is the spec-wire blinded envelope; BN reconstructs full from cache.
func TestPublishExecutionPayloadEnvelope_StatefulBlinded_OK(t *testing.T) {
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

	body := blindedJSONBody(t, signed)

	s := &Server{
		V1Alpha1ValidatorServer:       v1alpha1Server,
		ExecutionPayloadEnvelopeCache: envelopeCacheFor(signed),
	}
	wireEnvelopeGossipDeps(t, s)
	req := httptest.NewRequest(http.MethodPost, "/eth/v1/beacon/execution_payload_envelope", bytes.NewReader(body))
	req.Header.Set(api.VersionHeader, version.String(version.Gloas))
	req.Header.Set(api.ExecutionPayloadBlindedHeader, "true")
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}

	s.PublishExecutionPayloadEnvelope(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

// Missing Eth-Execution-Payload-Blinded header must be a 400.
func TestPublishExecutionPayloadEnvelope_MissingBlindedHeader(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/eth/v1/beacon/execution_payload_envelope", bytes.NewReader([]byte("{}")))
	req.Header.Set(api.VersionHeader, version.String(version.Gloas))
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}

	s.PublishExecutionPayloadEnvelope(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, true, bytes.Contains(w.Body.Bytes(), []byte(api.ExecutionPayloadBlindedHeader)))
}

func TestPublishExecutionPayloadEnvelope_InvalidBody(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/eth/v1/beacon/execution_payload_envelope", bytes.NewReader([]byte("not json")))
	req.Header.Set(api.VersionHeader, version.String(version.Gloas))
	req.Header.Set(api.ExecutionPayloadBlindedHeader, "false")
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
	wireEnvelopeGossipDeps(t, s)
	req := httptest.NewRequest(http.MethodPost, "/eth/v1/beacon/execution_payload_envelope", bytes.NewReader(body))
	req.Header.Set(api.VersionHeader, version.String(version.Gloas))
	req.Header.Set(api.ExecutionPayloadBlindedHeader, "false")
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}

	s.PublishExecutionPayloadEnvelope(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

// DB integration failed -> Aborted maps to 202.
func TestPublishExecutionPayloadEnvelope_ImportFailureReturns202(t *testing.T) {
	ctrl := gomock.NewController(t)
	signed := testSignedEnvelope()
	contents, err := structs.SignedExecutionPayloadEnvelopeContentsFromConsensus(signed, nil, nil)
	require.NoError(t, err)
	body, err := json.Marshal(contents)
	require.NoError(t, err)

	v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
	v1alpha1Server.EXPECT().PublishExecutionPayloadEnvelope(
		gomock.Any(), gomock.Any(),
	).Return(nil, status.Error(codes.Aborted, "import failed"))

	s := &Server{V1Alpha1ValidatorServer: v1alpha1Server}
	wireEnvelopeGossipDeps(t, s)
	req := httptest.NewRequest(http.MethodPost, "/eth/v1/beacon/execution_payload_envelope", bytes.NewReader(body))
	req.Header.Set(api.VersionHeader, version.String(version.Gloas))
	req.Header.Set(api.ExecutionPayloadBlindedHeader, "false")
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}

	s.PublishExecutionPayloadEnvelope(w, req)
	require.Equal(t, http.StatusAccepted, w.Code)
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
	wireEnvelopeGossipDeps(t, s)
	req := httptest.NewRequest(http.MethodPost, "/eth/v1/beacon/execution_payload_envelope", bytes.NewReader(body))
	req.Header.Set(api.VersionHeader, version.String(version.Gloas))
	req.Header.Set(api.ExecutionPayloadBlindedHeader, "false")
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
	wireEnvelopeGossipDeps(t, s)
	req := httptest.NewRequest(http.MethodPost, "/eth/v1/beacon/execution_payload_envelope", bytes.NewReader(body))
	req.Header.Set(api.VersionHeader, version.String(version.Gloas))
	req.Header.Set(api.ExecutionPayloadBlindedHeader, "false")
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
	body := blindedJSONBody(t, signed)

	s := &Server{
		V1Alpha1ValidatorServer:       v1alpha1Server,
		ExecutionPayloadEnvelopeCache: envelopeCacheFor(signed),
	}
	wireEnvelopeGossipDeps(t, s)
	req := httptest.NewRequest(http.MethodPost, "/eth/v1/beacon/execution_payload_envelope", bytes.NewReader(body))
	req.Header.Set(api.VersionHeader, version.String(version.Gloas))
	req.Header.Set(api.ExecutionPayloadBlindedHeader, "true")
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}

	s.PublishExecutionPayloadEnvelope(w, req)
	require.Equal(t, http.StatusInternalServerError, w.Code)
}

// SSZ stateful: send SignedWireBlindedExecutionPayloadEnvelope, header=true.
func TestPublishExecutionPayloadEnvelope_SSZ_StatefulBlinded(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	ctrl := gomock.NewController(t)
	signed := testSignedEnvelope()
	blinded, err := structs.SignedWireBlindedFromFull(signed)
	require.NoError(t, err)
	sszBody, err := blinded.MarshalSSZ()
	require.NoError(t, err)

	v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
	v1alpha1Server.EXPECT().PublishExecutionPayloadEnvelope(
		gomock.Any(), gomock.Any(),
	).Return(&emptypb.Empty{}, nil)

	s := &Server{
		V1Alpha1ValidatorServer:       v1alpha1Server,
		ExecutionPayloadEnvelopeCache: envelopeCacheFor(signed),
	}
	wireEnvelopeGossipDeps(t, s)
	req := httptest.NewRequest(http.MethodPost, "/eth/v1/beacon/execution_payload_envelope", bytes.NewReader(sszBody))
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set(api.VersionHeader, version.String(version.Gloas))
	req.Header.Set(api.ExecutionPayloadBlindedHeader, "true")
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}

	s.PublishExecutionPayloadEnvelope(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

// SSZ stateful with no cache entry must fail (cannot reconstruct full).
func TestPublishExecutionPayloadEnvelope_SSZ_StatefulBlinded_CacheMiss(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	signed := testSignedEnvelope()
	blinded, err := structs.SignedWireBlindedFromFull(signed)
	require.NoError(t, err)
	sszBody, err := blinded.MarshalSSZ()
	require.NoError(t, err)

	s := &Server{
		ExecutionPayloadEnvelopeCache: cache.NewExecutionPayloadEnvelopeCache(),
	}
	req := httptest.NewRequest(http.MethodPost, "/eth/v1/beacon/execution_payload_envelope", bytes.NewReader(sszBody))
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set(api.VersionHeader, version.String(version.Gloas))
	req.Header.Set(api.ExecutionPayloadBlindedHeader, "true")
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}

	s.PublishExecutionPayloadEnvelope(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, true, bytes.Contains(w.Body.Bytes(), []byte("no cached execution payload envelope")))
}

// A cached envelope whose HTR differs from the signed blinded envelope must be rejected,
// even when the beacon_block_root matches.
func TestPublishExecutionPayloadEnvelope_StatefulBlinded_HTRMismatch(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	signed := testSignedEnvelope()
	body := blindedJSONBody(t, signed)

	tampered := testSignedEnvelope()
	tampered.Message.BuilderIndex = signed.Message.BuilderIndex + 1

	s := &Server{
		ExecutionPayloadEnvelopeCache: envelopeCacheFor(tampered),
	}
	req := httptest.NewRequest(http.MethodPost, "/eth/v1/beacon/execution_payload_envelope", bytes.NewReader(body))
	req.Header.Set(api.VersionHeader, version.String(version.Gloas))
	req.Header.Set(api.ExecutionPayloadBlindedHeader, "true")
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}

	s.PublishExecutionPayloadEnvelope(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, true, bytes.Contains(w.Body.Bytes(), []byte("hash tree root does not match")))
}

func TestPublishExecutionPayloadEnvelope_SSZ_Contents(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	ctrl := gomock.NewController(t)
	signed := testSignedEnvelope()
	contents := &ethpb.SignedExecutionPayloadEnvelopeContents{
		SignedExecutionPayloadEnvelope: signed,
	}
	sszBody, err := contents.MarshalSSZ()
	require.NoError(t, err)

	v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
	v1alpha1Server.EXPECT().PublishExecutionPayloadEnvelope(
		gomock.Any(), gomock.Any(),
	).Return(&emptypb.Empty{}, nil)

	s := &Server{V1Alpha1ValidatorServer: v1alpha1Server}
	wireEnvelopeGossipDeps(t, s)
	req := httptest.NewRequest(http.MethodPost, "/eth/v1/beacon/execution_payload_envelope", bytes.NewReader(sszBody))
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set(api.VersionHeader, version.String(version.Gloas))
	req.Header.Set(api.ExecutionPayloadBlindedHeader, "false")
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}

	s.PublishExecutionPayloadEnvelope(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestPublishExecutionPayloadEnvelope_BroadcastValidation(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	signed := testSignedEnvelope()
	envRoot := bytesutil.ToBytes32(signed.Message.BeaconBlockRoot)
	envSlot := primitives.Slot(signed.Message.Payload.SlotNumber)
	body := blindedJSONBody(t, signed)

	// State that fails gloas.VerifyExecutionPayloadEnvelope (slot mismatch is
	// enough). Lets us exercise the consensus path and assert it actually runs.
	failingState, err := util.NewBeaconStateGloas()
	require.NoError(t, err)

	otherRoot := bytesutil.ToBytes32(bytesutil.PadTo([]byte("other-root"), 32))

	cases := []struct {
		name              string
		query             string
		headRoot          [32]byte
		headState         state.BeaconState
		headStateErr      error
		canonicalAtEnvSlt *[32]byte // nil → CanonicalNodeAtSlot returns a zero root
		expectPublish     bool
		expectedStatus    int
		expectedBody      string
	}{
		{name: "default (gossip)", query: "", expectPublish: true, expectedStatus: http.StatusOK},
		{name: "explicit gossip", query: "?broadcast_validation=gossip", expectPublish: true, expectedStatus: http.StatusOK},
		{
			name:           "consensus envRoot not head",
			query:          "?broadcast_validation=consensus",
			headRoot:       otherRoot,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "is not canonical head",
		},
		{
			name:           "consensus verification fails",
			query:          "?broadcast_validation=consensus",
			headRoot:       envRoot,
			headState:      failingState,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "consensus validation failed",
		},
		{
			name:              "consensus_and_equivocation equivocation detected",
			query:             "?broadcast_validation=consensus_and_equivocation",
			canonicalAtEnvSlt: &otherRoot,
			expectedStatus:    http.StatusBadRequest,
			expectedBody:      "block is equivocated",
		},
		{
			name:           "consensus_and_equivocation no equivocation runs consensus check",
			query:          "?broadcast_validation=consensus_and_equivocation",
			headRoot:       envRoot,
			headState:      failingState,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "consensus validation failed",
		},
		{
			name:           "consensus head state error is internal",
			query:          "?broadcast_validation=consensus",
			headRoot:       envRoot,
			headStateErr:   errors.New("state unavailable"),
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   "could not get head state",
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

			chainSvc := &chainMock.ChainService{
				Root:                tc.headRoot[:],
				State:               tc.headState,
				HeadStateErr:        tc.headStateErr,
				FinalizedCheckPoint: &ethpb.Checkpoint{},
			}
			if tc.canonicalAtEnvSlt != nil {
				chainSvc.MockCanonicalRoots = map[primitives.Slot][32]byte{envSlot: *tc.canonicalAtEnvSlt}
				// full=false mirrors the wall clock slot case; the root alone must trip the check.
				chainSvc.MockCanonicalFull = map[primitives.Slot]bool{envSlot: false}
			}
			s := &Server{
				V1Alpha1ValidatorServer:       v1alpha1Server,
				ForkchoiceFetcher:             chainSvc,
				HeadFetcher:                   chainSvc,
				FinalizationFetcher:           chainSvc,
				ExecutionPayloadEnvelopeCache: envelopeCacheFor(signed),
			}
			wireEnvelopeGossipDeps(t, s)
			req := httptest.NewRequest(http.MethodPost, "/eth/v1/beacon/execution_payload_envelope"+tc.query, bytes.NewReader(body))
			req.Header.Set(api.VersionHeader, version.String(version.Gloas))
			req.Header.Set(api.ExecutionPayloadBlindedHeader, "true")
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

// Each REJECT-class gossip condition must 400 and suppress the broadcast.
func TestPublishExecutionPayloadEnvelope_GossipValidation(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	signed := testSignedEnvelope()
	envSlot := primitives.Slot(signed.Message.Payload.SlotNumber)

	matchingBid := func() *ethpb.SignedExecutionPayloadBid {
		bid := util.GenerateTestSignedExecutionPayloadBid(envSlot)
		bid.Message.BuilderIndex = signed.Message.BuilderIndex
		bid.Message.BlockHash = signed.Message.Payload.BlockHash
		reqRoot, err := signed.Message.ExecutionRequests.HashTreeRoot()
		require.NoError(t, err)
		bid.Message.ExecutionRequestsRoot = reqRoot[:]
		return bid
	}

	headState, err := util.NewBeaconStateGloas()
	require.NoError(t, err)

	cases := []struct {
		name         string
		blocker      *testutil.MockBlocker
		finalized    *ethpb.Checkpoint
		expectedBody string
	}{
		{
			name:         "unknown block root",
			blocker:      &testutil.MockBlocker{ErrorToReturn: lookup.NewBlockNotFoundError("missing")},
			expectedBody: "envelope beacon block root is unknown",
		},
		{
			name:         "slot below finalized",
			blocker:      &testutil.MockBlocker{BlockToReturn: gloasBlockWithBid(t, envSlot, util.GenerateTestSignedExecutionPayloadBid(envSlot))},
			finalized:    &ethpb.Checkpoint{Epoch: 100},
			expectedBody: "before finalized",
		},
		{
			name:         "slot mismatch",
			blocker:      &testutil.MockBlocker{BlockToReturn: gloasBlockWithBid(t, envSlot.Add(1), util.GenerateTestSignedExecutionPayloadBid(envSlot))},
			expectedBody: "envelope slot does not match block slot",
		},
		{
			// GenerateTestSignedExecutionPayloadBid uses builder index 1; the envelope uses 42.
			name:         "builder mismatch",
			blocker:      &testutil.MockBlocker{BlockToReturn: gloasBlockWithBid(t, envSlot, util.GenerateTestSignedExecutionPayloadBid(envSlot))},
			expectedBody: "builder index does not match",
		},
		{
			name: "payload hash mismatch",
			blocker: &testutil.MockBlocker{BlockToReturn: gloasBlockWithBid(t, envSlot, func() *ethpb.SignedExecutionPayloadBid {
				bid := matchingBid()
				bid.Message.BlockHash = bytesutil.PadTo([]byte("other-hash"), 32)
				return bid
			}())},
			expectedBody: "block hash does not match",
		},
		{
			name: "execution requests root mismatch",
			blocker: &testutil.MockBlocker{BlockToReturn: gloasBlockWithBid(t, envSlot, func() *ethpb.SignedExecutionPayloadBid {
				bid := matchingBid()
				bid.Message.ExecutionRequestsRoot = make([]byte, 32)
				return bid
			}())},
			expectedBody: "execution requests root does not match",
		},
		{
			// Bid-consistent envelope with a garbage signature must fail the final check.
			name:         "invalid signature",
			blocker:      &testutil.MockBlocker{BlockToReturn: gloasBlockWithBid(t, envSlot, matchingBid())},
			expectedBody: "gossip validation failed",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			contents, err := structs.SignedExecutionPayloadEnvelopeContentsFromConsensus(signed, nil, nil)
			require.NoError(t, err)
			body, err := json.Marshal(contents)
			require.NoError(t, err)

			finalized := tc.finalized
			if finalized == nil {
				finalized = &ethpb.Checkpoint{}
			}
			chainSvc := &chainMock.ChainService{State: headState, FinalizedCheckPoint: finalized}
			s := &Server{
				Blocker:                 tc.blocker,
				HeadFetcher:             chainSvc,
				FinalizationFetcher:     chainSvc,
				PayloadEnvelopeVerifier: verification.NewEnvelopeVerifier,
			}
			req := httptest.NewRequest(http.MethodPost, "/eth/v1/beacon/execution_payload_envelope", bytes.NewReader(body))
			req.Header.Set(api.VersionHeader, version.String(version.Gloas))
			req.Header.Set(api.ExecutionPayloadBlindedHeader, "false")
			w := httptest.NewRecorder()
			w.Body = &bytes.Buffer{}

			s.PublishExecutionPayloadEnvelope(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
			assert.Equal(t, true, bytes.Contains(w.Body.Bytes(), []byte(tc.expectedBody)))
		})
	}
}
