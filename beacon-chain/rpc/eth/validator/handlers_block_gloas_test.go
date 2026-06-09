package validator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/kzg"
	blockchainTesting "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/peerdas"
	rewardtesting "github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/eth/rewards/testing"
	mockSync "github.com/OffchainLabs/prysm/v7/beacon-chain/sync/initial-sync/testing"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	mock2 "github.com/OffchainLabs/prysm/v7/testing/mock"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"go.uber.org/mock/gomock"
)

var (
	testRandao   = "0x1b66ac1fb663c9bc59509846d6ec05345bd908eda73e670af888da41af171505cc411d61252fb6cb3fa0017b679f8bb2305b26a285fa2737f175668d0dff91cc1b66ac1fb663c9bc59509846d6ec05345bd908eda73e670af888da41af171505"
	testGraffiti = "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2"
)

func testEnvelope() *eth.ExecutionPayloadEnvelope {
	return &eth.ExecutionPayloadEnvelope{
		Payload: &enginev1.ExecutionPayloadGloas{
			ParentHash:    make([]byte, 32),
			FeeRecipient:  make([]byte, 20),
			StateRoot:     make([]byte, 32),
			ReceiptsRoot:  make([]byte, 32),
			LogsBloom:     make([]byte, 256),
			PrevRandao:    make([]byte, 32),
			BaseFeePerGas: make([]byte, 32),
			BlockHash:     make([]byte, 32),
			SlotNumber:    1,
		},
		BuilderIndex:          0,
		BeaconBlockRoot:       make([]byte, 32),
		ParentBeaconBlockRoot: make([]byte, 32),
	}
}

func gloasGenericBlock() *eth.GenericBeaconBlock {
	return &eth.GenericBeaconBlock{
		Block: &eth.GenericBeaconBlock_Gloas{
			Gloas: util.NewBeaconBlockGloas().Block,
		},
	}
}

func TestProduceBlockV4_IncludePayloadTrue(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	ctrl := gomock.NewController(t)
	v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
	v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(gloasGenericBlock(), nil)
	v1alpha1Server.EXPECT().GetExecutionPayloadEnvelope(gomock.Any(), gomock.Any()).Return(
		&eth.ExecutionPayloadEnvelopeResponse{Envelope: testEnvelope()}, nil,
	)

	server := &Server{
		V1Alpha1Server:        v1alpha1Server,
		SyncChecker:           &mockSync.Sync{IsSyncing: false},
		OptimisticModeFetcher: &blockchainTesting.ChainService{},
		BlockRewardFetcher:    &rewardtesting.MockBlockRewardFetcher{Rewards: &structs.BlockRewards{Total: "10"}},
	}
	request := httptest.NewRequest(http.MethodGet, fmt.Sprintf("http://foo.example/eth/v4/validator/blocks/1?randao_reveal=%s&graffiti=%s", testRandao, testGraffiti), nil)
	request.SetPathValue("slot", "1")
	writer := httptest.NewRecorder()
	writer.Body = &bytes.Buffer{}
	server.ProduceBlockV4(writer, request)
	assert.Equal(t, http.StatusOK, writer.Code)

	var resp structs.ProduceBlockV4Response
	require.NoError(t, json.Unmarshal(writer.Body.Bytes(), &resp))
	assert.Equal(t, "gloas", resp.Version)
	assert.Equal(t, true, resp.ExecutionPayloadIncluded)
	assert.Equal(t, "10000000000", resp.ConsensusBlockValue)

	var blockContents structs.BlockContentsGloas
	require.NoError(t, json.Unmarshal(resp.Data, &blockContents))
	assert.NotNil(t, blockContents.Block)
	assert.NotNil(t, blockContents.ExecutionPayloadEnvelope)

	require.Equal(t, "gloas", writer.Header().Get(api.VersionHeader))
	require.Equal(t, "10000000000", writer.Header().Get(api.ConsensusBlockValueHeader))
	require.Equal(t, "true", writer.Header().Get(api.ExecutionPayloadIncludedHeader))
}

