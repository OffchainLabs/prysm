package beacon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"net/http/httptest"
	"testing"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	chainMock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	p2pMock "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	mockSync "github.com/OffchainLabs/prysm/v7/beacon-chain/sync/initial-sync/testing"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"google.golang.org/protobuf/proto"
)

func testJSONSignedBid() *structs.SignedExecutionPayloadBid {
	hex32 := "0x" + strings.Repeat("00", 32)
	hex20 := "0x" + strings.Repeat("00", 20)
	hex96 := "0x" + strings.Repeat("00", 96)
	return &structs.SignedExecutionPayloadBid{
		Message: &structs.ExecutionPayloadBid{
			ParentBlockHash:    hex32,
			ParentBlockRoot:    hex32,
			BlockHash:          hex32,
			PrevRandao:         hex32,
			FeeRecipient:       hex20,
			GasLimit:           "30000000",
			BuilderIndex:       "1",
			Slot:               "100",
			Value:              "0",
			ExecutionPayment:   "0",
			BlobKzgCommitments: []string{},
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
	broadcaster := &p2pMock.MockBroadcaster{}
	s := &Server{
		SyncChecker:           &mockSync.Sync{IsSyncing: false},
		HeadFetcher:           &chainMock.ChainService{},
		TimeFetcher:           &chainMock.ChainService{},
		OptimisticModeFetcher: &chainMock.ChainService{},
		Broadcaster:           broadcaster,
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
	require.Equal(t, 1, len(broadcaster.BroadcastMessages))
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
	broadcaster := &p2pMock.MockBroadcaster{}
	s := &Server{
		SyncChecker:           &mockSync.Sync{IsSyncing: false},
		HeadFetcher:           &chainMock.ChainService{},
		TimeFetcher:           &chainMock.ChainService{},
		OptimisticModeFetcher: &chainMock.ChainService{},
		Broadcaster:           broadcaster,
	}

	bid := &ethpb.SignedExecutionPayloadBid{
		Message: &ethpb.ExecutionPayloadBid{
			ParentBlockHash:  make([]byte, 32),
			ParentBlockRoot:  make([]byte, 32),
			BlockHash:        make([]byte, 32),
			PrevRandao:       make([]byte, 32),
			FeeRecipient:     make([]byte, 20),
			GasLimit:         30000000,
			BuilderIndex:     1,
			Slot:             100,
			Value:            0,
			ExecutionPayment: 0,
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
	require.Equal(t, 1, len(broadcaster.BroadcastMessages))
}

// errorBroadcaster is a test broadcaster that always returns an error.
type errorBroadcaster struct{ p2pMock.MockBroadcaster }

func (e *errorBroadcaster) Broadcast(_ context.Context, _ proto.Message) error {
	return fmt.Errorf("broadcast failed")
}

func TestPublishSignedExecutionPayloadBid_BroadcastError(t *testing.T) {
	s := &Server{
		SyncChecker:           &mockSync.Sync{IsSyncing: false},
		HeadFetcher:           &chainMock.ChainService{},
		TimeFetcher:           &chainMock.ChainService{},
		OptimisticModeFetcher: &chainMock.ChainService{},
		Broadcaster:           &errorBroadcaster{},
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
	require.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, true, bytes.Contains(w.Body.Bytes(), []byte("Could not broadcast")))
}
