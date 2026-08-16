package beacon

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	chainMock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	dbutil "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	p2pmock "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/core"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	mockSync "github.com/OffchainLabs/prysm/v7/beacon-chain/sync/initial-sync/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

func testJSONSignedBid() *structs.SignedExecutionPayloadBid {
	hex32 := "0x" + strings.Repeat("00", 32)
	hex20 := "0x" + strings.Repeat("00", 20)
	hex96 := "0x" + strings.Repeat("00", 96)
	return &structs.SignedExecutionPayloadBid{
		Message: &structs.ExecutionPayloadBid{
			ParentBlockHash:       hex32,
			ParentBlockRoot:       hex32,
			BlockHash:             hex32,
			PrevRandao:            hex32,
			FeeRecipient:          hex20,
			GasLimit:              "30000000",
			BuilderIndex:          "1",
			Slot:                  "1",
			Value:                 "0",
			ExecutionPayment:      "0",
			BlobKzgCommitments:    []string{},
			ExecutionRequestsRoot: hex32,
		},
		Signature: hex96,
	}
}

func TestPublishSignedExecutionPayloadBid_NoVersionHeader(t *testing.T) {
	s := &Server{
		SyncChecker:           &mockSync.Sync{IsSyncing: false},
		HeadFetcher:           &chainMock.ChainService{},
		TimeFetcher:           &chainMock.ChainService{},
		OptimisticModeFetcher: &chainMock.ChainService{},
	}
	req := httptest.NewRequest(http.MethodPost, "/eth/v2/beacon/execution_payload/bid", nil)
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}

	s.PublishSignedExecutionPayloadBid(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, true, bytes.Contains(w.Body.Bytes(), []byte("header is required")))
}

func TestPublishSignedExecutionPayloadBid_EmptyBody(t *testing.T) {
	s := &Server{
		SyncChecker:           &mockSync.Sync{IsSyncing: false},
		HeadFetcher:           &chainMock.ChainService{},
		TimeFetcher:           &chainMock.ChainService{},
		OptimisticModeFetcher: &chainMock.ChainService{},
	}
	req := httptest.NewRequest(http.MethodPost, "/eth/v2/beacon/execution_payload/bid", nil)
	req.Header.Set(api.VersionHeader, "gloas")
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}

	s.PublishSignedExecutionPayloadBid(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, true, bytes.Contains(w.Body.Bytes(), []byte("No data submitted")))
}

func TestPublishSignedExecutionPayloadBid_Syncing(t *testing.T) {
	s := &Server{
		SyncChecker:           &mockSync.Sync{IsSyncing: true},
		HeadFetcher:           &chainMock.ChainService{},
		TimeFetcher:           &chainMock.ChainService{},
		OptimisticModeFetcher: &chainMock.ChainService{},
	}
	req := httptest.NewRequest(http.MethodPost, "/eth/v2/beacon/execution_payload/bid", nil)
	req.Header.Set(api.VersionHeader, "gloas")
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}

	s.PublishSignedExecutionPayloadBid(w, req)
	require.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestPublishSignedExecutionPayloadBid_JSON(t *testing.T) {
	s := &Server{
		SyncChecker:           &mockSync.Sync{},
		HeadFetcher:           &chainMock.ChainService{},
		TimeFetcher:           &chainMock.ChainService{},
		OptimisticModeFetcher: &chainMock.ChainService{},
		CoreService:           executionPayloadBidCoreService(t),
	}

	bid := testJSONSignedBid()
	body, err := json.Marshal(bid)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/eth/v2/beacon/execution_payload/bid", bytes.NewReader(body))
	req.Header.Set(api.VersionHeader, "gloas")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}

	s.PublishSignedExecutionPayloadBid(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestPublishSignedExecutionPayloadBid_MalformedJSON(t *testing.T) {
	s := &Server{
		SyncChecker:           &mockSync.Sync{IsSyncing: false},
		HeadFetcher:           &chainMock.ChainService{},
		TimeFetcher:           &chainMock.ChainService{},
		OptimisticModeFetcher: &chainMock.ChainService{},
	}
	req := httptest.NewRequest(http.MethodPost, "/eth/v2/beacon/execution_payload/bid", bytes.NewReader([]byte("{bad json")))
	req.Header.Set(api.VersionHeader, "gloas")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}

	s.PublishSignedExecutionPayloadBid(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, true, bytes.Contains(w.Body.Bytes(), []byte("Could not decode request body")))
}