// TestProduceBlockV4_IncludePayloadTrue_PopulatedCache covers the cache-hit
// path: when the producer has cached data column sidecars for this slot, the
// v4 response derives raw blobs and flat KZG proofs from them and embeds them
// in the BlockContentsGloas body.
func TestProduceBlockV4_IncludePayloadTrue_PopulatedCache(t *testing.T) {
	require.NoError(t, kzg.Start())

	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	const blobCount = 2
	rawBlobs := make([]kzg.Blob, blobCount)
	for i := range rawBlobs {
		rawBlobs[i] = kzg.Blob{uint8(i + 1)}
	}
	cellsPerBlob, proofsPerBlob := util.GenerateCellsAndProofs(t, rawBlobs)
	envelope := testEnvelope()
	blockRoot := bytesutil.ToBytes32(envelope.BeaconBlockRoot)
	roSidecars, err := peerdas.DataColumnSidecarsGloas(cellsPerBlob, proofsPerBlob, primitives.Slot(envelope.Payload.SlotNumber), blockRoot)
	require.NoError(t, err)

	envCache := cache.NewExecutionPayloadEnvelopeCache()
	envCache.Set(&cache.ExecutionPayloadContents{
		Envelope:    envelope,
		DataColumns: roSidecars,
	})

	ctrl := gomock.NewController(t)
	v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
	v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(gloasGenericBlock(), nil)
	v1alpha1Server.EXPECT().GetExecutionPayloadEnvelope(gomock.Any(), gomock.Any()).Return(
		&eth.ExecutionPayloadEnvelopeResponse{Envelope: envelope}, nil,
	)

	server := &Server{
		V1Alpha1Server:                v1alpha1Server,
		ExecutionPayloadEnvelopeCache: envCache,
		SyncChecker:                   &mockSync.Sync{IsSyncing: false},
		OptimisticModeFetcher:         &blockchainTesting.ChainService{},
		BlockRewardFetcher:            &rewardtesting.MockBlockRewardFetcher{Rewards: &structs.BlockRewards{Total: "10"}},
	}

	request := httptest.NewRequest(http.MethodGet, fmt.Sprintf("http://foo.example/eth/v4/validator/blocks/1?randao_reveal=%s&graffiti=%s", testRandao, testGraffiti), nil)
	request.SetPathValue("slot", "1")
	writer := httptest.NewRecorder()
	writer.Body = &bytes.Buffer{}
	server.ProduceBlockV4(writer, request)
	require.Equal(t, http.StatusOK, writer.Code)

	var resp structs.ProduceBlockV4Response
	require.NoError(t, json.Unmarshal(writer.Body.Bytes(), &resp))
	var blockContents structs.BlockContentsGloas
	require.NoError(t, json.Unmarshal(resp.Data, &blockContents))
	require.Equal(t, blobCount, len(blockContents.Blobs))
	require.Equal(t, blobCount*fieldparams.NumberOfColumns, len(blockContents.KzgProofs))
}

func TestProduceBlockV4_IncludePayloadFalse(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	ctrl := gomock.NewController(t)
	v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
	v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(gloasGenericBlock(), nil)

	server := &Server{
		V1Alpha1Server:        v1alpha1Server,
		SyncChecker:           &mockSync.Sync{IsSyncing: false},
		OptimisticModeFetcher: &blockchainTesting.ChainService{},
		BlockRewardFetcher:    &rewardtesting.MockBlockRewardFetcher{Rewards: &structs.BlockRewards{Total: "10"}},
	}
	request := httptest.NewRequest(http.MethodGet, fmt.Sprintf("http://foo.example/eth/v4/validator/blocks/1?randao_reveal=%s&graffiti=%s&include_payload=false", testRandao, testGraffiti), nil)
	request.SetPathValue("slot", "1")
	writer := httptest.NewRecorder()
	writer.Body = &bytes.Buffer{}
	server.ProduceBlockV4(writer, request)
	assert.Equal(t, http.StatusOK, writer.Code)

	var resp structs.ProduceBlockV4Response
	require.NoError(t, json.Unmarshal(writer.Body.Bytes(), &resp))
	assert.Equal(t, "gloas", resp.Version)
	assert.Equal(t, false, resp.ExecutionPayloadIncluded)

	var block structs.BeaconBlockGloas
	require.NoError(t, json.Unmarshal(resp.Data, &block))
	assert.NotNil(t, block.Body)

	require.Equal(t, "gloas", writer.Header().Get(api.VersionHeader))
	require.Equal(t, "false", writer.Header().Get(api.ExecutionPayloadIncludedHeader))
}

