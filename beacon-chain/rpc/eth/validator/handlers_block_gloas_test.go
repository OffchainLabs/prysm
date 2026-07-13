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
	blockchainTesting "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	rewardtesting "github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/eth/rewards/testing"
	mockSync "github.com/OffchainLabs/prysm/v7/beacon-chain/sync/initial-sync/testing"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	mock2 "github.com/OffchainLabs/prysm/v7/testing/mock"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/ethereum/go-ethereum/common/hexutil"
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
		ExecutionRequests:     &enginev1.ExecutionRequestsGloas{},
		BuilderIndex:          0,
		BeaconBlockRoot:       make([]byte, 32),
		ParentBeaconBlockRoot: make([]byte, 32),
	}
}

func gloasGenericBlock() *eth.GenericBeaconBlock {
	return gloasGenericBlockWithBuilder(params.BeaconConfig().BuilderIndexSelfBuild)
}

func gloasGenericBlockWithBuilder(builderIndex primitives.BuilderIndex) *eth.GenericBeaconBlock {
	blk := util.NewBeaconBlockGloas().Block
	blk.Body.SignedExecutionPayloadBid.Message.BuilderIndex = builderIndex
	return &eth.GenericBeaconBlock{
		Block: &eth.GenericBeaconBlock_Gloas{Gloas: blk},
	}
}

// gloasGenericBlockContents mirrors a self-built block: the producer bundles the envelope inline.
func gloasGenericBlockContents() *eth.GenericBeaconBlock {
	return &eth.GenericBeaconBlock{
		Block: &eth.GenericBeaconBlock_GloasContents{
			GloasContents: &eth.BeaconBlockContentsGloas{
				Block:                    util.NewBeaconBlockGloas().Block,
				ExecutionPayloadEnvelope: testEnvelope(),
			},
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
	v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(gloasGenericBlockContents(), nil)

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

// TestProduceBlockV4_IncludePayloadTrue_WithBlobs covers the blob path: the producer bundles
// raw blobs and flat KZG proofs in GloasContents, which the v4 response embeds in the body.
func TestProduceBlockV4_IncludePayloadTrue_WithBlobs(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	const blobCount = 2
	blobs := make([][]byte, blobCount)
	for i := range blobs {
		blobs[i] = []byte{byte(i + 1)}
	}
	proofs := make([][]byte, blobCount*fieldparams.NumberOfColumns)
	for i := range proofs {
		proofs[i] = make([]byte, 48)
	}

	contents := gloasGenericBlockContents()
	gc := contents.Block.(*eth.GenericBeaconBlock_GloasContents).GloasContents
	gc.Blobs = blobs
	gc.KzgProofs = proofs

	ctrl := gomock.NewController(t)
	v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
	v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(contents, nil)

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

// An external builder bid returns only the block, even with include_payload=true.
func TestProduceBlockV4_BuilderBidExcludesPayload(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	ctrl := gomock.NewController(t)
	v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
	// External builder bid: the producer returns the block alone (no inline contents), so the payload is excluded.
	v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(gloasGenericBlockWithBuilder(3), nil)

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
	require.Equal(t, http.StatusOK, writer.Code)

	var resp structs.ProduceBlockV4Response
	require.NoError(t, json.Unmarshal(writer.Body.Bytes(), &resp))
	assert.Equal(t, false, resp.ExecutionPayloadIncluded)
	require.Equal(t, "false", writer.Header().Get(api.ExecutionPayloadIncludedHeader))

	var block structs.BeaconBlockGloas
	require.NoError(t, json.Unmarshal(resp.Data, &block))
	assert.NotNil(t, block.Body)
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
	v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(gloasGenericBlockContents(), nil)

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

// GET returns blinded SSZ that must roundtrip with HTR matching the full envelope.
func TestExecutionPayloadEnvelope_SSZ(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	ctrl := gomock.NewController(t)
	envelope := testEnvelope()
	wireBlinded, err := envelope.WireBlinded()
	require.NoError(t, err)
	v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
	v1alpha1Server.EXPECT().GetExecutionPayloadEnvelope(gomock.Any(), gomock.Any()).Return(
		&eth.ExecutionPayloadEnvelopeResponse{Blinded: wireBlinded}, nil,
	)

	server := &Server{V1Alpha1Server: v1alpha1Server}
	bbrHex := hexutil.Encode(envelope.BeaconBlockRoot)
	request := httptest.NewRequest(http.MethodGet, "http://foo.example/eth/v1/validator/execution_payload_envelope/1/"+bbrHex, nil)
	request.SetPathValue("slot", "1")
	request.SetPathValue("beacon_block_root", bbrHex)
	request.Header.Set("Accept", "application/octet-stream")
	writer := httptest.NewRecorder()
	writer.Body = &bytes.Buffer{}
	server.ExecutionPayloadEnvelope(writer, request)
	assert.Equal(t, http.StatusOK, writer.Code)
	assert.Equal(t, "application/octet-stream", writer.Header().Get("Content-Type"))
	assert.Equal(t, version.String(version.Gloas), writer.Header().Get("Eth-Consensus-Version"))

	blinded := &eth.WireBlindedExecutionPayloadEnvelope{}
	require.NoError(t, blinded.UnmarshalSSZ(writer.Body.Bytes()))
	wantHTR, err := envelope.HashTreeRoot()
	require.NoError(t, err)
	gotHTR, err := blinded.HashTreeRoot()
	require.NoError(t, err)
	assert.Equal(t, wantHTR, gotHTR)
}

func TestExecutionPayloadEnvelope_BeaconBlockRootMismatch(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	ctrl := gomock.NewController(t)
	envelope := testEnvelope()
	wireBlinded, err := envelope.WireBlinded()
	require.NoError(t, err)
	v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
	v1alpha1Server.EXPECT().GetExecutionPayloadEnvelope(gomock.Any(), gomock.Any()).Return(
		&eth.ExecutionPayloadEnvelopeResponse{Blinded: wireBlinded}, nil,
	)

	server := &Server{V1Alpha1Server: v1alpha1Server}
	requested := make([]byte, 32)
	requested[0] = 1 // differs from the cached envelope's zero root
	bbrHex := hexutil.Encode(requested)
	request := httptest.NewRequest(http.MethodGet, "http://foo.example/eth/v1/validator/execution_payload_envelope/1/"+bbrHex, nil)
	request.SetPathValue("slot", "1")
	request.SetPathValue("beacon_block_root", bbrHex)
	writer := httptest.NewRecorder()
	writer.Body = &bytes.Buffer{}
	server.ExecutionPayloadEnvelope(writer, request)
	assert.Equal(t, http.StatusNotFound, writer.Code)
	assert.StringContains(t, "does not match", writer.Body.String())
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