func TestPublishSignedExecutionPayloadBid_InvalidSSZ(t *testing.T) {
	s := &Server{
		SyncChecker:           &mockSync.Sync{IsSyncing: false},
		HeadFetcher:           &chainMock.ChainService{},
		TimeFetcher:           &chainMock.ChainService{},
		OptimisticModeFetcher: &chainMock.ChainService{},
	}
	req := httptest.NewRequest(http.MethodPost, "/eth/v2/beacon/execution_payload/bid", bytes.NewReader([]byte{0x01, 0x02}))
	req.Header.Set(api.VersionHeader, "gloas")
	req.Header.Set("Content-Type", "application/octet-stream")
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}

	s.PublishSignedExecutionPayloadBid(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, true, bytes.Contains(w.Body.Bytes(), []byte("Could not unmarshal SSZ")))
}

func TestPublishSignedExecutionPayloadBid_SSZ(t *testing.T) {
	s := &Server{
		SyncChecker:           &mockSync.Sync{},
		HeadFetcher:           &chainMock.ChainService{},
		TimeFetcher:           &chainMock.ChainService{},
		OptimisticModeFetcher: &chainMock.ChainService{},
		CoreService:           executionPayloadBidCoreService(t),
	}

	bid := &ethpb.SignedExecutionPayloadBid{
		Message: &ethpb.ExecutionPayloadBid{
			ParentBlockHash:       make([]byte, 32),
			ParentBlockRoot:       make([]byte, 32),
			BlockHash:             make([]byte, 32),
			PrevRandao:            make([]byte, 32),
			FeeRecipient:          make([]byte, 20),
			GasLimit:              30000000,
			BuilderIndex:          1,
			Slot:                  1,
			Value:                 0,
			ExecutionPayment:      0,
			ExecutionRequestsRoot: make([]byte, 32),
		},
		Signature: make([]byte, 96),
	}
	sszBytes, err := bid.MarshalSSZ()
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/eth/v2/beacon/execution_payload/bid", bytes.NewReader(sszBytes))
	req.Header.Set(api.VersionHeader, "gloas")
	req.Header.Set("Content-Type", "application/octet-stream")
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}

	s.PublishSignedExecutionPayloadBid(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestPublishSignedExecutionPayloadBid_ValidationFailure(t *testing.T) {
	coreService := executionPayloadBidCoreService(t)
	coreService.ProposerPreferencesCache = cache.NewProposerPreferencesCache()
	s := &Server{
		SyncChecker:           &mockSync.Sync{},
		HeadFetcher:           &chainMock.ChainService{},
		TimeFetcher:           &chainMock.ChainService{},
		OptimisticModeFetcher: &chainMock.ChainService{},
		CoreService:           coreService,
	}

	body, err := json.Marshal(testJSONSignedBid())
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/eth/v2/beacon/execution_payload/bid", bytes.NewReader(body))
	req.Header.Set(api.VersionHeader, "gloas")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}

	s.PublishSignedExecutionPayloadBid(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, true, bytes.Contains(w.Body.Bytes(), []byte("no proposer preferences")))
}

func TestPublishSignedExecutionPayloadBid_InternalError(t *testing.T) {
	coreService := executionPayloadBidCoreService(t)
	coreService.P2P = &failingBidBroadcaster{MockBroadcaster: &p2pmock.MockBroadcaster{}}
	s := &Server{
		SyncChecker:           &mockSync.Sync{},
		HeadFetcher:           &chainMock.ChainService{},
		TimeFetcher:           &chainMock.ChainService{},
		OptimisticModeFetcher: &chainMock.ChainService{},
		CoreService:           coreService,
	}

	body, err := json.Marshal(testJSONSignedBid())
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/eth/v2/beacon/execution_payload/bid", bytes.NewReader(body))
	req.Header.Set(api.VersionHeader, "gloas")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}

	s.PublishSignedExecutionPayloadBid(w, req)
	require.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, true, bytes.Contains(w.Body.Bytes(), []byte("could not broadcast")))
}