func TestProduceBlockV4_PreGloasSlotRejected(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 100
	params.OverrideBeaconConfig(cfg)

	ctrl := gomock.NewController(t)
	v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
	server := &Server{
		V1Alpha1Server:        v1alpha1Server,
		SyncChecker:           &mockSync.Sync{IsSyncing: false},
		OptimisticModeFetcher: &blockchainTesting.ChainService{},
	}
	request := httptest.NewRequest(http.MethodGet, fmt.Sprintf("http://foo.example/eth/v4/validator/blocks/1?randao_reveal=%s&graffiti=%s", testRandao, testGraffiti), nil)
	request.SetPathValue("slot", "1")
	writer := httptest.NewRecorder()
	writer.Body = &bytes.Buffer{}
	server.ProduceBlockV4(writer, request)
	assert.Equal(t, http.StatusBadRequest, writer.Code)
	assert.StringContains(t, "only supported for Gloas", writer.Body.String())
}

func TestProduceBlockV4_Syncing(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	ctrl := gomock.NewController(t)
	chainService := &blockchainTesting.ChainService{}
	v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
	server := &Server{
		V1Alpha1Server:        v1alpha1Server,
		SyncChecker:           &mockSync.Sync{IsSyncing: true},
		HeadFetcher:           chainService,
		TimeFetcher:           chainService,
		OptimisticModeFetcher: chainService,
	}
	request := httptest.NewRequest(http.MethodGet, fmt.Sprintf("http://foo.example/eth/v4/validator/blocks/1?randao_reveal=%s&graffiti=%s", testRandao, testGraffiti), nil)
	writer := httptest.NewRecorder()
	writer.Body = &bytes.Buffer{}
	server.ProduceBlockV4(writer, request)
	assert.Equal(t, http.StatusServiceUnavailable, writer.Code)
}

func TestProduceBlockV4_SSZ_IncludePayloadTrue(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	ctrl := gomock.NewController(t)
	v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
	v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(gloasGenericBlock(), nil)
	v1alpha1Server.EXPECT().GetExecutionPayloadEnvelope(gomock.Any(), gomock.Any()).Return(
		&eth.ExecutionPayloadEnvelopeResponse{Envelope: testEnvelope()}, nil,
	)

	server := &Server{
		V1Alpha1Server:        v1alpha1Server,
		SyncChecker:           &mockSync.Sync{IsSyncing: false},
		OptimisticModeFetcher: &blockchainTesting.ChainService{},
		BlockRewardFetcher:    &rewardtesting.MockBlockRewardFetcher{Rewards: &structs.BlockRewards{Total: "10"}},
	}
	request := httptest.NewRequest(http.MethodGet, fmt.Sprintf("http://foo.example/eth/v4/validator/blocks/1?randao_reveal=%s&graffiti=%s", testRandao, testGraffiti), nil)
	request.SetPathValue("slot", "1")
	request.Header.Set("Accept", "application/octet-stream")
	writer := httptest.NewRecorder()
	writer.Body = &bytes.Buffer{}
	server.ProduceBlockV4(writer, request)
	assert.Equal(t, http.StatusOK, writer.Code)
	assert.Equal(t, "application/octet-stream", writer.Header().Get("Content-Type"))
	assert.Equal(t, true, writer.Body.Len() > 0)
}

func TestProduceBlockV4_SSZ_IncludePayloadFalse(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	ctrl := gomock.NewController(t)
	v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
	v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(gloasGenericBlock(), nil)

	server := &Server{
		V1Alpha1Server:        v1alpha1Server,
		SyncChecker:           &mockSync.Sync{IsSyncing: false},
		OptimisticModeFetcher: &blockchainTesting.ChainService{},
		BlockRewardFetcher:    &rewardtesting.MockBlockRewardFetcher{Rewards: &structs.BlockRewards{Total: "10"}},
	}
	request := httptest.NewRequest(http.MethodGet, fmt.Sprintf("http://foo.example/eth/v4/validator/blocks/1?randao_reveal=%s&graffiti=%s&include_payload=false", testRandao, testGraffiti), nil)
	request.SetPathValue("slot", "1")
	request.Header.Set("Accept", "application/octet-stream")
	writer := httptest.NewRecorder()
	writer.Body = &bytes.Buffer{}
	server.ProduceBlockV4(writer, request)
	assert.Equal(t, http.StatusOK, writer.Code)
	assert.Equal(t, "application/octet-stream", writer.Header().Get("Content-Type"))
}