func executionPayloadBidCoreService(t *testing.T) *core.Service {
	t.Helper()
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)
	ctx := t.Context()
	st, _ := util.DeterministicGenesisStateGloas(t, 64)
	parentRoot := [32]byte{}
	require.NoError(t, transition.UpdateNextSlotCache(ctx, parentRoot[:], st))
	database := dbutil.SetupDB(t)
	genesisRoot := [32]byte{'g'}
	require.NoError(t, database.SaveGenesisBlockRoot(ctx, genesisRoot))
	preferencesCache := cache.NewProposerPreferencesCache()
	preferencesCache.Add(cache.ProposerPreference{DependentRoot: genesisRoot}, 1)
	return &core.Service{
		SyncChecker:              &mockSync.Sync{},
		P2P:                      &p2pmock.MockBroadcaster{},
		BeaconDB:                 database,
		ProposerPreferencesCache: preferencesCache,
		HighestBidCache:          cache.NewHighestExecutionPayloadBidCache(),
		OperationNotifier:        &chainMock.MockOperationNotifier{},
		ForkchoiceFetcher: &chainMock.ChainService{
			ForkchoiceGasLimits: map[[32]byte]uint64{parentRoot: 30_000_000},
		},
		NewExecutionPayloadBidVerifier: func(interfaces.ROSignedExecutionPayloadBid, []verification.Requirement) verification.ExecutionPayloadBidVerifier {
			return &acceptingBidVerifier{}
		},
	}
}

type acceptingBidVerifier struct{}

func (*acceptingBidVerifier) VerifyCurrentOrNextSlot() error             { return nil }
func (*acceptingBidVerifier) VerifyBidSlotMatches(primitives.Slot) error { return nil }
func (*acceptingBidVerifier) VerifyBuilderActive(state.ReadOnlyBeaconState) error {
	return nil
}
func (*acceptingBidVerifier) VerifyBuilderVersion(state.ReadOnlyBeaconState) error {
	return nil
}
func (*acceptingBidVerifier) VerifyExecutionPaymentZero() error      { return nil }
func (*acceptingBidVerifier) VerifyFeeRecipientMatches([]byte) error { return nil }
func (*acceptingBidVerifier) VerifyBlobKzgCommitmentsLimit() error   { return nil }
func (*acceptingBidVerifier) VerifyPrevRandao(state.ReadOnlyBeaconState) error {
	return nil
}
func (*acceptingBidVerifier) VerifyParentBlockRootSeen(func([32]byte) bool) error { return nil }
func (*acceptingBidVerifier) VerifyBidCompatibleWithHead(func(interfaces.ROExecutionPayloadBid) bool) error {
	return nil
}
func (*acceptingBidVerifier) VerifyBidSlotHigherThanParent(primitives.Slot) error { return nil }
func (*acceptingBidVerifier) VerifyParentBlockHash(func([32]byte, [32]byte) bool) error {
	return nil
}
func (*acceptingBidVerifier) VerifyGasLimitTargetCompatible(uint64, uint64) error { return nil }
func (*acceptingBidVerifier) VerifyBuilderCanCoverBid(state.ReadOnlyBeaconState) error {
	return nil
}
func (*acceptingBidVerifier) VerifySignature(state.ReadOnlyBeaconState) error { return nil }
func (*acceptingBidVerifier) SatisfyRequirement(verification.Requirement)     {}

type failingBidBroadcaster struct {
	*p2pmock.MockBroadcaster
}

func (*failingBidBroadcaster) Broadcast(context.Context, proto.Message) error {
	return errors.New("broadcast failed")
}
