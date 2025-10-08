package beacon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v6/api"
	"github.com/OffchainLabs/prysm/v6/api/server/structs"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/kzg"
	chainMock "github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/cache/depositsnapshot"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db"
	dbTest "github.com/OffchainLabs/prysm/v6/beacon-chain/db/testing"
	doublylinkedtree "github.com/OffchainLabs/prysm/v6/beacon-chain/forkchoice/doubly-linked-tree"
	mockp2p "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/testing"
	rpctesting "github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/eth/shared/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/lookup"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/testutil"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	mockSync "github.com/OffchainLabs/prysm/v6/beacon-chain/sync/initial-sync/testing"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/crypto/bls"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v6/network/httputil"
	eth "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	mock2 "github.com/OffchainLabs/prysm/v6/testing/mock"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
	ssz "github.com/prysmaticlabs/fastssz"
	"github.com/prysmaticlabs/go-bitfield"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/mock"
	"go.uber.org/mock/gomock"
)

func fillDBTestBlocks(ctx context.Context, t *testing.T, beaconDB db.Database) (*eth.SignedBeaconBlock, []*eth.BeaconBlockContainer) {
	parentRoot := [32]byte{1, 2, 3}
	genBlk := util.NewBeaconBlock()
	genBlk.Block.ParentRoot = parentRoot[:]
	root, err := genBlk.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, beaconDB, genBlk)
	require.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, root))

	count := primitives.Slot(100)
	blks := make([]interfaces.ReadOnlySignedBeaconBlock, count)
	blkContainers := make([]*eth.BeaconBlockContainer, count)
	for i := primitives.Slot(0); i < count; i++ {
		b := util.NewBeaconBlock()
		b.Block.Slot = i
		b.Block.ParentRoot = bytesutil.PadTo([]byte{uint8(i)}, 32)
		root, err := b.Block.HashTreeRoot()
		require.NoError(t, err)
		blks[i], err = blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		blkContainers[i] = &eth.BeaconBlockContainer{
			Block:     &eth.BeaconBlockContainer_Phase0Block{Phase0Block: b},
			BlockRoot: root[:],
		}
	}
	require.NoError(t, beaconDB.SaveBlocks(ctx, blks))
	headRoot := bytesutil.ToBytes32(blkContainers[len(blks)-1].BlockRoot)
	summary := &eth.StateSummary{
		Root: headRoot[:],
		Slot: blkContainers[len(blks)-1].Block.(*eth.BeaconBlockContainer_Phase0Block).Phase0Block.Block.Slot,
	}
	require.NoError(t, beaconDB.SaveStateSummary(ctx, summary))
	require.NoError(t, beaconDB.SaveHeadBlockRoot(ctx, headRoot))
	return genBlk, blkContainers
}

func TestGetBlockV2(t *testing.T) {
	t.Run("Unsycned Block", func(t *testing.T) {
		mockBlockFetcher := &testutil.MockBlocker{BlockToReturn: nil}
		mockChainService := &chainMock.ChainService{
			FinalizedRoots: map[[32]byte]bool{},
		}
		s := &Server{
			FinalizationFetcher: mockChainService,
			Blocker:             mockBlockFetcher,
		}

		request := httptest.NewRequest(http.MethodGet, "http://foo.example/eth/v2/beacon/blocks/{block_id}", nil)
		request.SetPathValue("block_id", "123552314")
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetBlockV2(writer, request)
		require.Equal(t, http.StatusNotFound, writer.Code)
	})
	t.Run("phase0", func(t *testing.T) {
		b := util.NewBeaconBlock()
		b.Block.Slot = 123
		sb, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		mockBlockFetcher := &testutil.MockBlocker{BlockToReturn: sb}
		mockChainService := &chainMock.ChainService{
			FinalizedRoots: map[[32]byte]bool{},
		}
		s := &Server{
			FinalizationFetcher: mockChainService,
			Blocker:             mockBlockFetcher,
		}

		request := httptest.NewRequest(http.MethodGet, "http://foo.example/eth/v2/beacon/blocks/{block_id}", nil)
		request.SetPathValue("block_id", "head")
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetBlockV2(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetBlockV2Response{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, version.String(version.Phase0), resp.Version)
		sbb := &structs.SignedBeaconBlock{Message: &structs.BeaconBlock{}}
		require.NoError(t, json.Unmarshal(resp.Data.Message, sbb.Message))
		sbb.Signature = resp.Data.Signature
		genericBlk, err := sbb.ToGeneric()
		require.NoError(t, err)
		blk := genericBlk.GetPhase0()
		require.NoError(t, err)
		assert.DeepEqual(t, blk, b)
	})
	t.Run("altair", func(t *testing.T) {
		b := util.NewBeaconBlockAltair()
		b.Block.Slot = 123
		sb, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		mockBlockFetcher := &testutil.MockBlocker{BlockToReturn: sb}
		mockChainService := &chainMock.ChainService{
			FinalizedRoots: map[[32]byte]bool{},
		}
		s := &Server{
			FinalizationFetcher: mockChainService,
			Blocker:             mockBlockFetcher,
		}

		request := httptest.NewRequest(http.MethodGet, "http://foo.example/eth/v2/beacon/blocks/{block_id}", nil)
		request.SetPathValue("block_id", "head")
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetBlockV2(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetBlockV2Response{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, version.String(version.Altair), resp.Version)
		sbb := &structs.SignedBeaconBlockAltair{Message: &structs.BeaconBlockAltair{}}
		require.NoError(t, json.Unmarshal(resp.Data.Message, sbb.Message))
		sbb.Signature = resp.Data.Signature
		genericBlk, err := sbb.ToGeneric()
		require.NoError(t, err)
		blk := genericBlk.GetAltair()
		require.NoError(t, err)
		assert.DeepEqual(t, blk, b)
	})
	t.Run("bellatrix", func(t *testing.T) {
		b := util.NewBeaconBlockBellatrix()
		b.Block.Slot = 123
		sb, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		mockBlockFetcher := &testutil.MockBlocker{BlockToReturn: sb}
		mockChainService := &chainMock.ChainService{
			FinalizedRoots: map[[32]byte]bool{},
		}
		s := &Server{
			OptimisticModeFetcher: mockChainService,
			FinalizationFetcher:   mockChainService,
			Blocker:               mockBlockFetcher,
		}

		request := httptest.NewRequest(http.MethodGet, "http://foo.example/eth/v2/beacon/blocks/{block_id}", nil)
		request.SetPathValue("block_id", "head")
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetBlockV2(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetBlockV2Response{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, version.String(version.Bellatrix), resp.Version)
		sbb := &structs.SignedBeaconBlockBellatrix{Message: &structs.BeaconBlockBellatrix{}}
		require.NoError(t, json.Unmarshal(resp.Data.Message, sbb.Message))
		sbb.Signature = resp.Data.Signature
		genericBlk, err := sbb.ToGeneric()
		require.NoError(t, err)
		blk := genericBlk.GetBellatrix()
		require.NoError(t, err)
		assert.DeepEqual(t, blk, b)
	})
	t.Run("capella", func(t *testing.T) {
		b := util.NewBeaconBlockCapella()
		b.Block.Slot = 123
		sb, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		mockBlockFetcher := &testutil.MockBlocker{BlockToReturn: sb}
		mockChainService := &chainMock.ChainService{
			FinalizedRoots: map[[32]byte]bool{},
		}
		s := &Server{
			OptimisticModeFetcher: mockChainService,
			FinalizationFetcher:   mockChainService,
			Blocker:               mockBlockFetcher,
		}

		request := httptest.NewRequest(http.MethodGet, "http://foo.example/eth/v2/beacon/blocks/{block_id}", nil)
		request.SetPathValue("block_id", "head")
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetBlockV2(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetBlockV2Response{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, version.String(version.Capella), resp.Version)
		sbb := &structs.SignedBeaconBlockCapella{Message: &structs.BeaconBlockCapella{}}
		require.NoError(t, json.Unmarshal(resp.Data.Message, sbb.Message))
		sbb.Signature = resp.Data.Signature
		genericBlk, err := sbb.ToGeneric()
		require.NoError(t, err)
		blk := genericBlk.GetCapella()
		require.NoError(t, err)
		assert.DeepEqual(t, blk, b)
	})
	t.Run("deneb", func(t *testing.T) {
		b := util.NewBeaconBlockDeneb()
		b.Block.Slot = 123
		sb, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		mockBlockFetcher := &testutil.MockBlocker{BlockToReturn: sb}
		mockChainService := &chainMock.ChainService{
			FinalizedRoots: map[[32]byte]bool{},
		}
		s := &Server{
			OptimisticModeFetcher: mockChainService,
			FinalizationFetcher:   mockChainService,
			Blocker:               mockBlockFetcher,
		}

		request := httptest.NewRequest(http.MethodGet, "http://foo.example/eth/v2/beacon/blocks/{block_id}", nil)
		request.SetPathValue("block_id", "head")
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetBlockV2(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetBlockV2Response{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, version.String(version.Deneb), resp.Version)
		sbb := &structs.SignedBeaconBlockDeneb{Message: &structs.BeaconBlockDeneb{}}
		require.NoError(t, json.Unmarshal(resp.Data.Message, sbb.Message))
		sbb.Signature = resp.Data.Signature
		blk, err := sbb.ToConsensus()
		require.NoError(t, err)
		assert.DeepEqual(t, blk, b)
	})
	t.Run("electra", func(t *testing.T) {
		b := util.NewBeaconBlockElectra()
		b.Block.Slot = 123
		sb, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		mockBlockFetcher := &testutil.MockBlocker{BlockToReturn: sb}
		mockChainService := &chainMock.ChainService{
			FinalizedRoots: map[[32]byte]bool{},
		}
		s := &Server{
			OptimisticModeFetcher: mockChainService,
			FinalizationFetcher:   mockChainService,
			Blocker:               mockBlockFetcher,
		}

		request := httptest.NewRequest(http.MethodGet, "http://foo.example/eth/v2/beacon/blocks/{block_id}", nil)
		request.SetPathValue("block_id", "head")
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetBlockV2(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetBlockV2Response{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, version.String(version.Electra), resp.Version)
		sbb := &structs.SignedBeaconBlockElectra{Message: &structs.BeaconBlockElectra{}}
		require.NoError(t, json.Unmarshal(resp.Data.Message, sbb.Message))
		sbb.Signature = resp.Data.Signature
		blk, err := sbb.ToConsensus()
		require.NoError(t, err)
		assert.DeepEqual(t, blk, b)
	})
	t.Run("fulu", func(t *testing.T) {
		b := util.NewBeaconBlockFulu()
		b.Block.Slot = 123
		sb, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		mockBlockFetcher := &testutil.MockBlocker{BlockToReturn: sb}
		mockChainService := &chainMock.ChainService{
			FinalizedRoots: map[[32]byte]bool{},
		}
		s := &Server{
			OptimisticModeFetcher: mockChainService,
			FinalizationFetcher:   mockChainService,
			Blocker:               mockBlockFetcher,
		}

		request := httptest.NewRequest(http.MethodGet, "http://foo.example/eth/v2/beacon/blocks/{block_id}", nil)
		request.SetPathValue("block_id", "head")
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetBlockV2(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetBlockV2Response{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, version.String(version.Fulu), resp.Version)
		sbb := &structs.SignedBeaconBlockFulu{Message: &structs.BeaconBlockElectra{}}
		require.NoError(t, json.Unmarshal(resp.Data.Message, sbb.Message))
		sbb.Signature = resp.Data.Signature
		blk, err := sbb.ToConsensus()
		require.NoError(t, err)
		assert.DeepEqual(t, blk, b)
	})
	t.Run("execution optimistic", func(t *testing.T) {
		b := util.NewBeaconBlockBellatrix()
		sb, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		r, err := sb.Block().HashTreeRoot()
		require.NoError(t, err)
		mockBlockFetcher := &testutil.MockBlocker{BlockToReturn: sb}
		mockChainService := &chainMock.ChainService{
			OptimisticRoots: map[[32]byte]bool{r: true},
			FinalizedRoots:  map[[32]byte]bool{},
		}
		s := &Server{
			OptimisticModeFetcher: mockChainService,
			FinalizationFetcher:   mockChainService,
			Blocker:               mockBlockFetcher,
		}

		request := httptest.NewRequest(http.MethodGet, "http://foo.example/eth/v2/beacon/blocks/{block_id}", nil)
		request.SetPathValue("block_id", "head")
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetBlockV2(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetBlockV2Response{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, true, resp.ExecutionOptimistic)
	})
	t.Run("finalized", func(t *testing.T) {
		b := util.NewBeaconBlock()
		sb, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		r, err := sb.Block().HashTreeRoot()
		require.NoError(t, err)
		mockBlockFetcher := &testutil.MockBlocker{BlockToReturn: sb}

		t.Run("true", func(t *testing.T) {
			mockChainService := &chainMock.ChainService{FinalizedRoots: map[[32]byte]bool{r: true}}
			s := &Server{
				OptimisticModeFetcher: mockChainService,
				FinalizationFetcher:   mockChainService,
				Blocker:               mockBlockFetcher,
			}

			request := httptest.NewRequest(http.MethodGet, "http://foo.example/eth/v2/beacon/blocks/{block_id}", nil)
			request.SetPathValue("block_id", hexutil.Encode(r[:]))
			writer := httptest.NewRecorder()
			writer.Body = &bytes.Buffer{}

			s.GetBlockV2(writer, request)
			require.Equal(t, http.StatusOK, writer.Code)
			resp := &structs.GetBlockV2Response{}
			require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
			assert.Equal(t, true, resp.Finalized)
		})
		t.Run("false", func(t *testing.T) {
			mockChainService := &chainMock.ChainService{FinalizedRoots: map[[32]byte]bool{r: false}}
			s := &Server{
				OptimisticModeFetcher: mockChainService,
				FinalizationFetcher:   mockChainService,
				Blocker:               mockBlockFetcher,
			}

			request := httptest.NewRequest(http.MethodGet, "http://foo.example/eth/v2/beacon/blocks/{block_id}", nil)
			request.SetPathValue("block_id", hexutil.Encode(r[:]))
			writer := httptest.NewRecorder()
			writer.Body = &bytes.Buffer{}

			s.GetBlockV2(writer, request)
			require.Equal(t, http.StatusOK, writer.Code)
			resp := &structs.GetBlockV2Response{}
			require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
			assert.Equal(t, false, resp.Finalized)
		})
	})
}

func TestGetBlockSSZV2(t *testing.T) {
	t.Run("phase0", func(t *testing.T) {
		b := util.NewBeaconBlock()
		b.Block.Slot = 123
		sb, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)

		s := &Server{
			Blocker: &testutil.MockBlocker{BlockToReturn: sb},
		}

		request := httptest.NewRequest(http.MethodGet, "http://foo.example/eth/v2/beacon/blocks/{block_id}", nil)
		request.SetPathValue("block_id", "head")
		request.Header.Set("Accept", api.OctetStreamMediaType)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetBlockV2(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		assert.Equal(t, version.String(version.Phase0), writer.Header().Get(api.VersionHeader))
		sszExpected, err := b.MarshalSSZ()
		require.NoError(t, err)
		assert.DeepEqual(t, sszExpected, writer.Body.Bytes())
	})
	t.Run("altair", func(t *testing.T) {
		b := util.NewBeaconBlockAltair()
		b.Block.Slot = 123
		sb, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)

		s := &Server{
			Blocker: &testutil.MockBlocker{BlockToReturn: sb},
		}

		request := httptest.NewRequest(http.MethodGet, "http://foo.example/eth/v2/beacon/blocks/{block_id}", nil)
		request.SetPathValue("block_id", "head")
		request.Header.Set("Accept", api.OctetStreamMediaType)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetBlockV2(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		assert.Equal(t, version.String(version.Altair), writer.Header().Get(api.VersionHeader))
		sszExpected, err := b.MarshalSSZ()
		require.NoError(t, err)
		assert.DeepEqual(t, sszExpected, writer.Body.Bytes())
	})
	t.Run("bellatrix", func(t *testing.T) {
		b := util.NewBeaconBlockBellatrix()
		b.Block.Slot = 123
		sb, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)

		s := &Server{
			Blocker: &testutil.MockBlocker{BlockToReturn: sb},
		}

		request := httptest.NewRequest(http.MethodGet, "http://foo.example/eth/v2/beacon/blocks/{block_id}", nil)
		request.SetPathValue("block_id", "head")
		request.Header.Set("Accept", api.OctetStreamMediaType)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetBlockV2(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		assert.Equal(t, version.String(version.Bellatrix), writer.Header().Get(api.VersionHeader))
		sszExpected, err := b.MarshalSSZ()
		require.NoError(t, err)
		assert.DeepEqual(t, sszExpected, writer.Body.Bytes())
	})
	t.Run("capella", func(t *testing.T) {
		b := util.NewBeaconBlockCapella()
		b.Block.Slot = 123
		sb, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)

		s := &Server{
			Blocker: &testutil.MockBlocker{BlockToReturn: sb},
		}

		request := httptest.NewRequest(http.MethodGet, "http://foo.example/eth/v2/beacon/blocks/{block_id}", nil)
		request.SetPathValue("block_id", "head")
		request.Header.Set("Accept", api.OctetStreamMediaType)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetBlockV2(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		assert.Equal(t, version.String(version.Capella), writer.Header().Get(api.VersionHeader))
		sszExpected, err := b.MarshalSSZ()
		require.NoError(t, err)
		assert.DeepEqual(t, sszExpected, writer.Body.Bytes())
	})
	t.Run("deneb", func(t *testing.T) {
		b := util.NewBeaconBlockDeneb()
		b.Block.Slot = 123
		sb, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)

		s := &Server{
			Blocker: &testutil.MockBlocker{BlockToReturn: sb},
		}

		request := httptest.NewRequest(http.MethodGet, "http://foo.example/eth/v2/beacon/blocks/{block_id}", nil)
		request.SetPathValue("block_id", "head")
		request.Header.Set("Accept", api.OctetStreamMediaType)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetBlockV2(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		assert.Equal(t, version.String(version.Deneb), writer.Header().Get(api.VersionHeader))
		sszExpected, err := b.MarshalSSZ()
		require.NoError(t, err)
		assert.DeepEqual(t, sszExpected, writer.Body.Bytes())
	})
	t.Run("electra", func(t *testing.T) {
		b := util.NewBeaconBlockElectra()
		b.Block.Slot = 123
		sb, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)

		s := &Server{
			Blocker: &testutil.MockBlocker{BlockToReturn: sb},
		}

		request := httptest.NewRequest(http.MethodGet, "http://foo.example/eth/v2/beacon/blocks/{block_id}", nil)
		request.SetPathValue("block_id", "head")
		request.Header.Set("Accept", api.OctetStreamMediaType)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetBlockV2(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		assert.Equal(t, version.String(version.Electra), writer.Header().Get(api.VersionHeader))
		sszExpected, err := b.MarshalSSZ()
		require.NoError(t, err)
		assert.DeepEqual(t, sszExpected, writer.Body.Bytes())
	})
	t.Run("fulu", func(t *testing.T) {
		b := util.NewBeaconBlockFulu()
		b.Block.Slot = 123
		sb, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)

		s := &Server{
			Blocker: &testutil.MockBlocker{BlockToReturn: sb},
		}

		request := httptest.NewRequest(http.MethodGet, "http://foo.example/eth/v2/beacon/blocks/{block_id}", nil)
		request.SetPathValue("block_id", "head")
		request.Header.Set("Accept", api.OctetStreamMediaType)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetBlockV2(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		assert.Equal(t, version.String(version.Fulu), writer.Header().Get(api.VersionHeader))
		sszExpected, err := b.MarshalSSZ()
		require.NoError(t, err)
		assert.DeepEqual(t, sszExpected, writer.Body.Bytes())
	})
}

func TestGetBlockAttestations(t *testing.T) {
	preElectraAtts := []*eth.Attestation{
		{
			AggregationBits: bitfield.Bitlist{0x00},
			Data: &eth.AttestationData{
				Slot:            123,
				CommitteeIndex:  123,
				BeaconBlockRoot: bytesutil.PadTo([]byte("root1"), 32),
				Source: &eth.Checkpoint{
					Epoch: 123,
					Root:  bytesutil.PadTo([]byte("root1"), 32),
				},
				Target: &eth.Checkpoint{
					Epoch: 123,
					Root:  bytesutil.PadTo([]byte("root1"), 32),
				},
			},
			Signature: bytesutil.PadTo([]byte("sig1"), 96),
		},
		{
			AggregationBits: bitfield.Bitlist{0x01},
			Data: &eth.AttestationData{
				Slot:            456,
				CommitteeIndex:  456,
				BeaconBlockRoot: bytesutil.PadTo([]byte("root2"), 32),
				Source: &eth.Checkpoint{
					Epoch: 456,
					Root:  bytesutil.PadTo([]byte("root2"), 32),
				},
				Target: &eth.Checkpoint{
					Epoch: 456,
					Root:  bytesutil.PadTo([]byte("root2"), 32),
				},
			},
			Signature: bytesutil.PadTo([]byte("sig2"), 96),
		},
	}
	electraAtts := []*eth.AttestationElectra{
		{
			AggregationBits: bitfield.Bitlist{0x00},
			Data: &eth.AttestationData{
				Slot:            123,
				CommitteeIndex:  123,
				BeaconBlockRoot: bytesutil.PadTo([]byte("root1"), 32),
				Source: &eth.Checkpoint{
					Epoch: 123,
					Root:  bytesutil.PadTo([]byte("root1"), 32),
				},
				Target: &eth.Checkpoint{
					Epoch: 123,
					Root:  bytesutil.PadTo([]byte("root1"), 32),
				},
			},
			Signature:     bytesutil.PadTo([]byte("sig1"), 96),
			CommitteeBits: primitives.NewAttestationCommitteeBits(),
		},
		{
			AggregationBits: bitfield.Bitlist{0x01},
			Data: &eth.AttestationData{
				Slot:            456,
				CommitteeIndex:  456,
				BeaconBlockRoot: bytesutil.PadTo([]byte("root2"), 32),
				Source: &eth.Checkpoint{
					Epoch: 456,
					Root:  bytesutil.PadTo([]byte("root2"), 32),
				},
				Target: &eth.Checkpoint{
					Epoch: 456,
					Root:  bytesutil.PadTo([]byte("root2"), 32),
				},
			},
			Signature:     bytesutil.PadTo([]byte("sig2"), 96),
			CommitteeBits: primitives.NewAttestationCommitteeBits(),
		},
	}

	b := util.NewBeaconBlock()
	b.Block.Body.Attestations = preElectraAtts
	sb, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)

	bb := util.NewBeaconBlockBellatrix()
	bb.Block.Body.Attestations = preElectraAtts
	bsb, err := blocks.NewSignedBeaconBlock(bb)
	require.NoError(t, err)

	eb := util.NewBeaconBlockElectra()
	eb.Block.Body.Attestations = electraAtts
	esb, err := blocks.NewSignedBeaconBlock(eb)
	require.NoError(t, err)

	t.Run("v1", func(t *testing.T) {
		t.Run("ok", func(t *testing.T) {
			mockChainService := &chainMock.ChainService{
				FinalizedRoots: map[[32]byte]bool{},
			}

			s := &Server{
				OptimisticModeFetcher: mockChainService,
				FinalizationFetcher:   mockChainService,
				Blocker:               &testutil.MockBlocker{BlockToReturn: sb},
			}

			request := httptest.NewRequest(http.MethodGet, "http://foo.example/eth/v1/beacon/blocks/{block_id}/attestations", nil)
			request.SetPathValue("block_id", "head")
			writer := httptest.NewRecorder()
			writer.Body = &bytes.Buffer{}

			s.GetBlockAttestations(writer, request)
			require.Equal(t, http.StatusOK, writer.Code)

			resp := &structs.GetBlockAttestationsResponse{}
			require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
			require.Equal(t, len(b.Block.Body.Attestations), len(resp.Data))

			atts := make([]*eth.Attestation, len(b.Block.Body.Attestations))
			for i, a := range resp.Data {
				atts[i], err = a.ToConsensus()
				require.NoError(t, err)
			}
			assert.DeepEqual(t, b.Block.Body.Attestations, atts)
		})
		t.Run("execution-optimistic", func(t *testing.T) {
			r, err := bsb.Block().HashTreeRoot()
			require.NoError(t, err)
			mockChainService := &chainMock.ChainService{
				OptimisticRoots: map[[32]byte]bool{r: true},
				FinalizedRoots:  map[[32]byte]bool{},
			}
			s := &Server{
				OptimisticModeFetcher: mockChainService,
				FinalizationFetcher:   mockChainService,
				Blocker:               &testutil.MockBlocker{BlockToReturn: bsb},
			}

			request := httptest.NewRequest(http.MethodGet, "http://foo.example/eth/v1/beacon/blocks/{block_id}/attestations", nil)
			request.SetPathValue("block_id", "head")
			writer := httptest.NewRecorder()
			writer.Body = &bytes.Buffer{}

			s.GetBlockAttestations(writer, request)
			require.Equal(t, http.StatusOK, writer.Code)
			resp := &structs.GetBlockAttestationsResponse{}
			require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
			assert.Equal(t, true, resp.ExecutionOptimistic)
		})
		t.Run("finalized", func(t *testing.T) {
			r, err := sb.Block().HashTreeRoot()
			require.NoError(t, err)

			t.Run("true", func(t *testing.T) {
				mockChainService := &chainMock.ChainService{FinalizedRoots: map[[32]byte]bool{r: true}}
				s := &Server{
					OptimisticModeFetcher: mockChainService,
					FinalizationFetcher:   mockChainService,
					Blocker:               &testutil.MockBlocker{BlockToReturn: sb},
				}

				request := httptest.NewRequest(http.MethodGet, "http://foo.example/eth/v1/beacon/blocks/{block_id}/attestations", nil)
				request.SetPathValue("block_id", "head")
				writer := httptest.NewRecorder()
				writer.Body = &bytes.Buffer{}

				s.GetBlockAttestations(writer, request)
				require.Equal(t, http.StatusOK, writer.Code)
				resp := &structs.GetBlockAttestationsResponse{}
				require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
				assert.Equal(t, true, resp.Finalized)
			})
			t.Run("false", func(t *testing.T) {
				mockChainService := &chainMock.ChainService{FinalizedRoots: map[[32]byte]bool{r: false}}
				s := &Server{
					OptimisticModeFetcher: mockChainService,
					FinalizationFetcher:   mockChainService,
					Blocker:               &testutil.MockBlocker{BlockToReturn: sb},
				}

				request := httptest.NewRequest(http.MethodGet, "http://foo.example/eth/v1/beacon/blocks/{block_id}/attestations", nil)
				request.SetPathValue("block_id", "head")
				writer := httptest.NewRecorder()
				writer.Body = &bytes.Buffer{}

				s.GetBlockAttestations(writer, request)
				require.Equal(t, http.StatusOK, writer.Code)
				resp := &structs.GetBlockAttestationsResponse{}
				require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
				assert.Equal(t, false, resp.ExecutionOptimistic)
			})
		})
	})

	t.Run("V2", func(t *testing.T) {
		t.Run("ok-pre-electra", func(t *testing.T) {
			mockChainService := &chainMock.ChainService{
				FinalizedRoots: map[[32]byte]bool{},
			}

			s := &Server{
				OptimisticModeFetcher: mockChainService,
				FinalizationFetcher:   mockChainService,
				Blocker:               &testutil.MockBlocker{BlockToReturn: sb},
			}

			request := httptest.NewRequest(http.MethodGet, "http://foo.example/eth/v2/beacon/blocks/{block_id}/attestations", nil)
			request.SetPathValue("block_id", "head")
			writer := httptest.NewRecorder()
			writer.Body = &bytes.Buffer{}

			s.GetBlockAttestationsV2(writer, request)
			require.Equal(t, http.StatusOK, writer.Code)

			resp := &structs.GetBlockAttestationsV2Response{}
			require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))

			var attStructs []structs.Attestation
			require.NoError(t, json.Unmarshal(resp.Data, &attStructs))

			atts := make([]*eth.Attestation, len(attStructs))
			for i, attStruct := range attStructs {
				atts[i], err = attStruct.ToConsensus()
				require.NoError(t, err)
			}

			assert.DeepEqual(t, b.Block.Body.Attestations, atts)
			assert.Equal(t, "phase0", resp.Version)
		})
		t.Run("ok-post-electra", func(t *testing.T) {
			mockChainService := &chainMock.ChainService{
				FinalizedRoots: map[[32]byte]bool{},
			}

			s := &Server{
				OptimisticModeFetcher: mockChainService,
				FinalizationFetcher:   mockChainService,
				Blocker:               &testutil.MockBlocker{BlockToReturn: esb},
			}

			mockBlockFetcher := &testutil.MockBlocker{BlockToReturn: esb}
			s.Blocker = mockBlockFetcher

			request := httptest.NewRequest(http.MethodGet, "http://foo.example/eth/v2/beacon/blocks/{block_id}/attestations", nil)
			request.SetPathValue("block_id", "head")
			writer := httptest.NewRecorder()
			writer.Body = &bytes.Buffer{}

			s.GetBlockAttestationsV2(writer, request)
			require.Equal(t, http.StatusOK, writer.Code)

			resp := &structs.GetBlockAttestationsV2Response{}
			require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))

			var attStructs []structs.AttestationElectra
			require.NoError(t, json.Unmarshal(resp.Data, &attStructs))

			atts := make([]*eth.AttestationElectra, len(attStructs))
			for i, attStruct := range attStructs {
				atts[i], err = attStruct.ToConsensus()
				require.NoError(t, err)
			}

			assert.DeepEqual(t, eb.Block.Body.Attestations, atts)
			assert.Equal(t, "electra", resp.Version)
		})
		t.Run("execution-optimistic", func(t *testing.T) {
			r, err := bsb.Block().HashTreeRoot()
			require.NoError(t, err)
			mockChainService := &chainMock.ChainService{
				OptimisticRoots: map[[32]byte]bool{r: true},
				FinalizedRoots:  map[[32]byte]bool{},
			}
			s := &Server{
				OptimisticModeFetcher: mockChainService,
				FinalizationFetcher:   mockChainService,
				Blocker:               &testutil.MockBlocker{BlockToReturn: bsb},
			}

			request := httptest.NewRequest(http.MethodGet, "http://foo.example/eth/v2/beacon/blocks/{block_id}/attestations", nil)
			request.SetPathValue("block_id", "head")
			writer := httptest.NewRecorder()
			writer.Body = &bytes.Buffer{}

			s.GetBlockAttestationsV2(writer, request)
			require.Equal(t, http.StatusOK, writer.Code)
			resp := &structs.GetBlockAttestationsV2Response{}
			require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
			assert.Equal(t, true, resp.ExecutionOptimistic)
			assert.Equal(t, "bellatrix", resp.Version)
		})
		t.Run("finalized", func(t *testing.T) {
			r, err := sb.Block().HashTreeRoot()
			require.NoError(t, err)

			t.Run("true", func(t *testing.T) {
				mockChainService := &chainMock.ChainService{FinalizedRoots: map[[32]byte]bool{r: true}}
				s := &Server{
					OptimisticModeFetcher: mockChainService,
					FinalizationFetcher:   mockChainService,
					Blocker:               &testutil.MockBlocker{BlockToReturn: sb},
				}

				request := httptest.NewRequest(http.MethodGet, "http://foo.example/eth/v2/beacon/blocks/{block_id}/attestations", nil)
				request.SetPathValue("block_id", "head")
				writer := httptest.NewRecorder()
				writer.Body = &bytes.Buffer{}

				s.GetBlockAttestationsV2(writer, request)
				require.Equal(t, http.StatusOK, writer.Code)
				resp := &structs.GetBlockAttestationsV2Response{}
				require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
				assert.Equal(t, true, resp.Finalized)
				assert.Equal(t, "phase0", resp.Version)
			})
			t.Run("false", func(t *testing.T) {
				mockChainService := &chainMock.ChainService{FinalizedRoots: map[[32]byte]bool{r: false}}
				s := &Server{
					OptimisticModeFetcher: mockChainService,
					FinalizationFetcher:   mockChainService,
					Blocker:               &testutil.MockBlocker{BlockToReturn: sb},
				}

				request := httptest.NewRequest(http.MethodGet, "http://foo.example/eth/v2/beacon/blocks/{block_id}/attestations", nil)
				request.SetPathValue("block_id", "head")
				writer := httptest.NewRecorder()
				writer.Body = &bytes.Buffer{}

				s.GetBlockAttestationsV2(writer, request)
				require.Equal(t, http.StatusOK, writer.Code)
				resp := &structs.GetBlockAttestationsV2Response{}
				require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
				assert.Equal(t, false, resp.ExecutionOptimistic)
				assert.Equal(t, "phase0", resp.Version)
			})
		})
	})

	t.Run("empty-attestations", func(t *testing.T) {
		t.Run("v1", func(t *testing.T) {
			b := util.NewBeaconBlock()
			b.Block.Body.Attestations = []*eth.Attestation{} // Explicitly set empty attestations
			sb, err := blocks.NewSignedBeaconBlock(b)
			require.NoError(t, err)

			mockChainService := &chainMock.ChainService{
				FinalizedRoots: map[[32]byte]bool{},
			}

			s := &Server{
				OptimisticModeFetcher: mockChainService,
				FinalizationFetcher:   mockChainService,
				Blocker:               &testutil.MockBlocker{BlockToReturn: sb},
			}

			request := httptest.NewRequest(http.MethodGet, "http://foo.example/eth/v1/beacon/blocks/{block_id}/attestations", nil)
			request.SetPathValue("block_id", "head")
			writer := httptest.NewRecorder()
			writer.Body = &bytes.Buffer{}

			s.GetBlockAttestations(writer, request)
			require.Equal(t, http.StatusOK, writer.Code)
			resp := &structs.GetBlockAttestationsResponse{}
			require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))

			// Ensure data is empty array, not null
			require.NotNil(t, resp.Data)
			assert.Equal(t, 0, len(resp.Data))
		})

		t.Run("v2-pre-electra", func(t *testing.T) {
			b := util.NewBeaconBlock()
			b.Block.Body.Attestations = []*eth.Attestation{} // Explicitly set empty attestations
			sb, err := blocks.NewSignedBeaconBlock(b)
			require.NoError(t, err)
			mockChainService := &chainMock.ChainService{
				FinalizedRoots: map[[32]byte]bool{},
			}

			s := &Server{
				OptimisticModeFetcher: mockChainService,
				FinalizationFetcher:   mockChainService,
				Blocker:               &testutil.MockBlocker{BlockToReturn: sb},
			}

			request := httptest.NewRequest(http.MethodGet, "http://foo.example/eth/v2/beacon/blocks/{block_id}/attestations", nil)
			request.SetPathValue("block_id", "head")
			writer := httptest.NewRecorder()
			writer.Body = &bytes.Buffer{}

			s.GetBlockAttestationsV2(writer, request)
			require.Equal(t, http.StatusOK, writer.Code)
			resp := &structs.GetBlockAttestationsV2Response{}
			require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
			// Ensure data is "[]", not null
			require.NotNil(t, resp.Data)
			assert.Equal(t, string(json.RawMessage("[]")), string(resp.Data))
		})

		t.Run("v2-electra", func(t *testing.T) {
			eb := util.NewBeaconBlockFulu()
			eb.Block.Body.Attestations = []*eth.AttestationElectra{} // Explicitly set empty attestations
			esb, err := blocks.NewSignedBeaconBlock(eb)
			require.NoError(t, err)

			mockChainService := &chainMock.ChainService{
				FinalizedRoots: map[[32]byte]bool{},
			}

			s := &Server{
				OptimisticModeFetcher: mockChainService,
				FinalizationFetcher:   mockChainService,
				Blocker:               &testutil.MockBlocker{BlockToReturn: esb},
			}

			request := httptest.NewRequest(http.MethodGet, "http://foo.example/eth/v2/beacon/blocks/{block_id}/attestations", nil)
			request.SetPathValue("block_id", "head")
			writer := httptest.NewRecorder()
			writer.Body = &bytes.Buffer{}

			s.GetBlockAttestationsV2(writer, request)
			require.Equal(t, http.StatusOK, writer.Code)
			resp := &structs.GetBlockAttestationsV2Response{}
			require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))

			// Ensure data is "[]", not null
			require.NotNil(t, resp.Data)
			assert.Equal(t, string(json.RawMessage("[]")), string(resp.Data))
			assert.Equal(t, "fulu", resp.Version)
		})
	})
}

func TestGetBlindedBlock(t *testing.T) {
	t.Run("phase0", func(t *testing.T) {
		b := util.NewBeaconBlock()
		sb, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)

		s := &Server{
			FinalizationFetcher: &chainMock.ChainService{},
			Blocker:             &testutil.MockBlocker{BlockToReturn: sb},
		}

		request := httptest.NewRequest(http.MethodGet, "http://foo.example/eth/v1/beacon/blinded_blocks/{block_id}", nil)
		request.SetPathValue("block_id", "head")
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetBlindedBlock(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetBlockV2Response{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, version.String(version.Phase0), resp.Version)
		sbb := &structs.SignedBeaconBlock{Message: &structs.BeaconBlock{}}
		require.NoError(t, json.Unmarshal(resp.Data.Message, sbb.Message))
		sbb.Signature = resp.Data.Signature
		genericBlk, err := sbb.ToGeneric()
		require.NoError(t, err)
		blk := genericBlk.GetPhase0()
		require.NoError(t, err)
		assert.DeepEqual(t, blk, b)
	})
	t.Run("altair", func(t *testing.T) {
		b := util.NewBeaconBlockAltair()
		sb, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)

		s := &Server{
			FinalizationFetcher: &chainMock.ChainService{},
			Blocker:             &testutil.MockBlocker{BlockToReturn: sb},
		}

		request := httptest.NewRequest(http.MethodGet, "http://foo.example/eth/v1/beacon/blinded_blocks/{block_id}", nil)
		request.SetPathValue("block_id", "head")
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetBlindedBlock(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetBlockV2Response{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, version.String(version.Altair), resp.Version)
		sbb := &structs.SignedBeaconBlockAltair{Message: &structs.BeaconBlockAltair{}}
		require.NoError(t, json.Unmarshal(resp.Data.Message, sbb.Message))
		sbb.Signature = resp.Data.Signature
		genericBlk, err := sbb.ToGeneric()
		require.NoError(t, err)
		blk := genericBlk.GetAltair()
		require.NoError(t, err)
		assert.DeepEqual(t, blk, b)
	})
	t.Run("bellatrix", func(t *testing.T) {
		b := util.NewBlindedBeaconBlockBellatrix()
		sb, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)

		mockChainService := &chainMock.ChainService{}
		s := &Server{
			OptimisticModeFetcher: mockChainService,
			FinalizationFetcher:   mockChainService,
			Blocker:               &testutil.MockBlocker{BlockToReturn: sb},
		}

		request := httptest.NewRequest(http.MethodGet, "http://foo.example/eth/v1/beacon/blinded_blocks/{block_id}", nil)
		request.SetPathValue("block_id", "head")
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetBlindedBlock(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetBlockV2Response{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, version.String(version.Bellatrix), resp.Version)
		sbb := &structs.SignedBlindedBeaconBlockBellatrix{Message: &structs.BlindedBeaconBlockBellatrix{}}
		require.NoError(t, json.Unmarshal(resp.Data.Message, sbb.Message))
		sbb.Signature = resp.Data.Signature
		genericBlk, err := sbb.ToGeneric()
		require.NoError(t, err)
		blk := genericBlk.GetBlindedBellatrix()
		require.NoError(t, err)
		assert.DeepEqual(t, blk, b)
	})
	t.Run("capella", func(t *testing.T) {
		b := util.NewBlindedBeaconBlockCapella()
		sb, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)

		mockChainService := &chainMock.ChainService{}
		s := &Server{
			OptimisticModeFetcher: mockChainService,
			FinalizationFetcher:   mockChainService,
			Blocker:               &testutil.MockBlocker{BlockToReturn: sb},
		}

		request := httptest.NewRequest(http.MethodGet, "http://foo.example/eth/v1/beacon/blinded_blocks/{block_id}", nil)
		request.SetPathValue("block_id", "head")
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetBlindedBlock(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetBlockV2Response{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, version.String(version.Capella), resp.Version)
		sbb := &structs.SignedBlindedBeaconBlockCapella{Message: &structs.BlindedBeaconBlockCapella{}}
		require.NoError(t, json.Unmarshal(resp.Data.Message, sbb.Message))
		sbb.Signature = resp.Data.Signature
		genericBlk, err := sbb.ToGeneric()
		require.NoError(t, err)
		blk := genericBlk.GetBlindedCapella()
		require.NoError(t, err)
		assert.DeepEqual(t, blk, b)
	})
	t.Run("deneb", func(t *testing.T) {
		b := util.NewBlindedBeaconBlockDeneb()
		sb, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)

		mockChainService := &chainMock.ChainService{}
		s := &Server{
			OptimisticModeFetcher: mockChainService,
			FinalizationFetcher:   mockChainService,
			Blocker:               &testutil.MockBlocker{BlockToReturn: sb},
		}

		request := httptest.NewRequest(http.MethodGet, "http://foo.example/eth/v1/beacon/blinded_blocks/{block_id}", nil)
		request.SetPathValue("block_id", "head")
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetBlindedBlock(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetBlockV2Response{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, version.String(version.Deneb), resp.Version)
		sbb := &structs.SignedBlindedBeaconBlockDeneb{Message: &structs.BlindedBeaconBlockDeneb{}}
		require.NoError(t, json.Unmarshal(resp.Data.Message, sbb.Message))
		sbb.Signature = resp.Data.Signature
		blk, err := sbb.ToConsensus()
		require.NoError(t, err)
		assert.DeepEqual(t, blk, b)
	})
	t.Run("electra", func(t *testing.T) {
		b := util.NewBlindedBeaconBlockElectra()
		sb, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)

		mockChainService := &chainMock.ChainService{}
		s := &Server{
			OptimisticModeFetcher: mockChainService,
			FinalizationFetcher:   mockChainService,
			Blocker:               &testutil.MockBlocker{BlockToReturn: sb},
		}

		request := httptest.NewRequest(http.MethodGet, "http://foo.example/eth/v1/beacon/blinded_blocks/{block_id}", nil)
		request.SetPathValue("block_id", "head")
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetBlindedBlock(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetBlockV2Response{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, version.String(version.Electra), resp.Version)
		sbb := &structs.SignedBlindedBeaconBlockElectra{Message: &structs.BlindedBeaconBlockElectra{}}
		require.NoError(t, json.Unmarshal(resp.Data.Message, sbb.Message))
		sbb.Signature = resp.Data.Signature
		blk, err := sbb.ToConsensus()
		require.NoError(t, err)
		assert.DeepEqual(t, blk, b)
	})
	t.Run("fulu", func(t *testing.T) {
		b := util.NewBlindedBeaconBlockFulu()
		sb, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)

		mockChainService := &chainMock.ChainService{}
		s := &Server{
			OptimisticModeFetcher: mockChainService,
			FinalizationFetcher:   mockChainService,
			Blocker:               &testutil.MockBlocker{BlockToReturn: sb},
		}

		request := httptest.NewRequest(http.MethodGet, "http://foo.example/eth/v1/beacon/blinded_blocks/{block_id}", nil)
		request.SetPathValue("block_id", "head")
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetBlindedBlock(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetBlockV2Response{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, version.String(version.Fulu), resp.Version)
		sbb := &structs.SignedBlindedBeaconBlockFulu{Message: &structs.BlindedBeaconBlockFulu{}}
		require.NoError(t, json.Unmarshal(resp.Data.Message, sbb.Message))
		sbb.Signature = resp.Data.Signature
		blk, err := sbb.ToConsensus()
		require.NoError(t, err)
		assert.DeepEqual(t, blk, b)
	})
	t.Run("execution optimistic", func(t *testing.T) {
		b := util.NewBlindedBeaconBlockBellatrix()
		sb, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		r, err := sb.Block().HashTreeRoot()
		require.NoError(t, err)

		mockChainService := &chainMock.ChainService{
			OptimisticRoots: map[[32]byte]bool{r: true},
		}
		s := &Server{
			FinalizationFetcher:   mockChainService,
			Blocker:               &testutil.MockBlocker{BlockToReturn: sb},
			OptimisticModeFetcher: mockChainService,
		}

		request := httptest.NewRequest(http.MethodGet, "http://foo.example/eth/v1/beacon/blinded_blocks/{block_id}", nil)
		request.SetPathValue("block_id", "head")
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetBlindedBlock(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetBlockV2Response{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, true, resp.ExecutionOptimistic)
	})
	t.Run("finalized", func(t *testing.T) {
		b := util.NewBeaconBlock()
		sb, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		root, err := sb.Block().HashTreeRoot()
		require.NoError(t, err)

		mockChainService := &chainMock.ChainService{
			FinalizedRoots: map[[32]byte]bool{root: true},
		}
		s := &Server{
			FinalizationFetcher: mockChainService,
			Blocker:             &testutil.MockBlocker{BlockToReturn: sb},
		}

		request := httptest.NewRequest(http.MethodGet, "http://foo.example/eth/v1/beacon/blinded_blocks/{block_id}", nil)
		request.SetPathValue("block_id", hexutil.Encode(root[:]))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetBlindedBlock(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetBlockV2Response{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, true, resp.Finalized)
	})
	t.Run("not finalized", func(t *testing.T) {
		b := util.NewBeaconBlock()
		sb, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		root, err := sb.Block().HashTreeRoot()
		require.NoError(t, err)

		mockChainService := &chainMock.ChainService{
			FinalizedRoots: map[[32]byte]bool{root: false},
		}
		s := &Server{
			FinalizationFetcher: mockChainService,
			Blocker:             &testutil.MockBlocker{BlockToReturn: sb},
		}

		request := httptest.NewRequest(http.MethodGet, "http://foo.example/eth/v1/beacon/blinded_blocks/{block_id}", nil)
		request.SetPathValue("block_id", hexutil.Encode(root[:]))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetBlindedBlock(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetBlockV2Response{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, false, resp.Finalized)
	})
}

func TestGetBlindedBlockSSZ(t *testing.T) {
	t.Run("phase0", func(t *testing.T) {
		b := util.NewBeaconBlock()
		sb, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)

		s := &Server{
			Blocker: &testutil.MockBlocker{BlockToReturn: sb},
		}

		request := httptest.NewRequest(http.MethodGet, "http://foo.example/eth/v1/beacon/blinded_blocks/{block_id}", nil)
		request.SetPathValue("block_id", "head")
		request.Header.Set("Accept", api.OctetStreamMediaType)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetBlindedBlock(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		assert.Equal(t, version.String(version.Phase0), writer.Header().Get(api.VersionHeader))
		sszExpected, err := b.MarshalSSZ()
		require.NoError(t, err)
		assert.DeepEqual(t, sszExpected, writer.Body.Bytes())
	})
	t.Run("Altair", func(t *testing.T) {
		b := util.NewBeaconBlockAltair()
		sb, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)

		s := &Server{
			Blocker: &testutil.MockBlocker{BlockToReturn: sb},
		}

		request := httptest.NewRequest(http.MethodGet, "http://foo.example/eth/v1/beacon/blinded_blocks/{block_id}", nil)
		request.SetPathValue("block_id", "head")
		request.Header.Set("Accept", api.OctetStreamMediaType)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetBlindedBlock(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		assert.Equal(t, version.String(version.Altair), writer.Header().Get(api.VersionHeader))
		sszExpected, err := b.MarshalSSZ()
		require.NoError(t, err)
		assert.DeepEqual(t, sszExpected, writer.Body.Bytes())
	})
	t.Run("Bellatrix", func(t *testing.T) {
		b := util.NewBlindedBeaconBlockBellatrix()
		sb, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)

		s := &Server{
			Blocker: &testutil.MockBlocker{BlockToReturn: sb},
		}

		request := httptest.NewRequest(http.MethodGet, "http://foo.example/eth/v1/beacon/blinded_blocks/{block_id}", nil)
		request.SetPathValue("block_id", "head")
		request.Header.Set("Accept", api.OctetStreamMediaType)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetBlindedBlock(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		assert.Equal(t, version.String(version.Bellatrix), writer.Header().Get(api.VersionHeader))
		sszExpected, err := b.MarshalSSZ()
		require.NoError(t, err)
		assert.DeepEqual(t, sszExpected, writer.Body.Bytes())
	})
	t.Run("Capella", func(t *testing.T) {
		b := util.NewBlindedBeaconBlockCapella()
		sb, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)

		s := &Server{
			Blocker: &testutil.MockBlocker{BlockToReturn: sb},
		}

		request := httptest.NewRequest(http.MethodGet, "http://foo.example/eth/v1/beacon/blinded_blocks/{block_id}", nil)
		request.SetPathValue("block_id", "head")
		request.Header.Set("Accept", api.OctetStreamMediaType)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetBlindedBlock(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		assert.Equal(t, version.String(version.Capella), writer.Header().Get(api.VersionHeader))
		sszExpected, err := b.MarshalSSZ()
		require.NoError(t, err)
		assert.DeepEqual(t, sszExpected, writer.Body.Bytes())
	})
	t.Run("Deneb", func(t *testing.T) {
		b := util.NewBlindedBeaconBlockDeneb()
		sb, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)

		s := &Server{
			Blocker: &testutil.MockBlocker{BlockToReturn: sb},
		}

		request := httptest.NewRequest(http.MethodGet, "http://foo.example/eth/v1/beacon/blinded_blocks/{block_id}", nil)
		request.SetPathValue("block_id", "head")
		request.Header.Set("Accept", api.OctetStreamMediaType)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetBlindedBlock(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		assert.Equal(t, version.String(version.Deneb), writer.Header().Get(api.VersionHeader))
		sszExpected, err := b.MarshalSSZ()
		require.NoError(t, err)
		assert.DeepEqual(t, sszExpected, writer.Body.Bytes())
	})
}

func TestPublishBlock(t *testing.T) {
	ctrl := gomock.NewController(t)
	t.Run("Phase 0", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			block, ok := req.Block.(*eth.GenericSignedBeaconBlock_Phase0)
			var signedblock *structs.SignedBeaconBlock
			err := json.Unmarshal([]byte(rpctesting.Phase0Block), &signedblock)
			require.NoError(t, err)
			require.DeepEqual(t, structs.BeaconBlockFromConsensus(block.Phase0.Block), signedblock.Message)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.Phase0Block)))
		request.Header.Set(api.VersionHeader, version.String(version.Phase0))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlock(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("Altair", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			block, ok := req.Block.(*eth.GenericSignedBeaconBlock_Altair)
			var signedblock *structs.SignedBeaconBlockAltair
			err := json.Unmarshal([]byte(rpctesting.AltairBlock), &signedblock)
			require.NoError(t, err)
			require.DeepEqual(t, structs.BeaconBlockAltairFromConsensus(block.Altair.Block), signedblock.Message)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.AltairBlock)))
		request.Header.Set(api.VersionHeader, version.String(version.Altair))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlock(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("Bellatrix", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			block, ok := req.Block.(*eth.GenericSignedBeaconBlock_Bellatrix)
			converted, err := structs.BeaconBlockBellatrixFromConsensus(block.Bellatrix.Block)
			require.NoError(t, err)
			var signedblock *structs.SignedBeaconBlockBellatrix
			err = json.Unmarshal([]byte(rpctesting.BellatrixBlock), &signedblock)
			require.NoError(t, err)
			require.DeepEqual(t, converted, signedblock.Message)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.BellatrixBlock)))
		request.Header.Set(api.VersionHeader, version.String(version.Bellatrix))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlock(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("Capella", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			block, ok := req.Block.(*eth.GenericSignedBeaconBlock_Capella)
			converted, err := structs.BeaconBlockCapellaFromConsensus(block.Capella.Block)
			require.NoError(t, err)
			var signedblock *structs.SignedBeaconBlockCapella
			err = json.Unmarshal([]byte(rpctesting.CapellaBlock), &signedblock)
			require.NoError(t, err)
			require.DeepEqual(t, converted, signedblock.Message)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.CapellaBlock)))
		request.Header.Set(api.VersionHeader, version.String(version.Capella))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlock(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("Capella block without version header succeeds", func(t *testing.T) {
		cfg := params.BeaconConfig().Copy()
		cfg.CapellaForkEpoch = 4
		params.OverrideBeaconConfig(cfg)
		params.SetupTestConfigCleanup(t)
		var signedblock *structs.SignedBeaconBlockCapella
		require.NoError(t, json.Unmarshal([]byte(rpctesting.CapellaBlock), &signedblock))
		signedblock.Message.Slot = fmt.Sprintf("%d", uint64(params.BeaconConfig().SlotsPerEpoch)*uint64(params.BeaconConfig().CapellaForkEpoch))
		newBlock, err := json.Marshal(signedblock)
		require.NoError(t, err)
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			block, ok := req.Block.(*eth.GenericSignedBeaconBlock_Capella)
			converted, err := structs.BeaconBlockCapellaFromConsensus(block.Capella.Block)
			require.NoError(t, err)
			var signedblock *structs.SignedBeaconBlockCapella
			err = json.Unmarshal(newBlock, &signedblock)
			require.NoError(t, err)
			require.DeepEqual(t, converted, signedblock.Message)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader(newBlock))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlock(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("Deneb", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			block, ok := req.Block.(*eth.GenericSignedBeaconBlock_Deneb)
			converted, err := structs.SignedBeaconBlockContentsDenebFromConsensus(block.Deneb)
			require.NoError(t, err)
			var signedblock *structs.SignedBeaconBlockContentsDeneb
			err = json.Unmarshal([]byte(rpctesting.DenebBlockContents), &signedblock)
			require.NoError(t, err)
			require.DeepEqual(t, converted, signedblock)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}
		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.DenebBlockContents)))
		request.Header.Set(api.VersionHeader, version.String(version.Deneb))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlock(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("Electra", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			block, ok := req.Block.(*eth.GenericSignedBeaconBlock_Electra)
			converted, err := structs.SignedBeaconBlockContentsElectraFromConsensus(block.Electra)
			require.NoError(t, err)
			var signedblock *structs.SignedBeaconBlockContentsElectra
			err = json.Unmarshal([]byte(rpctesting.FuluBlockContents), &signedblock)
			require.NoError(t, err)
			require.DeepEqual(t, converted, signedblock)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}
		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.ElectraBlockContents)))
		request.Header.Set(api.VersionHeader, version.String(version.Electra))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlock(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("Electra block without version header succeeds", func(t *testing.T) {
		cfg := params.BeaconConfig().Copy()
		cfg.ElectraForkEpoch = 6
		params.OverrideBeaconConfig(cfg)
		params.SetupTestConfigCleanup(t)
		var signedblock *structs.SignedBeaconBlockContentsElectra
		require.NoError(t, json.Unmarshal([]byte(rpctesting.ElectraBlockContents), &signedblock))
		signedblock.SignedBlock.Message.Slot = fmt.Sprintf("%d", uint64(params.BeaconConfig().SlotsPerEpoch)*uint64(params.BeaconConfig().ElectraForkEpoch))
		newContents, err := json.Marshal(signedblock)
		require.NoError(t, err)
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			block, ok := req.Block.(*eth.GenericSignedBeaconBlock_Electra)
			converted, err := structs.SignedBeaconBlockContentsElectraFromConsensus(block.Electra)
			require.NoError(t, err)
			var signedblock *structs.SignedBeaconBlockContentsElectra
			err = json.Unmarshal(newContents, &signedblock)
			require.NoError(t, err)
			require.DeepEqual(t, converted, signedblock)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader(newContents))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlock(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("Electra block without version header on wrong fork", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}
		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.ElectraBlockContents)))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlock(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		assert.StringContains(t, fmt.Sprintf("could not decode request body into %s consensus block", version.String(version.Phase0)), writer.Body.String())
	})
	t.Run("Fulu", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			block, ok := req.Block.(*eth.GenericSignedBeaconBlock_Fulu)
			converted, err := structs.SignedBeaconBlockContentsFuluFromConsensus(block.Fulu)
			require.NoError(t, err)
			var signedblock *structs.SignedBeaconBlockContentsFulu
			err = json.Unmarshal([]byte(rpctesting.FuluBlockContents), &signedblock)
			require.NoError(t, err)
			require.DeepEqual(t, converted, signedblock)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}
		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.FuluBlockContents)))
		request.Header.Set(api.VersionHeader, version.String(version.Fulu))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlock(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("invalid block", func(t *testing.T) {
		server := &Server{
			SyncChecker: &mockSync.Sync{IsSyncing: false},
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.BlindedBellatrixBlock)))
		request.Header.Set(api.VersionHeader, version.String(version.Bellatrix))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlock(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		assert.StringContains(t, fmt.Sprintf("could not decode request body into %s consensus block", version.String(version.Bellatrix)), writer.Body.String())
	})
	t.Run("wrong version header", func(t *testing.T) {
		server := &Server{
			SyncChecker: &mockSync.Sync{IsSyncing: false},
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.BellatrixBlock)))
		request.Header.Set(api.VersionHeader, version.String(version.Capella))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlock(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		assert.StringContains(t, fmt.Sprintf("could not decode request body into %s consensus block", version.String(version.Capella)), writer.Body.String())
	})
	t.Run("syncing", func(t *testing.T) {
		chainService := &chainMock.ChainService{}
		server := &Server{
			SyncChecker:           &mockSync.Sync{IsSyncing: true},
			HeadFetcher:           chainService,
			TimeFetcher:           chainService,
			OptimisticModeFetcher: chainService,
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte("foo")))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlock(writer, request)
		assert.Equal(t, http.StatusServiceUnavailable, writer.Code)
		assert.StringContains(t, "Beacon node is currently syncing and not serving request on that endpoint", writer.Body.String())
	})
}

func TestVersionHeaderFromRequest(t *testing.T) {
	t.Run("Fulu block contents returns fulu header", func(t *testing.T) {
		cfg := params.BeaconConfig().Copy()
		cfg.FuluForkEpoch = 7
		params.OverrideBeaconConfig(cfg)
		params.SetupTestConfigCleanup(t)
		var signedblock *structs.SignedBeaconBlockContentsFulu
		require.NoError(t, json.Unmarshal([]byte(rpctesting.FuluBlockContents), &signedblock))
		signedblock.SignedBlock.Message.Slot = fmt.Sprintf("%d", uint64(params.BeaconConfig().SlotsPerEpoch)*uint64(params.BeaconConfig().FuluForkEpoch))
		newContents, err := json.Marshal(signedblock)
		require.NoError(t, err)
		versionHead, err := versionHeaderFromRequest(newContents)
		require.NoError(t, err)
		require.Equal(t, version.String(version.Fulu), versionHead)
	})
	t.Run("Blinded Fulu block returns fulu header", func(t *testing.T) {
		cfg := params.BeaconConfig().Copy()
		cfg.FuluForkEpoch = 7
		params.OverrideBeaconConfig(cfg)
		params.SetupTestConfigCleanup(t)
		var signedblock *structs.SignedBlindedBeaconBlockFulu
		require.NoError(t, json.Unmarshal([]byte(rpctesting.BlindedFuluBlock), &signedblock))
		signedblock.Message.Slot = fmt.Sprintf("%d", uint64(params.BeaconConfig().SlotsPerEpoch)*uint64(params.BeaconConfig().FuluForkEpoch))
		newBlock, err := json.Marshal(signedblock)
		require.NoError(t, err)
		versionHead, err := versionHeaderFromRequest(newBlock)
		require.NoError(t, err)
		require.Equal(t, version.String(version.Fulu), versionHead)
	})
	t.Run("Electra block contents returns electra header", func(t *testing.T) {
		cfg := params.BeaconConfig().Copy()
		cfg.ElectraForkEpoch = 6
		params.OverrideBeaconConfig(cfg)
		params.SetupTestConfigCleanup(t)
		var signedblock *structs.SignedBeaconBlockContentsElectra
		require.NoError(t, json.Unmarshal([]byte(rpctesting.ElectraBlockContents), &signedblock))
		signedblock.SignedBlock.Message.Slot = fmt.Sprintf("%d", uint64(params.BeaconConfig().SlotsPerEpoch)*uint64(params.BeaconConfig().ElectraForkEpoch))
		newContents, err := json.Marshal(signedblock)
		require.NoError(t, err)
		versionHead, err := versionHeaderFromRequest(newContents)
		require.NoError(t, err)
		require.Equal(t, version.String(version.Electra), versionHead)
	})
	t.Run("Blinded Electra block returns electra header", func(t *testing.T) {
		cfg := params.BeaconConfig().Copy()
		cfg.ElectraForkEpoch = 6
		params.OverrideBeaconConfig(cfg)
		params.SetupTestConfigCleanup(t)
		var signedblock *structs.SignedBlindedBeaconBlockElectra
		require.NoError(t, json.Unmarshal([]byte(rpctesting.BlindedElectraBlock), &signedblock))
		signedblock.Message.Slot = fmt.Sprintf("%d", uint64(params.BeaconConfig().SlotsPerEpoch)*uint64(params.BeaconConfig().ElectraForkEpoch))
		newBlock, err := json.Marshal(signedblock)
		require.NoError(t, err)
		versionHead, err := versionHeaderFromRequest(newBlock)
		require.NoError(t, err)
		require.Equal(t, version.String(version.Electra), versionHead)
	})
	t.Run("Deneb block contents returns deneb header", func(t *testing.T) {
		cfg := params.BeaconConfig().Copy()
		cfg.DenebForkEpoch = 5
		params.OverrideBeaconConfig(cfg)
		params.SetupTestConfigCleanup(t)
		var signedblock *structs.SignedBeaconBlockContentsDeneb
		require.NoError(t, json.Unmarshal([]byte(rpctesting.DenebBlockContents), &signedblock))
		signedblock.SignedBlock.Message.Slot = fmt.Sprintf("%d", uint64(params.BeaconConfig().SlotsPerEpoch)*uint64(params.BeaconConfig().DenebForkEpoch))
		newContents, err := json.Marshal(signedblock)
		require.NoError(t, err)
		versionHead, err := versionHeaderFromRequest(newContents)
		require.NoError(t, err)
		require.Equal(t, version.String(version.Deneb), versionHead)
	})
	t.Run("Blinded Deneb block returns Deneb header", func(t *testing.T) {
		cfg := params.BeaconConfig().Copy()
		cfg.DenebForkEpoch = 5
		params.OverrideBeaconConfig(cfg)
		params.SetupTestConfigCleanup(t)
		var signedblock *structs.SignedBlindedBeaconBlockDeneb
		require.NoError(t, json.Unmarshal([]byte(rpctesting.BlindedDenebBlock), &signedblock))
		signedblock.Message.Slot = fmt.Sprintf("%d", uint64(params.BeaconConfig().SlotsPerEpoch)*uint64(params.BeaconConfig().DenebForkEpoch))
		newBlock, err := json.Marshal(signedblock)
		require.NoError(t, err)
		versionHead, err := versionHeaderFromRequest(newBlock)
		require.NoError(t, err)
		require.Equal(t, version.String(version.Deneb), versionHead)
	})
	t.Run("Capella block returns Capella header", func(t *testing.T) {
		cfg := params.BeaconConfig().Copy()
		cfg.CapellaForkEpoch = 4
		params.OverrideBeaconConfig(cfg)
		params.SetupTestConfigCleanup(t)
		var signedblock *structs.SignedBeaconBlockCapella
		require.NoError(t, json.Unmarshal([]byte(rpctesting.CapellaBlock), &signedblock))
		signedblock.Message.Slot = fmt.Sprintf("%d", uint64(params.BeaconConfig().SlotsPerEpoch)*uint64(params.BeaconConfig().CapellaForkEpoch))
		newBlock, err := json.Marshal(signedblock)
		require.NoError(t, err)
		versionHead, err := versionHeaderFromRequest(newBlock)
		require.NoError(t, err)
		require.Equal(t, version.String(version.Capella), versionHead)
	})
	t.Run("Blinded Capella block returns Capella header", func(t *testing.T) {
		cfg := params.BeaconConfig().Copy()
		cfg.CapellaForkEpoch = 4
		params.OverrideBeaconConfig(cfg)
		params.SetupTestConfigCleanup(t)
		var signedblock *structs.SignedBlindedBeaconBlockCapella
		require.NoError(t, json.Unmarshal([]byte(rpctesting.BlindedCapellaBlock), &signedblock))
		signedblock.Message.Slot = fmt.Sprintf("%d", uint64(params.BeaconConfig().SlotsPerEpoch)*uint64(params.BeaconConfig().CapellaForkEpoch))
		newBlock, err := json.Marshal(signedblock)
		require.NoError(t, err)
		versionHead, err := versionHeaderFromRequest(newBlock)
		require.NoError(t, err)
		require.Equal(t, version.String(version.Capella), versionHead)
	})
	t.Run("Bellatrix block returns capella header", func(t *testing.T) {
		cfg := params.BeaconConfig().Copy()
		cfg.BellatrixForkEpoch = 3
		params.OverrideBeaconConfig(cfg)
		params.SetupTestConfigCleanup(t)
		var signedblock *structs.SignedBeaconBlockBellatrix
		require.NoError(t, json.Unmarshal([]byte(rpctesting.BellatrixBlock), &signedblock))
		signedblock.Message.Slot = fmt.Sprintf("%d", uint64(params.BeaconConfig().SlotsPerEpoch)*uint64(params.BeaconConfig().BellatrixForkEpoch))
		newBlock, err := json.Marshal(signedblock)
		require.NoError(t, err)
		versionHead, err := versionHeaderFromRequest(newBlock)
		require.NoError(t, err)
		require.Equal(t, version.String(version.Bellatrix), versionHead)
	})
	t.Run("Blinded Capella block returns Capella header", func(t *testing.T) {
		cfg := params.BeaconConfig().Copy()
		cfg.BellatrixForkEpoch = 3
		params.OverrideBeaconConfig(cfg)
		params.SetupTestConfigCleanup(t)
		var signedblock *structs.SignedBlindedBeaconBlockBellatrix
		require.NoError(t, json.Unmarshal([]byte(rpctesting.BlindedBellatrixBlock), &signedblock))
		signedblock.Message.Slot = fmt.Sprintf("%d", uint64(params.BeaconConfig().SlotsPerEpoch)*uint64(params.BeaconConfig().BellatrixForkEpoch))
		newBlock, err := json.Marshal(signedblock)
		require.NoError(t, err)
		versionHead, err := versionHeaderFromRequest(newBlock)
		require.NoError(t, err)
		require.Equal(t, version.String(version.Bellatrix), versionHead)
	})
	t.Run("Altair block returns capella header", func(t *testing.T) {
		cfg := params.BeaconConfig().Copy()
		cfg.AltairForkEpoch = 2
		params.OverrideBeaconConfig(cfg)
		params.SetupTestConfigCleanup(t)
		var signedblock *structs.SignedBeaconBlockAltair
		require.NoError(t, json.Unmarshal([]byte(rpctesting.AltairBlock), &signedblock))
		signedblock.Message.Slot = fmt.Sprintf("%d", uint64(params.BeaconConfig().SlotsPerEpoch)*uint64(params.BeaconConfig().AltairForkEpoch))
		newBlock, err := json.Marshal(signedblock)
		require.NoError(t, err)
		versionHead, err := versionHeaderFromRequest(newBlock)
		require.NoError(t, err)
		require.Equal(t, version.String(version.Altair), versionHead)
	})
	t.Run("Phase0 block returns capella header", func(t *testing.T) {
		var signedblock *structs.SignedBeaconBlock
		require.NoError(t, json.Unmarshal([]byte(rpctesting.Phase0Block), &signedblock))
		newBlock, err := json.Marshal(signedblock)
		require.NoError(t, err)
		versionHead, err := versionHeaderFromRequest(newBlock)
		require.NoError(t, err)
		require.Equal(t, version.String(version.Phase0), versionHead)
	})
	t.Run("Malformed json returns error unable to peek slot from block contents", func(t *testing.T) {
		malformedJSON := []byte(`{"age": 30,}`)
		_, err := versionHeaderFromRequest(malformedJSON)
		require.ErrorContains(t, "unable to peek slot", err)
	})
}

func TestPublishBlockSSZ(t *testing.T) {
	ctrl := gomock.NewController(t)
	t.Run("Phase 0", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			block, ok := req.Block.(*eth.GenericSignedBeaconBlock_Phase0)
			var signedblock *structs.SignedBeaconBlock
			err := json.Unmarshal([]byte(rpctesting.Phase0Block), &signedblock)
			require.NoError(t, err)
			require.DeepEqual(t, structs.BeaconBlockFromConsensus(block.Phase0.Block), signedblock.Message)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		var blk structs.SignedBeaconBlock
		err := json.Unmarshal([]byte(rpctesting.Phase0Block), &blk)
		require.NoError(t, err)
		genericBlock, err := blk.ToGeneric()
		require.NoError(t, err)
		ssz, err := genericBlock.GetPhase0().MarshalSSZ()
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader(ssz))
		request.Header.Set("Content-Type", api.OctetStreamMediaType)
		request.Header.Set(api.VersionHeader, version.String(version.Phase0))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlock(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("Altair", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			block, ok := req.Block.(*eth.GenericSignedBeaconBlock_Altair)
			var signedblock *structs.SignedBeaconBlockAltair
			err := json.Unmarshal([]byte(rpctesting.AltairBlock), &signedblock)
			require.NoError(t, err)
			require.DeepEqual(t, structs.BeaconBlockAltairFromConsensus(block.Altair.Block), signedblock.Message)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		var blk structs.SignedBeaconBlockAltair
		err := json.Unmarshal([]byte(rpctesting.AltairBlock), &blk)
		require.NoError(t, err)
		genericBlock, err := blk.ToGeneric()
		require.NoError(t, err)
		ssz, err := genericBlock.GetAltair().MarshalSSZ()
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader(ssz))
		request.Header.Set("Content-Type", api.OctetStreamMediaType)
		request.Header.Set(api.VersionHeader, version.String(version.Altair))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlock(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("Bellatrix", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			_, ok := req.Block.(*eth.GenericSignedBeaconBlock_Bellatrix)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}
		var blk structs.SignedBeaconBlockBellatrix
		err := json.Unmarshal([]byte(rpctesting.BellatrixBlock), &blk)
		require.NoError(t, err)
		genericBlock, err := blk.ToGeneric()
		require.NoError(t, err)
		ssz, err := genericBlock.GetBellatrix().MarshalSSZ()
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader(ssz))
		request.Header.Set("Content-Type", api.OctetStreamMediaType)
		request.Header.Set(api.VersionHeader, version.String(version.Bellatrix))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlock(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("Capella", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			_, ok := req.Block.(*eth.GenericSignedBeaconBlock_Capella)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		var blk structs.SignedBeaconBlockCapella
		err := json.Unmarshal([]byte(rpctesting.CapellaBlock), &blk)
		require.NoError(t, err)
		genericBlock, err := blk.ToGeneric()
		require.NoError(t, err)
		ssz, err := genericBlock.GetCapella().MarshalSSZ()
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader(ssz))
		request.Header.Set("Content-Type", api.OctetStreamMediaType)
		request.Header.Set(api.VersionHeader, version.String(version.Capella))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlock(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("Deneb", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			_, ok := req.Block.(*eth.GenericSignedBeaconBlock_Deneb)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		var blk structs.SignedBeaconBlockContentsDeneb
		err := json.Unmarshal([]byte(rpctesting.DenebBlockContents), &blk)
		require.NoError(t, err)
		genericBlock, err := blk.ToGeneric()
		require.NoError(t, err)
		ssz, err := genericBlock.GetDeneb().MarshalSSZ()
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader(ssz))
		request.Header.Set("Content-Type", api.OctetStreamMediaType)
		request.Header.Set(api.VersionHeader, version.String(version.Deneb))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlock(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("Electra", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			_, ok := req.Block.(*eth.GenericSignedBeaconBlock_Electra)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		var blk structs.SignedBeaconBlockContentsElectra
		err := json.Unmarshal([]byte(rpctesting.FuluBlockContents), &blk)
		require.NoError(t, err)
		genericBlock, err := blk.ToGeneric()
		require.NoError(t, err)
		ssz, err := genericBlock.GetElectra().MarshalSSZ()
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader(ssz))
		request.Header.Set("Content-Type", api.OctetStreamMediaType)
		request.Header.Set(api.VersionHeader, version.String(version.Electra))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlock(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("Fulu", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			_, ok := req.Block.(*eth.GenericSignedBeaconBlock_Fulu)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		var blk structs.SignedBeaconBlockContentsFulu
		err := json.Unmarshal([]byte(rpctesting.FuluBlockContents), &blk)
		require.NoError(t, err)
		genericBlock, err := blk.ToGeneric()
		require.NoError(t, err)
		ssz, err := genericBlock.GetFulu().MarshalSSZ()
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader(ssz))
		request.Header.Set("Content-Type", api.OctetStreamMediaType)
		request.Header.Set(api.VersionHeader, version.String(version.Fulu))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlock(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("invalid block", func(t *testing.T) {
		server := &Server{
			SyncChecker: &mockSync.Sync{IsSyncing: false},
		}

		var blk structs.SignedBlindedBeaconBlockBellatrix
		err := json.Unmarshal([]byte(rpctesting.BlindedBellatrixBlock), &blk)
		require.NoError(t, err)
		genericBlock, err := blk.ToGeneric()
		require.NoError(t, err)
		ssz, err := genericBlock.GetBlindedBellatrix().MarshalSSZ()
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader(ssz))
		request.Header.Set("Content-Type", api.OctetStreamMediaType)
		request.Header.Set(api.VersionHeader, version.String(version.Bellatrix))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlock(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		assert.StringContains(t, fmt.Sprintf("could not decode request body into %s consensus block", version.String(version.Bellatrix)), writer.Body.String())
	})
	t.Run("wrong version header", func(t *testing.T) {
		server := &Server{
			SyncChecker: &mockSync.Sync{IsSyncing: false},
		}

		var blk structs.SignedBeaconBlockBellatrix
		err := json.Unmarshal([]byte(rpctesting.BellatrixBlock), &blk)
		require.NoError(t, err)
		genericBlock, err := blk.ToGeneric()
		require.NoError(t, err)
		ssz, err := genericBlock.GetBellatrix().MarshalSSZ()
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader(ssz))
		request.Header.Set("Content-Type", api.OctetStreamMediaType)
		request.Header.Set(api.VersionHeader, version.String(version.Capella))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlock(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		assert.StringContains(t, fmt.Sprintf("could not decode request body into %s consensus block", version.String(version.Capella)), writer.Body.String())
	})
	t.Run("syncing", func(t *testing.T) {
		chainService := &chainMock.ChainService{}
		server := &Server{
			SyncChecker:           &mockSync.Sync{IsSyncing: true},
			HeadFetcher:           chainService,
			TimeFetcher:           chainService,
			OptimisticModeFetcher: chainService,
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte("foo")))
		request.Header.Set("Content-Type", api.OctetStreamMediaType)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlock(writer, request)
		assert.Equal(t, http.StatusServiceUnavailable, writer.Code)
		assert.StringContains(t, "Beacon node is currently syncing and not serving request on that endpoint", writer.Body.String())
	})
}

func TestPublishBlindedBlock(t *testing.T) {
	ctrl := gomock.NewController(t)
	t.Run("Phase 0", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			block, ok := req.Block.(*eth.GenericSignedBeaconBlock_Phase0)
			var signedblock *structs.SignedBeaconBlock
			err := json.Unmarshal([]byte(rpctesting.Phase0Block), &signedblock)
			require.NoError(t, err)
			require.DeepEqual(t, structs.BeaconBlockFromConsensus(block.Phase0.Block), signedblock.Message)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.Phase0Block)))
		request.Header.Set(api.VersionHeader, version.String(version.Phase0))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlindedBlock(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("Altair", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			block, ok := req.Block.(*eth.GenericSignedBeaconBlock_Altair)
			var signedblock *structs.SignedBeaconBlockAltair
			err := json.Unmarshal([]byte(rpctesting.AltairBlock), &signedblock)
			require.NoError(t, err)
			require.DeepEqual(t, structs.BeaconBlockAltairFromConsensus(block.Altair.Block), signedblock.Message)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.AltairBlock)))
		request.Header.Set(api.VersionHeader, version.String(version.Altair))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlindedBlock(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("Blinded Bellatrix", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			block, ok := req.Block.(*eth.GenericSignedBeaconBlock_BlindedBellatrix)
			converted, err := structs.BlindedBeaconBlockBellatrixFromConsensus(block.BlindedBellatrix.Block)
			require.NoError(t, err)
			var signedblock *structs.SignedBlindedBeaconBlockBellatrix
			err = json.Unmarshal([]byte(rpctesting.BlindedBellatrixBlock), &signedblock)
			require.NoError(t, err)
			require.DeepEqual(t, converted, signedblock.Message)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.BlindedBellatrixBlock)))
		request.Header.Set(api.VersionHeader, version.String(version.Bellatrix))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlindedBlock(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("Blinded Capella", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			block, ok := req.Block.(*eth.GenericSignedBeaconBlock_BlindedCapella)
			converted, err := structs.BlindedBeaconBlockCapellaFromConsensus(block.BlindedCapella.Block)
			require.NoError(t, err)
			var signedblock *structs.SignedBlindedBeaconBlockCapella
			err = json.Unmarshal([]byte(rpctesting.BlindedCapellaBlock), &signedblock)
			require.NoError(t, err)
			require.DeepEqual(t, converted, signedblock.Message)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.BlindedCapellaBlock)))
		request.Header.Set(api.VersionHeader, version.String(version.Capella))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlindedBlock(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("Blinded Deneb", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			block, ok := req.Block.(*eth.GenericSignedBeaconBlock_BlindedDeneb)
			converted, err := structs.BlindedBeaconBlockDenebFromConsensus(block.BlindedDeneb.Message)
			require.NoError(t, err)
			var signedblock *structs.SignedBlindedBeaconBlockDeneb
			err = json.Unmarshal([]byte(rpctesting.BlindedDenebBlock), &signedblock)
			require.NoError(t, err)
			require.DeepEqual(t, converted, signedblock.Message)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.BlindedDenebBlock)))
		request.Header.Set(api.VersionHeader, version.String(version.Deneb))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlindedBlock(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("Blinded Electra", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			block, ok := req.Block.(*eth.GenericSignedBeaconBlock_BlindedElectra)
			converted, err := structs.BlindedBeaconBlockElectraFromConsensus(block.BlindedElectra.Message)
			require.NoError(t, err)
			var signedblock *structs.SignedBlindedBeaconBlockElectra
			err = json.Unmarshal([]byte(rpctesting.BlindedElectraBlock), &signedblock)
			require.NoError(t, err)
			require.DeepEqual(t, converted, signedblock.Message)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.BlindedElectraBlock)))
		request.Header.Set(api.VersionHeader, version.String(version.Electra))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlindedBlock(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("Blinded Electra block without version header succeeds", func(t *testing.T) {
		cfg := params.BeaconConfig().Copy()
		cfg.ElectraForkEpoch = 6
		params.OverrideBeaconConfig(cfg)
		params.SetupTestConfigCleanup(t)
		var signedblock *structs.SignedBlindedBeaconBlockElectra
		require.NoError(t, json.Unmarshal([]byte(rpctesting.BlindedElectraBlock), &signedblock))
		signedblock.Message.Slot = fmt.Sprintf("%d", uint64(params.BeaconConfig().SlotsPerEpoch)*uint64(params.BeaconConfig().ElectraForkEpoch))
		newBlock, err := json.Marshal(signedblock)
		require.NoError(t, err)
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			block, ok := req.Block.(*eth.GenericSignedBeaconBlock_BlindedElectra)
			converted, err := structs.BlindedBeaconBlockElectraFromConsensus(block.BlindedElectra.Message)
			require.NoError(t, err)
			var signedblock *structs.SignedBlindedBeaconBlockElectra
			err = json.Unmarshal(newBlock, &signedblock)
			require.NoError(t, err)
			require.DeepEqual(t, converted, signedblock.Message)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader(newBlock))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlindedBlock(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("Blinded Electra block without version header on wrong fork", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}
		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.BlindedElectraBlock)))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlindedBlock(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		// block is sent with slot == 1 which means it's in the phase0 fork
		assert.StringContains(t, fmt.Sprintf("could not decode request body into %s consensus block", version.String(version.Phase0)), writer.Body.String())
	})
	t.Run("Blinded Fulu", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			block, ok := req.Block.(*eth.GenericSignedBeaconBlock_BlindedFulu)
			converted, err := structs.BlindedBeaconBlockFuluFromConsensus(block.BlindedFulu.Message)
			require.NoError(t, err)
			var signedblock *structs.SignedBlindedBeaconBlockFulu
			err = json.Unmarshal([]byte(rpctesting.BlindedFuluBlock), &signedblock)
			require.NoError(t, err)
			require.DeepEqual(t, converted, signedblock.Message)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.BlindedFuluBlock)))
		request.Header.Set(api.VersionHeader, version.String(version.Fulu))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlindedBlock(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("invalid block", func(t *testing.T) {
		server := &Server{
			SyncChecker: &mockSync.Sync{IsSyncing: false},
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.BellatrixBlock)))
		request.Header.Set(api.VersionHeader, version.String(version.Bellatrix))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlindedBlock(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		assert.StringContains(t, fmt.Sprintf("could not decode request body into %s consensus block", version.String(version.Bellatrix)), writer.Body.String())
	})
	t.Run("wrong version header", func(t *testing.T) {
		server := &Server{
			SyncChecker: &mockSync.Sync{IsSyncing: false},
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.BlindedBellatrixBlock)))
		request.Header.Set(api.VersionHeader, version.String(version.Capella))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlindedBlock(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		assert.StringContains(t, fmt.Sprintf("could not decode request body into %s consensus block", version.String(version.Capella)), writer.Body.String())
	})
	t.Run("syncing", func(t *testing.T) {
		chainService := &chainMock.ChainService{}
		server := &Server{
			SyncChecker:           &mockSync.Sync{IsSyncing: true},
			HeadFetcher:           chainService,
			TimeFetcher:           chainService,
			OptimisticModeFetcher: chainService,
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte("foo")))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlindedBlock(writer, request)
		assert.Equal(t, http.StatusServiceUnavailable, writer.Code)
		assert.StringContains(t, "Beacon node is currently syncing and not serving request on that endpoint", writer.Body.String())
	})
}

func TestPublishBlindedBlockSSZ(t *testing.T) {
	ctrl := gomock.NewController(t)
	t.Run("Phase 0", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			block, ok := req.Block.(*eth.GenericSignedBeaconBlock_Phase0)
			var signedblock *structs.SignedBeaconBlock
			err := json.Unmarshal([]byte(rpctesting.Phase0Block), &signedblock)
			require.NoError(t, err)
			require.DeepEqual(t, structs.BeaconBlockFromConsensus(block.Phase0.Block), signedblock.Message)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		var blk structs.SignedBeaconBlock
		err := json.Unmarshal([]byte(rpctesting.Phase0Block), &blk)
		require.NoError(t, err)
		genericBlock, err := blk.ToGeneric()
		require.NoError(t, err)
		ssz, err := genericBlock.GetPhase0().MarshalSSZ()
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader(ssz))
		request.Header.Set("Content-Type", api.OctetStreamMediaType)
		request.Header.Set(api.VersionHeader, version.String(version.Phase0))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlindedBlock(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("Altair", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			block, ok := req.Block.(*eth.GenericSignedBeaconBlock_Altair)
			var signedblock *structs.SignedBeaconBlockAltair
			err := json.Unmarshal([]byte(rpctesting.AltairBlock), &signedblock)
			require.NoError(t, err)
			require.DeepEqual(t, structs.BeaconBlockAltairFromConsensus(block.Altair.Block), signedblock.Message)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		var blk structs.SignedBeaconBlockAltair
		err := json.Unmarshal([]byte(rpctesting.AltairBlock), &blk)
		require.NoError(t, err)
		genericBlock, err := blk.ToGeneric()
		require.NoError(t, err)
		ssz, err := genericBlock.GetAltair().MarshalSSZ()
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader(ssz))
		request.Header.Set("Content-Type", api.OctetStreamMediaType)
		request.Header.Set(api.VersionHeader, version.String(version.Altair))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlindedBlock(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("Bellatrix", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			_, ok := req.Block.(*eth.GenericSignedBeaconBlock_BlindedBellatrix)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		var blk structs.SignedBlindedBeaconBlockBellatrix
		err := json.Unmarshal([]byte(rpctesting.BlindedBellatrixBlock), &blk)
		require.NoError(t, err)
		genericBlock, err := blk.ToGeneric()
		require.NoError(t, err)
		ssz, err := genericBlock.GetBlindedBellatrix().MarshalSSZ()
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader(ssz))
		request.Header.Set("Content-Type", api.OctetStreamMediaType)
		request.Header.Set(api.VersionHeader, version.String(version.Bellatrix))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlindedBlock(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("Capella", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			_, ok := req.Block.(*eth.GenericSignedBeaconBlock_BlindedCapella)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		var blk structs.SignedBlindedBeaconBlockCapella
		err := json.Unmarshal([]byte(rpctesting.BlindedCapellaBlock), &blk)
		require.NoError(t, err)
		genericBlock, err := blk.ToGeneric()
		require.NoError(t, err)
		ssz, err := genericBlock.GetBlindedCapella().MarshalSSZ()
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader(ssz))
		request.Header.Set("Content-Type", api.OctetStreamMediaType)
		request.Header.Set(api.VersionHeader, version.String(version.Capella))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlindedBlock(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("Deneb", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			_, ok := req.Block.(*eth.GenericSignedBeaconBlock_BlindedDeneb)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		var blk structs.SignedBlindedBeaconBlockDeneb
		err := json.Unmarshal([]byte(rpctesting.BlindedDenebBlock), &blk)
		require.NoError(t, err)
		genericBlock, err := blk.ToGeneric()
		require.NoError(t, err)
		ssz, err := genericBlock.GetBlindedDeneb().MarshalSSZ()
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader(ssz))
		request.Header.Set("Content-Type", api.OctetStreamMediaType)
		request.Header.Set(api.VersionHeader, version.String(version.Deneb))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlindedBlock(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("Electra", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			_, ok := req.Block.(*eth.GenericSignedBeaconBlock_BlindedElectra)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		var blk structs.SignedBlindedBeaconBlockElectra
		err := json.Unmarshal([]byte(rpctesting.BlindedElectraBlock), &blk)
		require.NoError(t, err)
		genericBlock, err := blk.ToGeneric()
		require.NoError(t, err)
		ssz, err := genericBlock.GetBlindedElectra().MarshalSSZ()
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader(ssz))
		request.Header.Set("Content-Type", api.OctetStreamMediaType)
		request.Header.Set(api.VersionHeader, version.String(version.Electra))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlindedBlock(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("Fulu", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			_, ok := req.Block.(*eth.GenericSignedBeaconBlock_BlindedFulu)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		var blk structs.SignedBlindedBeaconBlockFulu
		err := json.Unmarshal([]byte(rpctesting.BlindedFuluBlock), &blk)
		require.NoError(t, err)
		genericBlock, err := blk.ToGeneric()
		require.NoError(t, err)
		ssz, err := genericBlock.GetBlindedFulu().MarshalSSZ()
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader(ssz))
		request.Header.Set("Content-Type", api.OctetStreamMediaType)
		request.Header.Set(api.VersionHeader, version.String(version.Fulu))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlindedBlock(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("invalid block", func(t *testing.T) {
		server := &Server{
			SyncChecker: &mockSync.Sync{IsSyncing: false},
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.BellatrixBlock)))
		request.Header.Set(api.VersionHeader, version.String(version.Bellatrix))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlindedBlock(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		assert.StringContains(t, fmt.Sprintf("could not decode request body into %s consensus block", version.String(version.Bellatrix)), writer.Body.String())
	})
	t.Run("wrong version header", func(t *testing.T) {
		server := &Server{
			SyncChecker: &mockSync.Sync{IsSyncing: false},
		}

		var blk structs.SignedBlindedBeaconBlockBellatrix
		err := json.Unmarshal([]byte(rpctesting.BlindedBellatrixBlock), &blk)
		require.NoError(t, err)
		genericBlock, err := blk.ToGeneric()
		require.NoError(t, err)
		ssz, err := genericBlock.GetBlindedBellatrix().MarshalSSZ()
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader(ssz))
		request.Header.Set("Content-Type", api.OctetStreamMediaType)
		request.Header.Set(api.VersionHeader, version.String(version.Capella))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlindedBlock(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		assert.StringContains(t, fmt.Sprintf("could not decode request body into %s consensus block", version.String(version.Capella)), writer.Body.String())
	})
	t.Run("syncing", func(t *testing.T) {
		chainService := &chainMock.ChainService{}
		server := &Server{
			SyncChecker:           &mockSync.Sync{IsSyncing: true},
			HeadFetcher:           chainService,
			TimeFetcher:           chainService,
			OptimisticModeFetcher: chainService,
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte("foo")))
		request.Header.Set("Content-Type", api.OctetStreamMediaType)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlindedBlock(writer, request)
		assert.Equal(t, http.StatusServiceUnavailable, writer.Code)
		assert.StringContains(t, "Beacon node is currently syncing and not serving request on that endpoint", writer.Body.String())
	})
}

func TestPublishBlockV2(t *testing.T) {
	ctrl := gomock.NewController(t)
	t.Run("Phase 0", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			block, ok := req.Block.(*eth.GenericSignedBeaconBlock_Phase0)
			var signedblock *structs.SignedBeaconBlock
			err := json.Unmarshal([]byte(rpctesting.Phase0Block), &signedblock)
			require.NoError(t, err)
			require.DeepEqual(t, structs.BeaconBlockFromConsensus(block.Phase0.Block), signedblock.Message)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.Phase0Block)))
		request.Header.Set(api.VersionHeader, version.String(version.Phase0))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlockV2(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("Altair", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			block, ok := req.Block.(*eth.GenericSignedBeaconBlock_Altair)
			var signedblock *structs.SignedBeaconBlockAltair
			err := json.Unmarshal([]byte(rpctesting.AltairBlock), &signedblock)
			require.NoError(t, err)
			require.DeepEqual(t, structs.BeaconBlockAltairFromConsensus(block.Altair.Block), signedblock.Message)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.AltairBlock)))
		request.Header.Set(api.VersionHeader, version.String(version.Altair))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlockV2(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("Bellatrix", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			block, ok := req.Block.(*eth.GenericSignedBeaconBlock_Bellatrix)
			converted, err := structs.BeaconBlockBellatrixFromConsensus(block.Bellatrix.Block)
			require.NoError(t, err)
			var signedblock *structs.SignedBeaconBlockBellatrix
			err = json.Unmarshal([]byte(rpctesting.BellatrixBlock), &signedblock)
			require.NoError(t, err)
			require.DeepEqual(t, converted, signedblock.Message)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.BellatrixBlock)))
		request.Header.Set(api.VersionHeader, version.String(version.Bellatrix))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlockV2(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("Capella", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			block, ok := req.Block.(*eth.GenericSignedBeaconBlock_Capella)
			converted, err := structs.BeaconBlockCapellaFromConsensus(block.Capella.Block)
			require.NoError(t, err)
			var signedblock *structs.SignedBeaconBlockCapella
			err = json.Unmarshal([]byte(rpctesting.CapellaBlock), &signedblock)
			require.NoError(t, err)
			require.DeepEqual(t, converted, signedblock.Message)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.CapellaBlock)))
		request.Header.Set(api.VersionHeader, version.String(version.Capella))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlockV2(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("Deneb", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			block, ok := req.Block.(*eth.GenericSignedBeaconBlock_Deneb)
			converted, err := structs.SignedBeaconBlockContentsDenebFromConsensus(block.Deneb)
			require.NoError(t, err)
			var signedblock *structs.SignedBeaconBlockContentsDeneb
			err = json.Unmarshal([]byte(rpctesting.DenebBlockContents), &signedblock)
			require.NoError(t, err)
			require.DeepEqual(t, converted, signedblock)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.DenebBlockContents)))
		request.Header.Set(api.VersionHeader, version.String(version.Deneb))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlockV2(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("Electra", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			block, ok := req.Block.(*eth.GenericSignedBeaconBlock_Electra)
			converted, err := structs.SignedBeaconBlockContentsElectraFromConsensus(block.Electra)
			require.NoError(t, err)
			var signedblock *structs.SignedBeaconBlockContentsElectra
			err = json.Unmarshal([]byte(rpctesting.ElectraBlockContents), &signedblock)
			require.NoError(t, err)
			require.DeepEqual(t, converted, signedblock)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.ElectraBlockContents)))
		request.Header.Set(api.VersionHeader, version.String(version.Electra))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlockV2(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("Fulu", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			block, ok := req.Block.(*eth.GenericSignedBeaconBlock_Fulu)
			converted, err := structs.SignedBeaconBlockContentsFuluFromConsensus(block.Fulu)
			require.NoError(t, err)
			var signedblock *structs.SignedBeaconBlockContentsFulu
			err = json.Unmarshal([]byte(rpctesting.FuluBlockContents), &signedblock)
			require.NoError(t, err)
			require.DeepEqual(t, converted, signedblock)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.FuluBlockContents)))
		request.Header.Set(api.VersionHeader, version.String(version.Fulu))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlockV2(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("invalid block", func(t *testing.T) {
		server := &Server{
			SyncChecker: &mockSync.Sync{IsSyncing: false},
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.BlindedBellatrixBlock)))
		request.Header.Set(api.VersionHeader, version.String(version.Bellatrix))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlockV2(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		assert.StringContains(t, fmt.Sprintf("could not decode request body into %s consensus block:", version.String(version.Bellatrix)), writer.Body.String())
	})
	t.Run("wrong version header", func(t *testing.T) {
		server := &Server{
			SyncChecker: &mockSync.Sync{IsSyncing: false},
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.BellatrixBlock)))
		request.Header.Set(api.VersionHeader, version.String(version.Capella))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlockV2(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		assert.StringContains(t, fmt.Sprintf("could not decode request body into %s consensus block:", version.String(version.Capella)), writer.Body.String())
	})
	t.Run("missing version header", func(t *testing.T) {
		server := &Server{
			SyncChecker: &mockSync.Sync{IsSyncing: false},
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.CapellaBlock)))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlockV2(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		assert.StringContains(t, api.VersionHeader+" header is required", writer.Body.String())
	})
	t.Run("syncing", func(t *testing.T) {
		chainService := &chainMock.ChainService{}
		server := &Server{
			SyncChecker:           &mockSync.Sync{IsSyncing: true},
			HeadFetcher:           chainService,
			TimeFetcher:           chainService,
			OptimisticModeFetcher: chainService,
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte("foo")))
		request.Header.Set(api.VersionHeader, version.String(version.Capella))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlockV2(writer, request)
		assert.Equal(t, http.StatusServiceUnavailable, writer.Code)
		assert.StringContains(t, "Beacon node is currently syncing and not serving request on that endpoint", writer.Body.String())
	})
}

func TestPublishBlockV2SSZ(t *testing.T) {
	ctrl := gomock.NewController(t)
	t.Run("Phase 0", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			block, ok := req.Block.(*eth.GenericSignedBeaconBlock_Phase0)
			var signedblock *structs.SignedBeaconBlock
			err := json.Unmarshal([]byte(rpctesting.Phase0Block), &signedblock)
			require.NoError(t, err)
			require.DeepEqual(t, structs.BeaconBlockFromConsensus(block.Phase0.Block), signedblock.Message)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		var blk structs.SignedBeaconBlock
		err := json.Unmarshal([]byte(rpctesting.Phase0Block), &blk)
		require.NoError(t, err)
		genericBlock, err := blk.ToGeneric()
		require.NoError(t, err)
		ssz, err := genericBlock.GetPhase0().MarshalSSZ()
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader(ssz))
		request.Header.Set("Content-Type", api.OctetStreamMediaType)
		request.Header.Set(api.VersionHeader, version.String(version.Phase0))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlockV2(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("Altair", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			block, ok := req.Block.(*eth.GenericSignedBeaconBlock_Altair)
			var signedblock *structs.SignedBeaconBlockAltair
			err := json.Unmarshal([]byte(rpctesting.AltairBlock), &signedblock)
			require.NoError(t, err)
			require.DeepEqual(t, structs.BeaconBlockAltairFromConsensus(block.Altair.Block), signedblock.Message)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		var blk structs.SignedBeaconBlockAltair
		err := json.Unmarshal([]byte(rpctesting.AltairBlock), &blk)
		require.NoError(t, err)
		genericBlock, err := blk.ToGeneric()
		require.NoError(t, err)
		ssz, err := genericBlock.GetAltair().MarshalSSZ()
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader(ssz))
		request.Header.Set("Content-Type", api.OctetStreamMediaType)
		request.Header.Set(api.VersionHeader, version.String(version.Altair))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlockV2(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("Bellatrix", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			_, ok := req.Block.(*eth.GenericSignedBeaconBlock_Bellatrix)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}
		var blk structs.SignedBeaconBlockBellatrix
		err := json.Unmarshal([]byte(rpctesting.BellatrixBlock), &blk)
		require.NoError(t, err)
		genericBlock, err := blk.ToGeneric()
		require.NoError(t, err)
		ssz, err := genericBlock.GetBellatrix().MarshalSSZ()
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader(ssz))
		request.Header.Set("Content-Type", api.OctetStreamMediaType)
		request.Header.Set(api.VersionHeader, version.String(version.Bellatrix))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlockV2(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("Capella", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			_, ok := req.Block.(*eth.GenericSignedBeaconBlock_Capella)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		var blk structs.SignedBeaconBlockCapella
		err := json.Unmarshal([]byte(rpctesting.CapellaBlock), &blk)
		require.NoError(t, err)
		genericBlock, err := blk.ToGeneric()
		require.NoError(t, err)
		ssz, err := genericBlock.GetCapella().MarshalSSZ()
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader(ssz))
		request.Header.Set("Content-Type", api.OctetStreamMediaType)
		request.Header.Set(api.VersionHeader, version.String(version.Capella))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlockV2(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("Deneb", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			_, ok := req.Block.(*eth.GenericSignedBeaconBlock_Deneb)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		var blk structs.SignedBeaconBlockContentsDeneb
		err := json.Unmarshal([]byte(rpctesting.DenebBlockContents), &blk)
		require.NoError(t, err)
		genericBlock, err := blk.ToGeneric()
		require.NoError(t, err)
		ssz, err := genericBlock.GetDeneb().MarshalSSZ()
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader(ssz))
		request.Header.Set("Content-Type", api.OctetStreamMediaType)
		request.Header.Set(api.VersionHeader, version.String(version.Deneb))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlockV2(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("Electra", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			_, ok := req.Block.(*eth.GenericSignedBeaconBlock_Electra)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		var blk structs.SignedBeaconBlockContentsElectra
		err := json.Unmarshal([]byte(rpctesting.ElectraBlockContents), &blk)
		require.NoError(t, err)
		genericBlock, err := blk.ToGeneric()
		require.NoError(t, err)
		ssz, err := genericBlock.GetElectra().MarshalSSZ()
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader(ssz))
		request.Header.Set("Content-Type", api.OctetStreamMediaType)
		request.Header.Set(api.VersionHeader, version.String(version.Electra))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlockV2(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("Fulu", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			_, ok := req.Block.(*eth.GenericSignedBeaconBlock_Fulu)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		var blk structs.SignedBeaconBlockContentsFulu
		err := json.Unmarshal([]byte(rpctesting.FuluBlockContents), &blk)
		require.NoError(t, err)
		genericBlock, err := blk.ToGeneric()
		require.NoError(t, err)
		ssz, err := genericBlock.GetFulu().MarshalSSZ()
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader(ssz))
		request.Header.Set("Content-Type", api.OctetStreamMediaType)
		request.Header.Set(api.VersionHeader, version.String(version.Fulu))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlockV2(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("invalid block", func(t *testing.T) {
		server := &Server{
			SyncChecker: &mockSync.Sync{IsSyncing: false},
		}

		var blk structs.SignedBlindedBeaconBlockBellatrix
		err := json.Unmarshal([]byte(rpctesting.BlindedBellatrixBlock), &blk)
		require.NoError(t, err)
		genericBlock, err := blk.ToGeneric()
		require.NoError(t, err)
		ssz, err := genericBlock.GetBlindedBellatrix().MarshalSSZ()
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader(ssz))
		request.Header.Set("Content-Type", api.OctetStreamMediaType)
		request.Header.Set(api.VersionHeader, version.String(version.Bellatrix))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlockV2(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		assert.StringContains(t, fmt.Sprintf("could not decode request body into %s consensus block", version.String(version.Bellatrix)), writer.Body.String())
	})
	t.Run("wrong version header", func(t *testing.T) {
		server := &Server{
			SyncChecker: &mockSync.Sync{IsSyncing: false},
		}

		var blk structs.SignedBeaconBlockBellatrix
		err := json.Unmarshal([]byte(rpctesting.BellatrixBlock), &blk)
		require.NoError(t, err)
		genericBlock, err := blk.ToGeneric()
		require.NoError(t, err)
		ssz, err := genericBlock.GetBellatrix().MarshalSSZ()
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader(ssz))
		request.Header.Set("Content-Type", api.OctetStreamMediaType)
		request.Header.Set(api.VersionHeader, version.String(version.Capella))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlockV2(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		assert.StringContains(t, fmt.Sprintf("could not decode request body into %s consensus block", version.String(version.Capella)), writer.Body.String())
	})
	t.Run("missing version header", func(t *testing.T) {
		server := &Server{
			SyncChecker: &mockSync.Sync{IsSyncing: false},
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.CapellaBlock)))
		request.Header.Set("Content-Type", api.OctetStreamMediaType)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlockV2(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		assert.StringContains(t, api.VersionHeader+" header is required", writer.Body.String())
	})
	t.Run("syncing", func(t *testing.T) {
		chainService := &chainMock.ChainService{}
		server := &Server{
			SyncChecker:           &mockSync.Sync{IsSyncing: true},
			HeadFetcher:           chainService,
			TimeFetcher:           chainService,
			OptimisticModeFetcher: chainService,
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte("foo")))
		request.Header.Set("Content-Type", api.OctetStreamMediaType)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlockV2(writer, request)
		assert.Equal(t, http.StatusServiceUnavailable, writer.Code)
		assert.StringContains(t, "Beacon node is currently syncing and not serving request on that endpoint", writer.Body.String())
	})
}

func TestPublishBlindedBlockV2(t *testing.T) {
	ctrl := gomock.NewController(t)
	t.Run("Phase 0", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			block, ok := req.Block.(*eth.GenericSignedBeaconBlock_Phase0)
			var signedblock *structs.SignedBeaconBlock
			err := json.Unmarshal([]byte(rpctesting.Phase0Block), &signedblock)
			require.NoError(t, err)
			require.DeepEqual(t, structs.BeaconBlockFromConsensus(block.Phase0.Block), signedblock.Message)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.Phase0Block)))
		request.Header.Set(api.VersionHeader, version.String(version.Phase0))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlindedBlockV2(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("Altair", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			block, ok := req.Block.(*eth.GenericSignedBeaconBlock_Altair)
			var signedblock *structs.SignedBeaconBlockAltair
			err := json.Unmarshal([]byte(rpctesting.AltairBlock), &signedblock)
			require.NoError(t, err)
			require.DeepEqual(t, structs.BeaconBlockAltairFromConsensus(block.Altair.Block), signedblock.Message)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.AltairBlock)))
		request.Header.Set(api.VersionHeader, version.String(version.Altair))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlindedBlockV2(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("Blinded Bellatrix", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			block, ok := req.Block.(*eth.GenericSignedBeaconBlock_BlindedBellatrix)
			converted, err := structs.BlindedBeaconBlockBellatrixFromConsensus(block.BlindedBellatrix.Block)
			require.NoError(t, err)
			var signedblock *structs.SignedBlindedBeaconBlockBellatrix
			err = json.Unmarshal([]byte(rpctesting.BlindedBellatrixBlock), &signedblock)
			require.NoError(t, err)
			require.DeepEqual(t, converted, signedblock.Message)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.BlindedBellatrixBlock)))
		request.Header.Set(api.VersionHeader, version.String(version.Bellatrix))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlindedBlockV2(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("Blinded Capella", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			block, ok := req.Block.(*eth.GenericSignedBeaconBlock_BlindedCapella)
			converted, err := structs.BlindedBeaconBlockCapellaFromConsensus(block.BlindedCapella.Block)
			require.NoError(t, err)
			var signedblock *structs.SignedBlindedBeaconBlockCapella
			err = json.Unmarshal([]byte(rpctesting.BlindedCapellaBlock), &signedblock)
			require.NoError(t, err)
			require.DeepEqual(t, converted, signedblock.Message)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.BlindedCapellaBlock)))
		request.Header.Set(api.VersionHeader, version.String(version.Capella))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlindedBlockV2(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("Blinded Deneb", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			block, ok := req.Block.(*eth.GenericSignedBeaconBlock_BlindedDeneb)
			converted, err := structs.BlindedBeaconBlockDenebFromConsensus(block.BlindedDeneb.Message)
			require.NoError(t, err)
			var signedblock *structs.SignedBlindedBeaconBlockDeneb
			err = json.Unmarshal([]byte(rpctesting.BlindedDenebBlock), &signedblock)
			require.NoError(t, err)
			require.DeepEqual(t, converted, signedblock.Message)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.BlindedDenebBlock)))
		request.Header.Set(api.VersionHeader, version.String(version.Deneb))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlindedBlockV2(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("Blinded Electra", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			block, ok := req.Block.(*eth.GenericSignedBeaconBlock_BlindedElectra)
			converted, err := structs.BlindedBeaconBlockElectraFromConsensus(block.BlindedElectra.Message)
			require.NoError(t, err)
			var signedblock *structs.SignedBlindedBeaconBlockElectra
			err = json.Unmarshal([]byte(rpctesting.BlindedElectraBlock), &signedblock)
			require.NoError(t, err)
			require.DeepEqual(t, converted, signedblock.Message)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.BlindedElectraBlock)))
		request.Header.Set(api.VersionHeader, version.String(version.Electra))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlindedBlockV2(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("Blinded Fulu", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			block, ok := req.Block.(*eth.GenericSignedBeaconBlock_BlindedFulu)
			converted, err := structs.BlindedBeaconBlockFuluFromConsensus(block.BlindedFulu.Message)
			require.NoError(t, err)
			var signedblock *structs.SignedBlindedBeaconBlockFulu
			err = json.Unmarshal([]byte(rpctesting.BlindedFuluBlock), &signedblock)
			require.NoError(t, err)
			require.DeepEqual(t, converted, signedblock.Message)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.BlindedFuluBlock)))
		request.Header.Set(api.VersionHeader, version.String(version.Fulu))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlindedBlockV2(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("invalid block", func(t *testing.T) {
		server := &Server{
			SyncChecker: &mockSync.Sync{IsSyncing: false},
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.BellatrixBlock)))
		request.Header.Set(api.VersionHeader, version.String(version.Bellatrix))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlindedBlockV2(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		assert.StringContains(t, fmt.Sprintf("could not decode request body into %s consensus block:", version.String(version.Bellatrix)), writer.Body.String())
	})
	t.Run("wrong version header", func(t *testing.T) {
		server := &Server{
			SyncChecker: &mockSync.Sync{IsSyncing: false},
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.BlindedBellatrixBlock)))
		request.Header.Set(api.VersionHeader, version.String(version.Capella))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlindedBlockV2(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		assert.StringContains(t, fmt.Sprintf("could not decode request body into %s consensus block", version.String(version.Capella)), writer.Body.String())
	})
	t.Run("missing version header", func(t *testing.T) {
		server := &Server{
			SyncChecker: &mockSync.Sync{IsSyncing: false},
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.BlindedCapellaBlock)))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlockV2(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		assert.StringContains(t, api.VersionHeader+" header is required", writer.Body.String())
	})
	t.Run("syncing", func(t *testing.T) {
		chainService := &chainMock.ChainService{}
		server := &Server{
			SyncChecker:           &mockSync.Sync{IsSyncing: true},
			HeadFetcher:           chainService,
			TimeFetcher:           chainService,
			OptimisticModeFetcher: chainService,
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte("foo")))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlindedBlockV2(writer, request)
		assert.Equal(t, http.StatusServiceUnavailable, writer.Code)
		assert.Equal(t, true, strings.Contains(writer.Body.String(), "Beacon node is currently syncing and not serving request on that endpoint"))
	})
}

func TestPublishBlindedBlockV2SSZ(t *testing.T) {
	ctrl := gomock.NewController(t)
	t.Run("Phase 0", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			block, ok := req.Block.(*eth.GenericSignedBeaconBlock_Phase0)
			var signedblock *structs.SignedBeaconBlock
			err := json.Unmarshal([]byte(rpctesting.Phase0Block), &signedblock)
			require.NoError(t, err)
			require.DeepEqual(t, structs.BeaconBlockFromConsensus(block.Phase0.Block), signedblock.Message)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		var blk structs.SignedBeaconBlock
		err := json.Unmarshal([]byte(rpctesting.Phase0Block), &blk)
		require.NoError(t, err)
		genericBlock, err := blk.ToGeneric()
		require.NoError(t, err)
		ssz, err := genericBlock.GetPhase0().MarshalSSZ()
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader(ssz))
		request.Header.Set("Content-Type", api.OctetStreamMediaType)
		request.Header.Set(api.VersionHeader, version.String(version.Phase0))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlindedBlockV2(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("Altair", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			block, ok := req.Block.(*eth.GenericSignedBeaconBlock_Altair)
			var signedblock *structs.SignedBeaconBlockAltair
			err := json.Unmarshal([]byte(rpctesting.AltairBlock), &signedblock)
			require.NoError(t, err)
			require.DeepEqual(t, structs.BeaconBlockAltairFromConsensus(block.Altair.Block), signedblock.Message)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		var blk structs.SignedBeaconBlockAltair
		err := json.Unmarshal([]byte(rpctesting.AltairBlock), &blk)
		require.NoError(t, err)
		genericBlock, err := blk.ToGeneric()
		require.NoError(t, err)
		ssz, err := genericBlock.GetAltair().MarshalSSZ()
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader(ssz))
		request.Header.Set("Content-Type", api.OctetStreamMediaType)
		request.Header.Set(api.VersionHeader, version.String(version.Altair))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlindedBlockV2(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("Bellatrix", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			_, ok := req.Block.(*eth.GenericSignedBeaconBlock_BlindedBellatrix)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		var blk structs.SignedBlindedBeaconBlockBellatrix
		err := json.Unmarshal([]byte(rpctesting.BlindedBellatrixBlock), &blk)
		require.NoError(t, err)
		genericBlock, err := blk.ToGeneric()
		require.NoError(t, err)
		ssz, err := genericBlock.GetBlindedBellatrix().MarshalSSZ()
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader(ssz))
		request.Header.Set("Content-Type", api.OctetStreamMediaType)
		request.Header.Set(api.VersionHeader, version.String(version.Bellatrix))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlindedBlockV2(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("Capella", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			_, ok := req.Block.(*eth.GenericSignedBeaconBlock_BlindedCapella)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		var blk structs.SignedBlindedBeaconBlockCapella
		err := json.Unmarshal([]byte(rpctesting.BlindedCapellaBlock), &blk)
		require.NoError(t, err)
		genericBlock, err := blk.ToGeneric()
		require.NoError(t, err)
		ssz, err := genericBlock.GetBlindedCapella().MarshalSSZ()
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader(ssz))
		request.Header.Set("Content-Type", api.OctetStreamMediaType)
		request.Header.Set(api.VersionHeader, version.String(version.Capella))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlindedBlockV2(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("Deneb", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			_, ok := req.Block.(*eth.GenericSignedBeaconBlock_BlindedDeneb)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		var blk structs.SignedBlindedBeaconBlockDeneb
		err := json.Unmarshal([]byte(rpctesting.BlindedDenebBlock), &blk)
		require.NoError(t, err)
		genericBlock, err := blk.ToGeneric()
		require.NoError(t, err)
		ssz, err := genericBlock.GetBlindedDeneb().MarshalSSZ()
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader(ssz))
		request.Header.Set("Content-Type", api.OctetStreamMediaType)
		request.Header.Set(api.VersionHeader, version.String(version.Deneb))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlindedBlock(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("Electra", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			_, ok := req.Block.(*eth.GenericSignedBeaconBlock_BlindedElectra)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		var blk structs.SignedBlindedBeaconBlockElectra
		err := json.Unmarshal([]byte(rpctesting.BlindedElectraBlock), &blk)
		require.NoError(t, err)
		genericBlock, err := blk.ToGeneric()
		require.NoError(t, err)
		ssz, err := genericBlock.GetBlindedElectra().MarshalSSZ()
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader(ssz))
		request.Header.Set("Content-Type", api.OctetStreamMediaType)
		request.Header.Set(api.VersionHeader, version.String(version.Electra))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlindedBlock(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("Fulu", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *eth.GenericSignedBeaconBlock) bool {
			_, ok := req.Block.(*eth.GenericSignedBeaconBlock_BlindedFulu)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		var blk structs.SignedBlindedBeaconBlockFulu
		err := json.Unmarshal([]byte(rpctesting.BlindedFuluBlock), &blk)
		require.NoError(t, err)
		genericBlock, err := blk.ToGeneric()
		require.NoError(t, err)
		ssz, err := genericBlock.GetBlindedFulu().MarshalSSZ()
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader(ssz))
		request.Header.Set("Content-Type", api.OctetStreamMediaType)
		request.Header.Set(api.VersionHeader, version.String(version.Fulu))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlindedBlock(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("invalid block", func(t *testing.T) {
		server := &Server{
			SyncChecker: &mockSync.Sync{IsSyncing: false},
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.BellatrixBlock)))
		request.Header.Set(api.VersionHeader, version.String(version.Bellatrix))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlindedBlockV2(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		assert.StringContains(t, fmt.Sprintf("could not decode request body into %s consensus block", version.String(version.Bellatrix)), writer.Body.String())
	})
	t.Run("wrong version header", func(t *testing.T) {
		server := &Server{
			SyncChecker: &mockSync.Sync{IsSyncing: false},
		}

		var blk structs.SignedBlindedBeaconBlockBellatrix
		err := json.Unmarshal([]byte(rpctesting.BlindedBellatrixBlock), &blk)
		require.NoError(t, err)
		genericBlock, err := blk.ToGeneric()
		require.NoError(t, err)
		ssz, err := genericBlock.GetBlindedBellatrix().MarshalSSZ()
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader(ssz))
		request.Header.Set("Content-Type", api.OctetStreamMediaType)
		request.Header.Set(api.VersionHeader, version.String(version.Capella))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlindedBlockV2(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		assert.StringContains(t, fmt.Sprintf("could not decode request body into %s consensus block", version.String(version.Capella)), writer.Body.String())
	})
	t.Run("missing version header", func(t *testing.T) {
		server := &Server{
			SyncChecker: &mockSync.Sync{IsSyncing: false},
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.BlindedCapellaBlock)))
		request.Header.Set("Content-Type", api.OctetStreamMediaType)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlockV2(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		assert.StringContains(t, api.VersionHeader+" header is required", writer.Body.String())
	})
	t.Run("syncing", func(t *testing.T) {
		chainService := &chainMock.ChainService{}
		server := &Server{
			SyncChecker:           &mockSync.Sync{IsSyncing: true},
			HeadFetcher:           chainService,
			TimeFetcher:           chainService,
			OptimisticModeFetcher: chainService,
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte("foo")))
		request.Header.Set("Content-Type", api.OctetStreamMediaType)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlindedBlockV2(writer, request)
		assert.Equal(t, http.StatusServiceUnavailable, writer.Code)
		assert.StringContains(t, "Beacon node is currently syncing and not serving request on that endpoint", writer.Body.String())
	})
}

func TestValidateConsensus(t *testing.T) {
	ctx := t.Context()

	parentState, privs := util.DeterministicGenesisState(t, params.MinimalSpecConfig().MinGenesisActiveValidatorCount)
	parentBlock, err := util.GenerateFullBlock(parentState, privs, util.DefaultBlockGenConfig(), parentState.Slot())
	require.NoError(t, err)
	parentSbb, err := blocks.NewSignedBeaconBlock(parentBlock)
	require.NoError(t, err)
	st, err := transition.ExecuteStateTransition(ctx, parentState, parentSbb)
	require.NoError(t, err)
	block, err := util.GenerateFullBlock(st, privs, util.DefaultBlockGenConfig(), st.Slot())
	require.NoError(t, err)
	parentRoot, err := parentSbb.Block().HashTreeRoot()
	require.NoError(t, err)
	mockChainService := &chainMock.ChainService{
		State: parentState,
		Root:  parentRoot[:],
	}
	server := &Server{
		Blocker:     &testutil.MockBlocker{RootBlockMap: map[[32]byte]interfaces.ReadOnlySignedBeaconBlock{parentRoot: parentSbb}},
		Stater:      &testutil.MockStater{StatesByRoot: map[[32]byte]state.BeaconState{bytesutil.ToBytes32(parentBlock.Block.StateRoot): parentState}},
		HeadFetcher: mockChainService,
	}

	require.NoError(t, server.validateConsensus(ctx, &eth.GenericSignedBeaconBlock{
		Block: &eth.GenericSignedBeaconBlock_Phase0{
			Phase0: block,
		},
	}))
}

func TestValidateEquivocation(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		st, err := util.NewBeaconState()
		require.NoError(t, err)
		require.NoError(t, st.SetSlot(10))
		blk, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlock())
		require.NoError(t, err)
		roblock, err := blocks.NewROBlockWithRoot(blk, bytesutil.ToBytes32([]byte("root")))
		require.NoError(t, err)
		fc := doublylinkedtree.New()
		require.NoError(t, fc.InsertNode(t.Context(), st, roblock))
		server := &Server{
			ForkchoiceFetcher: &chainMock.ChainService{ForkChoiceStore: fc},
		}
		blk.SetSlot(st.Slot() + 1)

		require.NoError(t, server.validateEquivocation(blk.Block()))
	})
	t.Run("block already exists", func(t *testing.T) {
		st, err := util.NewBeaconState()
		require.NoError(t, err)
		require.NoError(t, st.SetSlot(10))
		blk, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlock())
		require.NoError(t, err)
		blk.SetSlot(st.Slot())
		roblock, err := blocks.NewROBlockWithRoot(blk, bytesutil.ToBytes32([]byte("root")))
		require.NoError(t, err)

		fc := doublylinkedtree.New()
		require.NoError(t, fc.InsertNode(t.Context(), st, roblock))
		server := &Server{
			ForkchoiceFetcher: &chainMock.ChainService{ForkChoiceStore: fc},
		}
		err = server.validateEquivocation(blk.Block())
		assert.ErrorContains(t, "already exists", err)
		require.ErrorIs(t, err, errEquivocatedBlock)
	})
}

func TestServer_GetBlockRoot(t *testing.T) {
	beaconDB := dbTest.SetupDB(t)
	ctx := t.Context()

	url := "http://example.com/eth/v1/beacon/blocks/{block_id}/root"
	genBlk, blkContainers := fillDBTestBlocks(ctx, t, beaconDB)
	headBlock := blkContainers[len(blkContainers)-1]
	t.Run("get root", func(t *testing.T) {
		wsb, err := blocks.NewSignedBeaconBlock(headBlock.Block.(*eth.BeaconBlockContainer_Phase0Block).Phase0Block)
		require.NoError(t, err)

		mockChainFetcher := &chainMock.ChainService{
			DB:                  beaconDB,
			Block:               wsb,
			Root:                headBlock.BlockRoot,
			FinalizedCheckPoint: &eth.Checkpoint{Root: blkContainers[64].BlockRoot},
			FinalizedRoots:      map[[32]byte]bool{},
		}

		bs := &Server{
			BeaconDB:              beaconDB,
			ChainInfoFetcher:      mockChainFetcher,
			HeadFetcher:           mockChainFetcher,
			OptimisticModeFetcher: mockChainFetcher,
			FinalizationFetcher:   mockChainFetcher,
		}

		root, err := genBlk.Block.HashTreeRoot()
		require.NoError(t, err)

		tests := []struct {
			name     string
			blockID  map[string]string
			want     string
			wantErr  string
			wantCode int
		}{
			{
				name:     "bad formatting",
				blockID:  map[string]string{"block_id": "3bad0"},
				wantErr:  "Could not parse block ID",
				wantCode: http.StatusBadRequest,
			},
			{
				name:     "canonical slot",
				blockID:  map[string]string{"block_id": "30"},
				want:     hexutil.Encode(blkContainers[30].BlockRoot),
				wantErr:  "",
				wantCode: http.StatusOK,
			},
			{
				name:     "head",
				blockID:  map[string]string{"block_id": "head"},
				want:     hexutil.Encode(headBlock.BlockRoot),
				wantErr:  "",
				wantCode: http.StatusOK,
			},
			{
				name:     "finalized",
				blockID:  map[string]string{"block_id": "finalized"},
				want:     hexutil.Encode(blkContainers[64].BlockRoot),
				wantErr:  "",
				wantCode: http.StatusOK,
			},
			{
				name:     "genesis",
				blockID:  map[string]string{"block_id": "genesis"},
				want:     hexutil.Encode(root[:]),
				wantErr:  "",
				wantCode: http.StatusOK,
			},
			{
				name:     "genesis root",
				blockID:  map[string]string{"block_id": hexutil.Encode(root[:])},
				want:     hexutil.Encode(root[:]),
				wantErr:  "",
				wantCode: http.StatusOK,
			},
			{
				name:     "root",
				blockID:  map[string]string{"block_id": hexutil.Encode(blkContainers[20].BlockRoot)},
				want:     hexutil.Encode(blkContainers[20].BlockRoot),
				wantErr:  "",
				wantCode: http.StatusOK,
			},
			{
				name:     "non-existent root",
				blockID:  map[string]string{"block_id": hexutil.Encode(bytesutil.PadTo([]byte("hi there"), 32))},
				wantErr:  "Could not find block",
				wantCode: http.StatusNotFound,
			},
			{
				name:     "slot",
				blockID:  map[string]string{"block_id": "40"},
				want:     hexutil.Encode(blkContainers[40].BlockRoot),
				wantErr:  "",
				wantCode: http.StatusOK,
			},
			{
				name:     "no block",
				blockID:  map[string]string{"block_id": "105"},
				wantErr:  "Could not find any blocks with given slot",
				wantCode: http.StatusNotFound,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				blockID := tt.blockID
				request := httptest.NewRequest(http.MethodGet, url, nil)
				request.SetPathValue("block_id", blockID["block_id"])
				writer := httptest.NewRecorder()

				writer.Body = &bytes.Buffer{}

				bs.GetBlockRoot(writer, request)
				assert.Equal(t, tt.wantCode, writer.Code)
				resp := &structs.BlockRootResponse{}
				require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
				if tt.wantErr != "" {
					require.ErrorContains(t, tt.wantErr, errors.New(writer.Body.String()))
					return
				}
				require.NotNil(t, resp)
				require.DeepEqual(t, resp.Data.Root, tt.want)
			})
		}
	})
	t.Run("execution optimistic", func(t *testing.T) {
		wsb, err := blocks.NewSignedBeaconBlock(headBlock.Block.(*eth.BeaconBlockContainer_Phase0Block).Phase0Block)
		require.NoError(t, err)

		mockChainFetcher := &chainMock.ChainService{
			DB:                  beaconDB,
			Block:               wsb,
			Root:                headBlock.BlockRoot,
			FinalizedCheckPoint: &eth.Checkpoint{Root: blkContainers[64].BlockRoot},
			Optimistic:          true,
			FinalizedRoots:      map[[32]byte]bool{},
			OptimisticRoots: map[[32]byte]bool{
				bytesutil.ToBytes32(headBlock.BlockRoot): true,
			},
		}

		bs := &Server{
			BeaconDB:              beaconDB,
			ChainInfoFetcher:      mockChainFetcher,
			HeadFetcher:           mockChainFetcher,
			OptimisticModeFetcher: mockChainFetcher,
			FinalizationFetcher:   mockChainFetcher,
		}

		request := httptest.NewRequest(http.MethodGet, url, nil)
		request.SetPathValue("block_id", "head")
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		bs.GetBlockRoot(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.BlockRootResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.DeepEqual(t, resp.ExecutionOptimistic, true)
	})
	t.Run("finalized", func(t *testing.T) {
		wsb, err := blocks.NewSignedBeaconBlock(headBlock.Block.(*eth.BeaconBlockContainer_Phase0Block).Phase0Block)
		require.NoError(t, err)

		mockChainFetcher := &chainMock.ChainService{
			DB:                  beaconDB,
			Block:               wsb,
			Root:                headBlock.BlockRoot,
			FinalizedCheckPoint: &eth.Checkpoint{Root: blkContainers[64].BlockRoot},
			Optimistic:          true,
			FinalizedRoots: map[[32]byte]bool{
				bytesutil.ToBytes32(blkContainers[32].BlockRoot): true,
				bytesutil.ToBytes32(blkContainers[64].BlockRoot): false,
			},
		}

		bs := &Server{
			BeaconDB:              beaconDB,
			ChainInfoFetcher:      mockChainFetcher,
			HeadFetcher:           mockChainFetcher,
			OptimisticModeFetcher: mockChainFetcher,
			FinalizationFetcher:   mockChainFetcher,
		}
		t.Run("true", func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, url, nil)
			request.SetPathValue("block_id", "32")
			writer := httptest.NewRecorder()
			writer.Body = &bytes.Buffer{}

			bs.GetBlockRoot(writer, request)
			assert.Equal(t, http.StatusOK, writer.Code)
			resp := &structs.BlockRootResponse{}
			require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
			require.DeepEqual(t, resp.Finalized, true)
		})
		t.Run("false", func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, url, nil)
			request.SetPathValue("block_id", "64")
			writer := httptest.NewRecorder()
			writer.Body = &bytes.Buffer{}

			bs.GetBlockRoot(writer, request)
			assert.Equal(t, http.StatusOK, writer.Code)
			resp := &structs.BlockRootResponse{}
			require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
			require.DeepEqual(t, resp.Finalized, false)
		})
	})
}

func TestGetStateFork(t *testing.T) {
	ctx := t.Context()
	request := httptest.NewRequest(http.MethodGet, "http://foo.example/eth/v1/beacon/states/{state_id}/fork", nil)
	request.SetPathValue("state_id", "head")
	request.Header.Set("Accept", "application/octet-stream")
	writer := httptest.NewRecorder()
	writer.Body = &bytes.Buffer{}

	fillFork := func(state *eth.BeaconState) error {
		state.Fork = &eth.Fork{
			PreviousVersion: []byte("prev"),
			CurrentVersion:  []byte("curr"),
			Epoch:           123,
		}
		return nil
	}
	fakeState, err := util.NewBeaconState(fillFork)
	require.NoError(t, err)
	db := dbTest.SetupDB(t)

	chainService := &chainMock.ChainService{}
	server := &Server{
		Stater: &testutil.MockStater{
			BeaconState: fakeState,
		},
		HeadFetcher:           chainService,
		OptimisticModeFetcher: chainService,
		FinalizationFetcher:   chainService,
		BeaconDB:              db,
	}

	server.GetStateFork(writer, request)
	require.Equal(t, http.StatusOK, writer.Code)
	var stateForkReponse *structs.GetStateForkResponse
	err = json.Unmarshal(writer.Body.Bytes(), &stateForkReponse)
	require.NoError(t, err)
	expectedFork := fakeState.Fork()
	assert.Equal(t, fmt.Sprint(expectedFork.Epoch), stateForkReponse.Data.Epoch)
	assert.DeepEqual(t, hexutil.Encode(expectedFork.CurrentVersion), stateForkReponse.Data.CurrentVersion)
	assert.DeepEqual(t, hexutil.Encode(expectedFork.PreviousVersion), stateForkReponse.Data.PreviousVersion)
	t.Run("execution optimistic", func(t *testing.T) {
		request = httptest.NewRequest(http.MethodGet, "http://foo.example/eth/v1/beacon/states/{state_id}/fork", nil)
		request.SetPathValue("state_id", "head")
		request.Header.Set("Accept", "application/octet-stream")
		writer = httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		parentRoot := [32]byte{'a'}
		blk := util.NewBeaconBlock()
		blk.Block.ParentRoot = parentRoot[:]
		root, err := blk.Block.HashTreeRoot()
		require.NoError(t, err)
		util.SaveBlock(t, ctx, db, blk)
		require.NoError(t, db.SaveGenesisBlockRoot(ctx, root))

		chainService = &chainMock.ChainService{Optimistic: true}
		server = &Server{
			Stater: &testutil.MockStater{
				BeaconState: fakeState,
			},
			HeadFetcher:           chainService,
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
			BeaconDB:              db,
		}
		server.GetStateFork(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		err = json.Unmarshal(writer.Body.Bytes(), &stateForkReponse)
		require.NoError(t, err)
		assert.DeepEqual(t, true, stateForkReponse.ExecutionOptimistic)
	})

	t.Run("finalized", func(t *testing.T) {
		request = httptest.NewRequest(http.MethodGet, "http://foo.example/eth/v1/beacon/states/{state_id}/fork", nil)
		request.SetPathValue("state_id", "head")
		request.Header.Set("Accept", "application/octet-stream")
		writer = httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		parentRoot := [32]byte{'a'}
		blk := util.NewBeaconBlock()
		blk.Block.ParentRoot = parentRoot[:]
		root, err := blk.Block.HashTreeRoot()
		require.NoError(t, err)
		util.SaveBlock(t, ctx, db, blk)
		require.NoError(t, db.SaveGenesisBlockRoot(ctx, root))

		headerRoot, err := fakeState.LatestBlockHeader().HashTreeRoot()
		require.NoError(t, err)
		chainService = &chainMock.ChainService{
			FinalizedRoots: map[[32]byte]bool{
				headerRoot: true,
			},
		}
		server = &Server{
			Stater: &testutil.MockStater{
				BeaconState: fakeState,
			},
			HeadFetcher:           chainService,
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
			BeaconDB:              db,
		}
		server.GetStateFork(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		err = json.Unmarshal(writer.Body.Bytes(), &stateForkReponse)
		require.NoError(t, err)
		assert.DeepEqual(t, true, stateForkReponse.Finalized)
	})
}

func TestGetCommittees(t *testing.T) {
	db := dbTest.SetupDB(t)
	ctx := t.Context()
	url := "http://example.com/eth/v1/beacon/states/{state_id}/committees"

	var st state.BeaconState
	st, _ = util.DeterministicGenesisState(t, 8192)
	epoch := slots.ToEpoch(st.Slot())

	chainService := &chainMock.ChainService{}
	s := &Server{
		Stater: &testutil.MockStater{
			BeaconState: st,
		},
		HeadFetcher:           chainService,
		OptimisticModeFetcher: chainService,
		FinalizationFetcher:   chainService,
		BeaconDB:              db,
	}

	t.Run("Head all committees", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, url, nil)
		request.SetPathValue("state_id", "head")
		writer := httptest.NewRecorder()

		writer.Body = &bytes.Buffer{}
		s.GetCommittees(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetCommitteesResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, int(params.BeaconConfig().SlotsPerEpoch)*2, len(resp.Data))
		for _, datum := range resp.Data {
			index, err := strconv.ParseUint(datum.Index, 10, 32)
			require.NoError(t, err)
			slot, err := strconv.ParseUint(datum.Slot, 10, 32)
			require.NoError(t, err)
			assert.Equal(t, true, index == 0 || index == 1)
			assert.Equal(t, epoch, slots.ToEpoch(primitives.Slot(slot)))
		}
	})
	t.Run("Head all committees of epoch 10", func(t *testing.T) {
		query := url + "?epoch=10"
		request := httptest.NewRequest(http.MethodGet, query, nil)
		request.SetPathValue("state_id", "head")
		writer := httptest.NewRecorder()

		writer.Body = &bytes.Buffer{}
		s.GetCommittees(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetCommitteesResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		for _, datum := range resp.Data {
			slot, err := strconv.ParseUint(datum.Slot, 10, 32)
			require.NoError(t, err)
			assert.Equal(t, true, slot >= 320 && slot <= 351)
		}
	})
	t.Run("Head all committees of slot 4", func(t *testing.T) {
		query := url + "?slot=4"
		request := httptest.NewRequest(http.MethodGet, query, nil)
		request.SetPathValue("state_id", "head")
		writer := httptest.NewRecorder()

		writer.Body = &bytes.Buffer{}
		s.GetCommittees(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetCommitteesResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, 2, len(resp.Data))

		exSlot := uint64(4)
		exIndex := uint64(0)
		for _, datum := range resp.Data {
			slot, err := strconv.ParseUint(datum.Slot, 10, 32)
			require.NoError(t, err)
			index, err := strconv.ParseUint(datum.Index, 10, 32)
			require.NoError(t, err)
			assert.Equal(t, epoch, slots.ToEpoch(primitives.Slot(slot)))
			assert.Equal(t, exSlot, slot)
			assert.Equal(t, exIndex, index)
			exIndex++
		}
	})
	t.Run("Head all committees of index 1", func(t *testing.T) {
		query := url + "?index=1"
		request := httptest.NewRequest(http.MethodGet, query, nil)
		request.SetPathValue("state_id", "head")
		writer := httptest.NewRecorder()

		writer.Body = &bytes.Buffer{}
		s.GetCommittees(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetCommitteesResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, int(params.BeaconConfig().SlotsPerEpoch), len(resp.Data))

		exSlot := uint64(0)
		exIndex := uint64(1)
		for _, datum := range resp.Data {
			slot, err := strconv.ParseUint(datum.Slot, 10, 32)
			require.NoError(t, err)
			index, err := strconv.ParseUint(datum.Index, 10, 32)
			require.NoError(t, err)
			assert.Equal(t, epoch, slots.ToEpoch(primitives.Slot(slot)))
			assert.Equal(t, exSlot, slot)
			assert.Equal(t, exIndex, index)
			exSlot++
		}
	})
	t.Run("Head all committees of slot 2, index 1", func(t *testing.T) {
		query := url + "?slot=2&index=1"
		request := httptest.NewRequest(http.MethodGet, query, nil)
		request.SetPathValue("state_id", "head")
		writer := httptest.NewRecorder()

		writer.Body = &bytes.Buffer{}
		s.GetCommittees(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetCommitteesResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, 1, len(resp.Data))

		exIndex := uint64(1)
		exSlot := uint64(2)
		for _, datum := range resp.Data {
			index, err := strconv.ParseUint(datum.Index, 10, 32)
			require.NoError(t, err)
			slot, err := strconv.ParseUint(datum.Slot, 10, 32)
			require.NoError(t, err)
			assert.Equal(t, epoch, slots.ToEpoch(primitives.Slot(slot)))
			assert.Equal(t, exSlot, slot)
			assert.Equal(t, exIndex, index)
		}
	})
	t.Run("Execution optimistic", func(t *testing.T) {
		parentRoot := [32]byte{'a'}
		blk := util.NewBeaconBlock()
		blk.Block.ParentRoot = parentRoot[:]
		root, err := blk.Block.HashTreeRoot()
		require.NoError(t, err)
		util.SaveBlock(t, ctx, db, blk)
		require.NoError(t, db.SaveGenesisBlockRoot(ctx, root))

		chainService = &chainMock.ChainService{Optimistic: true}
		s = &Server{
			Stater: &testutil.MockStater{
				BeaconState: st,
			},
			HeadFetcher:           chainService,
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
			BeaconDB:              db,
		}

		request := httptest.NewRequest(http.MethodGet, url, nil)
		request.SetPathValue("state_id", "head")
		writer := httptest.NewRecorder()

		writer.Body = &bytes.Buffer{}
		s.GetCommittees(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetCommitteesResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, true, resp.ExecutionOptimistic)
	})
	t.Run("Finalized", func(t *testing.T) {
		parentRoot := [32]byte{'a'}
		blk := util.NewBeaconBlock()
		blk.Block.ParentRoot = parentRoot[:]
		root, err := blk.Block.HashTreeRoot()
		require.NoError(t, err)
		util.SaveBlock(t, ctx, db, blk)
		require.NoError(t, db.SaveGenesisBlockRoot(ctx, root))

		headerRoot, err := st.LatestBlockHeader().HashTreeRoot()
		require.NoError(t, err)
		chainService = &chainMock.ChainService{
			FinalizedRoots: map[[32]byte]bool{
				headerRoot: true,
			},
		}
		s = &Server{
			Stater: &testutil.MockStater{
				BeaconState: st,
			},
			HeadFetcher:           chainService,
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
			BeaconDB:              db,
		}

		request := httptest.NewRequest(http.MethodGet, url, nil)
		request.SetPathValue("state_id", "head")
		writer := httptest.NewRecorder()

		writer.Body = &bytes.Buffer{}
		s.GetCommittees(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetCommitteesResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.NoError(t, err)
		assert.Equal(t, true, resp.Finalized)
	})

	t.Run("Invalid slot for given epoch", func(t *testing.T) {
		cases := []struct {
			name      string
			epoch     string
			slot      string
			expectMsg string
		}{
			{
				name:      "Slot after the specified epoch",
				epoch:     "10",
				slot:      "400",
				expectMsg: "Slot 400 does not belong in epoch 10",
			},
			{
				name:      "Slot before the specified epoch",
				epoch:     "10",
				slot:      "300",
				expectMsg: "Slot 300 does not belong in epoch 10",
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				query := url + "?epoch=" + tc.epoch + "&slot=" + tc.slot
				request := httptest.NewRequest(http.MethodGet, query, nil)
				request.SetPathValue("state_id", "head")
				writer := httptest.NewRecorder()
				writer.Body = &bytes.Buffer{}
				s.GetCommittees(writer, request)
				assert.Equal(t, http.StatusBadRequest, writer.Code)
				var resp struct {
					Message string `json:"message"`
					Code    int    `json:"code"`
				}
				err := json.Unmarshal(writer.Body.Bytes(), &resp)
				assert.NoError(t, err)
				assert.Equal(t, tc.expectMsg, resp.Message)
			})
		}
	})
}

func TestGetBlockHeaders(t *testing.T) {
	beaconDB := dbTest.SetupDB(t)
	ctx := t.Context()

	_, blkContainers := fillDBTestBlocks(ctx, t, beaconDB)
	headBlock := blkContainers[len(blkContainers)-1]

	b1 := util.NewBeaconBlock()
	b1.Block.Slot = 30
	b1.Block.ParentRoot = bytesutil.PadTo([]byte{1}, 32)
	util.SaveBlock(t, ctx, beaconDB, b1)
	b2 := util.NewBeaconBlock()
	b2.Block.Slot = 30
	b2.Block.ParentRoot = bytesutil.PadTo([]byte{4}, 32)
	util.SaveBlock(t, ctx, beaconDB, b2)
	b3 := util.NewBeaconBlock()
	b3.Block.Slot = 31
	b3.Block.ParentRoot = bytesutil.PadTo([]byte{1}, 32)
	util.SaveBlock(t, ctx, beaconDB, b3)
	b4 := util.NewBeaconBlock()
	b4.Block.Slot = 28
	b4.Block.ParentRoot = bytesutil.PadTo([]byte{1}, 32)
	util.SaveBlock(t, ctx, beaconDB, b4)

	url := "http://example.com/eth/v1/beacon/headers"

	t.Run("list headers", func(t *testing.T) {
		wsb, err := blocks.NewSignedBeaconBlock(headBlock.Block.(*eth.BeaconBlockContainer_Phase0Block).Phase0Block)
		require.NoError(t, err)
		st, err := util.NewBeaconState()
		require.NoError(t, err)
		require.NoError(t, st.SetSlot(30))
		mockChainFetcher := &chainMock.ChainService{
			DB:                  beaconDB,
			Block:               wsb,
			Root:                headBlock.BlockRoot,
			FinalizedCheckPoint: &eth.Checkpoint{Root: blkContainers[64].BlockRoot},
			FinalizedRoots:      map[[32]byte]bool{},
			State:               st,
		}
		bs := &Server{
			BeaconDB:              beaconDB,
			ChainInfoFetcher:      mockChainFetcher,
			OptimisticModeFetcher: mockChainFetcher,
			FinalizationFetcher:   mockChainFetcher,
		}

		tests := []struct {
			name       string
			slot       string
			parentRoot string
			want       []*eth.SignedBeaconBlock
			wantErr    bool
		}{
			{
				name: "none",
				want: []*eth.SignedBeaconBlock{
					blkContainers[30].Block.(*eth.BeaconBlockContainer_Phase0Block).Phase0Block,
					b1,
					b2,
				},
			},
			{
				name: "slot",
				slot: "30",
				want: []*eth.SignedBeaconBlock{
					blkContainers[30].Block.(*eth.BeaconBlockContainer_Phase0Block).Phase0Block,
					b1,
					b2,
				},
			},
			{
				name:       "parent root",
				parentRoot: hexutil.Encode(b1.Block.ParentRoot),
				want: []*eth.SignedBeaconBlock{
					blkContainers[1].Block.(*eth.BeaconBlockContainer_Phase0Block).Phase0Block,
					b1,
					b3,
					b4,
				},
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				urlWithParams := fmt.Sprintf("%s?slot=%s&parent_root=%s", url, tt.slot, tt.parentRoot)
				request := httptest.NewRequest(http.MethodGet, urlWithParams, nil)
				writer := httptest.NewRecorder()

				writer.Body = &bytes.Buffer{}

				bs.GetBlockHeaders(writer, request)
				require.Equal(t, http.StatusOK, writer.Code)
				resp := &structs.GetBlockHeadersResponse{}
				require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))

				require.Equal(t, len(tt.want), len(resp.Data))
				for i, blk := range tt.want {
					expectedBodyRoot, err := blk.Block.Body.HashTreeRoot()
					require.NoError(t, err)
					expectedHeader := &eth.BeaconBlockHeader{
						Slot:          blk.Block.Slot,
						ProposerIndex: blk.Block.ProposerIndex,
						ParentRoot:    blk.Block.ParentRoot,
						StateRoot:     make([]byte, 32),
						BodyRoot:      expectedBodyRoot[:],
					}
					expectedHeaderRoot, err := expectedHeader.HashTreeRoot()
					require.NoError(t, err)
					assert.DeepEqual(t, hexutil.Encode(expectedHeaderRoot[:]), resp.Data[i].Root)
					assert.DeepEqual(t, structs.BeaconBlockHeaderFromConsensus(expectedHeader), resp.Data[i].Header.Message)
				}
			})
		}
	})

	t.Run("execution optimistic", func(t *testing.T) {
		wsb, err := blocks.NewSignedBeaconBlock(headBlock.Block.(*eth.BeaconBlockContainer_Phase0Block).Phase0Block)
		require.NoError(t, err)
		mockChainFetcher := &chainMock.ChainService{
			DB:                  beaconDB,
			Block:               wsb,
			Root:                headBlock.BlockRoot,
			FinalizedCheckPoint: &eth.Checkpoint{Root: blkContainers[64].BlockRoot},
			Optimistic:          true,
			FinalizedRoots:      map[[32]byte]bool{},
			OptimisticRoots: map[[32]byte]bool{
				bytesutil.ToBytes32(blkContainers[30].BlockRoot): true,
			},
		}
		bs := &Server{
			BeaconDB:              beaconDB,
			ChainInfoFetcher:      mockChainFetcher,
			OptimisticModeFetcher: mockChainFetcher,
			FinalizationFetcher:   mockChainFetcher,
		}
		slot := primitives.Slot(30)
		urlWithParams := fmt.Sprintf("%s?slot=%d", url, slot)
		request := httptest.NewRequest(http.MethodGet, urlWithParams, nil)
		writer := httptest.NewRecorder()

		writer.Body = &bytes.Buffer{}

		bs.GetBlockHeaders(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetBlockHeadersResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, true, resp.ExecutionOptimistic)
	})

	t.Run("finalized", func(t *testing.T) {
		wsb, err := blocks.NewSignedBeaconBlock(headBlock.Block.(*eth.BeaconBlockContainer_Phase0Block).Phase0Block)
		require.NoError(t, err)
		child1 := util.NewBeaconBlock()
		child1.Block.ParentRoot = bytesutil.PadTo([]byte("parent"), 32)
		child1.Block.Slot = 999
		util.SaveBlock(t, ctx, beaconDB, child1)
		child2 := util.NewBeaconBlock()
		child2.Block.ParentRoot = bytesutil.PadTo([]byte("parent"), 32)
		child2.Block.Slot = 1000
		util.SaveBlock(t, ctx, beaconDB, child2)
		child1Root, err := child1.Block.HashTreeRoot()
		require.NoError(t, err)
		child2Root, err := child2.Block.HashTreeRoot()
		require.NoError(t, err)
		mockChainFetcher := &chainMock.ChainService{
			DB:                  beaconDB,
			Block:               wsb,
			Root:                headBlock.BlockRoot,
			FinalizedCheckPoint: &eth.Checkpoint{Root: blkContainers[64].BlockRoot},
			FinalizedRoots:      map[[32]byte]bool{child1Root: true, child2Root: false},
		}
		bs := &Server{
			BeaconDB:              beaconDB,
			ChainInfoFetcher:      mockChainFetcher,
			OptimisticModeFetcher: mockChainFetcher,
			FinalizationFetcher:   mockChainFetcher,
		}

		t.Run("true", func(t *testing.T) {
			slot := primitives.Slot(999)
			urlWithParams := fmt.Sprintf("%s?slot=%d", url, slot)
			request := httptest.NewRequest(http.MethodGet, urlWithParams, nil)
			writer := httptest.NewRecorder()

			writer.Body = &bytes.Buffer{}

			bs.GetBlockHeaders(writer, request)
			require.Equal(t, http.StatusOK, writer.Code)
			resp := &structs.GetBlockHeadersResponse{}
			require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
			assert.Equal(t, true, resp.Finalized)
		})
		t.Run("false", func(t *testing.T) {
			slot := primitives.Slot(1000)
			urlWithParams := fmt.Sprintf("%s?slot=%d", url, slot)
			request := httptest.NewRequest(http.MethodGet, urlWithParams, nil)
			writer := httptest.NewRecorder()

			writer.Body = &bytes.Buffer{}

			bs.GetBlockHeaders(writer, request)
			require.Equal(t, http.StatusOK, writer.Code)
			resp := &structs.GetBlockHeadersResponse{}
			require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
			assert.Equal(t, false, resp.Finalized)
		})
		t.Run("false when at least one not finalized", func(t *testing.T) {
			urlWithParams := fmt.Sprintf("%s?parent_root=%s", url, hexutil.Encode(child1.Block.ParentRoot))
			request := httptest.NewRequest(http.MethodGet, urlWithParams, nil)
			writer := httptest.NewRecorder()

			writer.Body = &bytes.Buffer{}

			bs.GetBlockHeaders(writer, request)
			require.Equal(t, http.StatusOK, writer.Code)
			resp := &structs.GetBlockHeadersResponse{}
			require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
			assert.Equal(t, false, resp.Finalized)
		})
		t.Run("no blocks found", func(t *testing.T) {
			urlWithParams := fmt.Sprintf("%s?parent_root=%s", url, hexutil.Encode(bytes.Repeat([]byte{1}, 32)))
			request := httptest.NewRequest(http.MethodGet, urlWithParams, nil)
			writer := httptest.NewRecorder()

			writer.Body = &bytes.Buffer{}

			bs.GetBlockHeaders(writer, request)
			require.Equal(t, http.StatusNotFound, writer.Code)
			e := &httputil.DefaultJsonError{}
			require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
			assert.Equal(t, http.StatusNotFound, e.Code)
			assert.StringContains(t, "No blocks found", e.Message)
		})
	})
}

func TestServer_GetBlockHeader(t *testing.T) {
	b := util.NewBeaconBlock()
	b.Block.Slot = 123
	b.Block.ProposerIndex = 123
	b.Block.StateRoot = bytesutil.PadTo([]byte("stateroot"), 32)
	b.Block.ParentRoot = bytesutil.PadTo([]byte("parentroot"), 32)
	b.Block.Body.Graffiti = bytesutil.PadTo([]byte("graffiti"), 32)
	sb, err := blocks.NewSignedBeaconBlock(b)
	sb.SetSignature(bytesutil.PadTo([]byte("sig"), 96))
	require.NoError(t, err)

	mockBlockFetcher := &testutil.MockBlocker{BlockToReturn: sb}
	mockChainService := &chainMock.ChainService{
		FinalizedRoots: map[[32]byte]bool{},
	}
	s := &Server{
		ChainInfoFetcher:      mockChainService,
		OptimisticModeFetcher: mockChainService,
		FinalizationFetcher:   mockChainService,
		Blocker:               mockBlockFetcher,
	}

	t.Run("ok", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v1/beacon/headers/{block_id}", nil)
		request.SetPathValue("block_id", "head")
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetBlockHeader(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetBlockHeaderResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, true, resp.Data.Canonical)
		assert.Equal(t, "0xd7d92f6206707f2c9c4e7e82320617d5abac2b6461a65ea5bb1a154b5b5ea2fa", resp.Data.Root)
		assert.Equal(t, "0x736967000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000", resp.Data.Header.Signature)
		assert.Equal(t, "123", resp.Data.Header.Message.Slot)
		assert.Equal(t, "0x706172656e74726f6f7400000000000000000000000000000000000000000000", resp.Data.Header.Message.ParentRoot)
		assert.Equal(t, "123", resp.Data.Header.Message.ProposerIndex)
		assert.Equal(t, "0xdd32cbaa01c6c0ef399b293f86884ce6a15b532d34682edb16a48fa70ea5bc79", resp.Data.Header.Message.BodyRoot)
		assert.Equal(t, "0x7374617465726f6f740000000000000000000000000000000000000000000000", resp.Data.Header.Message.StateRoot)
	})
	t.Run("missing block_id", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v1/beacon/headers/{block_id}", nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetBlockHeader(writer, request)
		require.Equal(t, http.StatusBadRequest, writer.Code)
		e := &httputil.DefaultJsonError{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
		assert.StringContains(t, "block_id is required in URL params", e.Message)
	})
	t.Run("execution optimistic", func(t *testing.T) {
		r, err := sb.Block().HashTreeRoot()
		require.NoError(t, err)
		mockChainService := &chainMock.ChainService{
			OptimisticRoots: map[[32]byte]bool{r: true},
			FinalizedRoots:  map[[32]byte]bool{},
		}
		s := &Server{
			ChainInfoFetcher:      mockChainService,
			OptimisticModeFetcher: mockChainService,
			FinalizationFetcher:   mockChainService,
			Blocker:               mockBlockFetcher,
		}

		request := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v1/beacon/headers/{block_id}", nil)
		request.SetPathValue("block_id", "head")
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetBlockHeader(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetBlockHeaderResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, true, resp.ExecutionOptimistic)
	})
	t.Run("finalized", func(t *testing.T) {
		r, err := sb.Block().HashTreeRoot()
		require.NoError(t, err)

		t.Run("true", func(t *testing.T) {
			mockChainService := &chainMock.ChainService{FinalizedRoots: map[[32]byte]bool{r: true}}
			s := &Server{
				ChainInfoFetcher:      mockChainService,
				OptimisticModeFetcher: mockChainService,
				FinalizationFetcher:   mockChainService,
				Blocker:               mockBlockFetcher,
			}

			request := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v1/beacon/headers/{block_id}", nil)
			request.SetPathValue("block_id", hexutil.Encode(r[:]))
			writer := httptest.NewRecorder()
			writer.Body = &bytes.Buffer{}

			s.GetBlockHeader(writer, request)
			require.Equal(t, http.StatusOK, writer.Code)
			resp := &structs.GetBlockHeaderResponse{}
			require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
			assert.Equal(t, true, resp.Finalized)
		})
		t.Run("false", func(t *testing.T) {
			mockChainService := &chainMock.ChainService{FinalizedRoots: map[[32]byte]bool{r: false}}
			s := &Server{
				ChainInfoFetcher:      mockChainService,
				OptimisticModeFetcher: mockChainService,
				FinalizationFetcher:   mockChainService,
				Blocker:               mockBlockFetcher,
			}

			request := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v1/beacon/headers/{block_id}", nil)
			request.SetPathValue("block_id", hexutil.Encode(r[:]))
			writer := httptest.NewRecorder()
			writer.Body = &bytes.Buffer{}

			s.GetBlockHeader(writer, request)
			require.Equal(t, http.StatusOK, writer.Code)
			resp := &structs.GetBlockHeaderResponse{}
			require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
			assert.Equal(t, false, resp.Finalized)
		})
	})
}

func TestGetFinalityCheckpoints(t *testing.T) {
	fillCheckpoints := func(state *eth.BeaconState) error {
		state.PreviousJustifiedCheckpoint = &eth.Checkpoint{
			Root:  bytesutil.PadTo([]byte("previous"), 32),
			Epoch: 113,
		}
		state.CurrentJustifiedCheckpoint = &eth.Checkpoint{
			Root:  bytesutil.PadTo([]byte("current"), 32),
			Epoch: 123,
		}
		state.FinalizedCheckpoint = &eth.Checkpoint{
			Root:  bytesutil.PadTo([]byte("finalized"), 32),
			Epoch: 103,
		}
		return nil
	}
	fakeState, err := util.NewBeaconState(fillCheckpoints)
	require.NoError(t, err)

	stateProvider := func(ctx context.Context, stateId []byte) (state.BeaconState, error) {
		if bytes.Equal(stateId, []byte("foobar")) {
			return nil, &lookup.StateNotFoundError{}
		}
		return fakeState, nil
	}

	chainService := &chainMock.ChainService{}
	s := &Server{
		Stater: &testutil.MockStater{
			BeaconState:       fakeState,
			StateProviderFunc: stateProvider,
		},
		HeadFetcher:           chainService,
		OptimisticModeFetcher: chainService,
		FinalizationFetcher:   chainService,
	}

	t.Run("ok", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/eth/v1/beacon/states/{state_id}/finality_checkpoints", nil)
		request.SetPathValue("state_id", "head")
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetFinalityCheckpoints(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetFinalityCheckpointsResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.NotNil(t, resp.Data)
		assert.Equal(t, strconv.FormatUint(uint64(fakeState.FinalizedCheckpoint().Epoch), 10), resp.Data.Finalized.Epoch)
		assert.DeepEqual(t, hexutil.Encode(fakeState.FinalizedCheckpoint().Root), resp.Data.Finalized.Root)
		assert.Equal(t, strconv.FormatUint(uint64(fakeState.CurrentJustifiedCheckpoint().Epoch), 10), resp.Data.CurrentJustified.Epoch)
		assert.DeepEqual(t, hexutil.Encode(fakeState.CurrentJustifiedCheckpoint().Root), resp.Data.CurrentJustified.Root)
		assert.Equal(t, strconv.FormatUint(uint64(fakeState.PreviousJustifiedCheckpoint().Epoch), 10), resp.Data.PreviousJustified.Epoch)
		assert.DeepEqual(t, hexutil.Encode(fakeState.PreviousJustifiedCheckpoint().Root), resp.Data.PreviousJustified.Root)
	})
	t.Run("no state_id", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/eth/v1/beacon/states/{state_id}/finality_checkpoints", nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetFinalityCheckpoints(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &httputil.DefaultJsonError{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
		assert.StringContains(t, "state_id is required in URL params", e.Message)
	})
	t.Run("state not found", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/eth/v1/beacon/states/{state_id}/finality_checkpoints", nil)
		request.SetPathValue("state_id", "foobar")
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetFinalityCheckpoints(writer, request)
		assert.Equal(t, http.StatusNotFound, writer.Code)
		e := &httputil.DefaultJsonError{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusNotFound, e.Code)
		assert.StringContains(t, "State not found", e.Message)
	})
	t.Run("execution optimistic", func(t *testing.T) {
		chainService := &chainMock.ChainService{Optimistic: true}
		s := &Server{
			Stater: &testutil.MockStater{
				BeaconState: fakeState,
			},
			HeadFetcher:           chainService,
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
		}

		request := httptest.NewRequest(http.MethodGet, "/eth/v1/beacon/states/{state_id}/finality_checkpoints", nil)
		request.SetPathValue("state_id", "head")
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetFinalityCheckpoints(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetFinalityCheckpointsResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, true, resp.ExecutionOptimistic)
	})
	t.Run("finalized", func(t *testing.T) {
		headerRoot, err := fakeState.LatestBlockHeader().HashTreeRoot()
		require.NoError(t, err)
		chainService := &chainMock.ChainService{
			FinalizedRoots: map[[32]byte]bool{
				headerRoot: true,
			},
		}
		s := &Server{
			Stater: &testutil.MockStater{
				BeaconState: fakeState,
			},
			HeadFetcher:           chainService,
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
		}

		request := httptest.NewRequest(http.MethodGet, "/eth/v1/beacon/states/{state_id}/finality_checkpoints", nil)
		request.SetPathValue("state_id", "head")
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetFinalityCheckpoints(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetFinalityCheckpointsResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, true, resp.Finalized)
	})
}

func TestGetGenesis(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	config := params.BeaconConfig().Copy()
	config.GenesisForkVersion = []byte("genesis")
	params.OverrideBeaconConfig(config)
	genesis := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
	validatorsRoot := [32]byte{1, 2, 3, 4, 5, 6}

	t.Run("ok", func(t *testing.T) {
		chainService := &chainMock.ChainService{
			Genesis:        genesis,
			ValidatorsRoot: validatorsRoot,
		}
		s := Server{
			GenesisTimeFetcher: chainService,
			ChainInfoFetcher:   chainService,
		}

		request := httptest.NewRequest(http.MethodGet, "/eth/v1/beacon/genesis", nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetGenesis(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetGenesisResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.NotNil(t, resp.Data)

		assert.Equal(t, strconv.FormatInt(genesis.Unix(), 10), resp.Data.GenesisTime)
		assert.DeepEqual(t, hexutil.Encode(validatorsRoot[:]), resp.Data.GenesisValidatorsRoot)
		assert.DeepEqual(t, hexutil.Encode([]byte("genesis")), resp.Data.GenesisForkVersion)
	})
	t.Run("no genesis time", func(t *testing.T) {
		chainService := &chainMock.ChainService{
			Genesis:        time.Time{},
			ValidatorsRoot: validatorsRoot,
		}
		s := Server{
			GenesisTimeFetcher: chainService,
			ChainInfoFetcher:   chainService,
		}

		request := httptest.NewRequest(http.MethodGet, "/eth/v1/beacon/genesis", nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetGenesis(writer, request)
		assert.Equal(t, http.StatusNotFound, writer.Code)
		e := &httputil.DefaultJsonError{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusNotFound, e.Code)
		assert.StringContains(t, "Chain genesis info is not yet known", e.Message)
	})
	t.Run("no genesis validators root", func(t *testing.T) {
		chainService := &chainMock.ChainService{
			Genesis:        genesis,
			ValidatorsRoot: [32]byte{},
		}
		s := Server{
			GenesisTimeFetcher: chainService,
			ChainInfoFetcher:   chainService,
		}

		request := httptest.NewRequest(http.MethodGet, "/eth/v1/beacon/genesis", nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetGenesis(writer, request)
		assert.Equal(t, http.StatusNotFound, writer.Code)
		e := &httputil.DefaultJsonError{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusNotFound, e.Code)
		assert.StringContains(t, "Chain genesis info is not yet known", e.Message)
	})
}

func TestGetDepositSnapshot(t *testing.T) {
	beaconDB := dbTest.SetupDB(t)
	mockTrie := depositsnapshot.NewDepositTree()
	deposits := [][32]byte{
		bytesutil.ToBytes32([]byte{1}),
		bytesutil.ToBytes32([]byte{2}),
		bytesutil.ToBytes32([]byte{3}),
	}
	finalized := 2
	for _, leaf := range deposits {
		err := mockTrie.Insert(leaf[:], 0)
		require.NoError(t, err)
	}
	err := mockTrie.Finalize(1, deposits[1], 1)
	require.NoError(t, err)
	err = mockTrie.Finalize(2, deposits[2], 2)
	require.NoError(t, err)

	snapshot, err := mockTrie.GetSnapshot()
	require.NoError(t, err)
	root, err := snapshot.CalculateRoot()
	require.NoError(t, err)
	chainData := &eth.ETH1ChainData{
		DepositSnapshot: snapshot.ToProto(),
	}
	err = beaconDB.SaveExecutionChainData(t.Context(), chainData)
	require.NoError(t, err)
	s := Server{
		BeaconDB: beaconDB,
	}

	request := httptest.NewRequest(http.MethodGet, "/eth/v1/beacon/deposit_snapshot", nil)
	writer := httptest.NewRecorder()
	t.Run("JSON response", func(t *testing.T) {
		writer.Body = &bytes.Buffer{}
		s.GetDepositSnapshot(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetDepositSnapshotResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.NotNil(t, resp.Data)

		assert.Equal(t, hexutil.Encode(root[:]), resp.Data.DepositRoot)
		assert.Equal(t, hexutil.Encode(deposits[2][:]), resp.Data.ExecutionBlockHash)
		assert.Equal(t, strconv.Itoa(mockTrie.NumOfItems()), resp.Data.DepositCount)
		assert.Equal(t, finalized, len(resp.Data.Finalized))
	})
	t.Run("SSZ response", func(t *testing.T) {
		writer.Body = &bytes.Buffer{}
		request.Header.Set("Accept", api.OctetStreamMediaType)
		s.GetDepositSnapshot(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &eth.DepositSnapshot{}
		require.NoError(t, resp.UnmarshalSSZ(writer.Body.Bytes()))

		assert.Equal(t, hexutil.Encode(root[:]), hexutil.Encode(resp.DepositRoot))
		assert.Equal(t, hexutil.Encode(deposits[2][:]), hexutil.Encode(resp.ExecutionHash))
		assert.Equal(t, uint64(mockTrie.NumOfItems()), resp.DepositCount)
		assert.Equal(t, finalized, len(resp.Finalized))
	})
}

func TestServer_broadcastBlobSidecars(t *testing.T) {
	hook := logTest.NewGlobal()
	blockToPropose := util.NewBeaconBlockContentsDeneb()
	blockToPropose.Blobs = [][]byte{{0x01}, {0x02}, {0x03}}
	blockToPropose.KzgProofs = [][]byte{{0x01}, {0x02}, {0x03}}
	blockToPropose.Block.Block.Body.BlobKzgCommitments = [][]byte{bytesutil.PadTo([]byte("kc"), 48), bytesutil.PadTo([]byte("kc1"), 48), bytesutil.PadTo([]byte("kc2"), 48)}
	d := &eth.GenericSignedBeaconBlock_Deneb{Deneb: blockToPropose}
	b := &eth.GenericSignedBeaconBlock{Block: d}

	server := &Server{
		Broadcaster:         &mockp2p.MockBroadcaster{},
		FinalizationFetcher: &chainMock.ChainService{NotFinalized: true},
	}

	blk, err := blocks.NewSignedBeaconBlock(b.Block)
	require.NoError(t, err)
	require.NoError(t, server.broadcastSeenBlockSidecars(t.Context(), blk, b.GetDeneb().Blobs, b.GetDeneb().KzgProofs))
	require.LogsDoNotContain(t, hook, "Broadcasted blob sidecar for already seen block")

	server.FinalizationFetcher = &chainMock.ChainService{NotFinalized: false}
	require.NoError(t, server.broadcastSeenBlockSidecars(t.Context(), blk, b.GetDeneb().Blobs, b.GetDeneb().KzgProofs))
	require.LogsContain(t, hook, "Broadcasted blob sidecar for already seen block")
}

func Test_validateBlobs(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.BeaconConfig().FuluForkEpoch = params.BeaconConfig().ElectraForkEpoch + 4096*2
	ds := util.SlotAtEpoch(t, params.BeaconConfig().DenebForkEpoch)
	es := util.SlotAtEpoch(t, params.BeaconConfig().ElectraForkEpoch)
	fe := params.BeaconConfig().FuluForkEpoch
	fs := util.SlotAtEpoch(t, fe)

	require.NoError(t, kzg.Start())

	denebMax := params.BeaconConfig().MaxBlobsPerBlock(ds)
	blob := util.GetRandBlob(123)
	// Generate proper commitment and proof for the blob
	var kzgBlob kzg.Blob
	copy(kzgBlob[:], blob[:])
	commitment, err := kzg.BlobToKZGCommitment(&kzgBlob)
	require.NoError(t, err)
	proof, err := kzg.ComputeBlobKZGProof(&kzgBlob, commitment)
	require.NoError(t, err)
	blk := util.NewBeaconBlockDeneb()
	blk.Block.Slot = ds
	blk.Block.Body.BlobKzgCommitments = [][]byte{commitment[:]}
	b, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	s := &Server{}
	require.NoError(t, s.validateBlobs(b, [][]byte{blob[:]}, [][]byte{proof[:]}))

	require.ErrorContains(t, "number of blobs (1), proofs (0), and commitments (1) do not match", s.validateBlobs(b, [][]byte{blob[:]}, [][]byte{}))

	sk, err := bls.RandKey()
	require.NoError(t, err)
	blk.Block.Body.BlobKzgCommitments = [][]byte{sk.PublicKey().Marshal()}
	b, err = blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	require.ErrorContains(t, "could not verify blob proofs", s.validateBlobs(b, [][]byte{blob[:]}, [][]byte{proof[:]}))

	electraMax := params.BeaconConfig().MaxBlobsPerBlock(es)
	blobs := [][]byte{}
	commitments := [][]byte{}
	proofs := [][]byte{}
	for i := 0; i < electraMax+1; i++ {
		blobs = append(blobs, blob[:])
		commitments = append(commitments, commitment[:])
		proofs = append(proofs, proof[:])
	}
	t.Run("pre-Deneb block should return early", func(t *testing.T) {
		// Create a pre-Deneb block (e.g., Capella)
		blk := util.NewBeaconBlockCapella()
		b, err := blocks.NewSignedBeaconBlock(blk)
		require.NoError(t, err)
		s := &Server{}
		// Should return nil for pre-Deneb blocks regardless of blobs
		require.NoError(t, s.validateBlobs(b, [][]byte{}, [][]byte{}))
		require.NoError(t, s.validateBlobs(b, blobs[:1], proofs[:1]))
	})

	t.Run("Deneb block with valid single blob", func(t *testing.T) {
		blk := util.NewBeaconBlockDeneb()
		blk.Block.Slot = ds
		blk.Block.Body.BlobKzgCommitments = [][]byte{commitment[:]}
		b, err := blocks.NewSignedBeaconBlock(blk)
		require.NoError(t, err)
		s := &Server{}
		require.NoError(t, s.validateBlobs(b, [][]byte{blob[:]}, [][]byte{proof[:]}))
	})

	t.Run("Deneb block with max blobs (6)", func(t *testing.T) {
		blk := util.NewBeaconBlockDeneb()
		blk.Block.Slot = ds
		blk.Block.Body.BlobKzgCommitments = commitments[:6]
		b, err := blocks.NewSignedBeaconBlock(blk)
		require.NoError(t, err)
		s := &Server{}
		// Should pass with exactly 6 blobs
		require.NoError(t, s.validateBlobs(b, blobs[:denebMax], proofs[:denebMax]))
	})

	t.Run("Deneb block exceeding max blobs", func(t *testing.T) {
		blk := util.NewBeaconBlockDeneb()
		blk.Block.Slot = ds
		blk.Block.Body.BlobKzgCommitments = commitments[:7]
		b, err := blocks.NewSignedBeaconBlock(blk)
		require.NoError(t, err)
		s := &Server{}
		// Should fail with 7 blobs when max is 6
		err = s.validateBlobs(b, blobs[:denebMax+1], proofs[:denebMax+1])
		require.ErrorContains(t, "number of blobs over max", err)
	})

	t.Run("Electra block with valid blobs", func(t *testing.T) {
		blk := util.NewBeaconBlockElectra()
		blk.Block.Slot = es
		blk.Block.Body.BlobKzgCommitments = commitments[:9]
		b, err := blocks.NewSignedBeaconBlock(blk)
		require.NoError(t, err)
		s := &Server{}
		// Should pass with 9 blobs in Electra
		require.NoError(t, s.validateBlobs(b, blobs[:electraMax], proofs[:electraMax]))
	})

	t.Run("Electra block exceeding max blobs", func(t *testing.T) {
		blk := util.NewBeaconBlockElectra()
		blk.Block.Slot = es
		blk.Block.Body.BlobKzgCommitments = commitments[:electraMax+1]
		b, err := blocks.NewSignedBeaconBlock(blk)
		require.NoError(t, err)
		s := &Server{}
		// Should fail with 10 blobs when max is 9
		err = s.validateBlobs(b, blobs[:electraMax+1], proofs[:electraMax+1])
		require.ErrorContains(t, "number of blobs over max", err)
	})

	t.Run("Fulu block with valid cell proofs", func(t *testing.T) {
		blk := util.NewBeaconBlockFulu()
		blk.Block.Slot = fs

		// Generate valid commitments and cell proofs for testing
		blobCount := 2
		commitments := make([][]byte, blobCount)
		fuluBlobs := make([][]byte, blobCount)
		var kzgBlobs []kzg.Blob

		for i := 0; i < blobCount; i++ {
			blob := util.GetRandBlob(int64(i))
			fuluBlobs[i] = blob[:]
			var kzgBlob kzg.Blob
			copy(kzgBlob[:], blob[:])
			kzgBlobs = append(kzgBlobs, kzgBlob)

			// Generate commitment
			commitment, err := kzg.BlobToKZGCommitment(&kzgBlob)
			require.NoError(t, err)
			commitments[i] = commitment[:]
		}

		blk.Block.Body.BlobKzgCommitments = commitments
		b, err := blocks.NewSignedBeaconBlock(blk)
		require.NoError(t, err)

		// Generate cell proofs for the blobs (flattened format like execution client)
		numberOfColumns := params.BeaconConfig().NumberOfColumns
		cellProofs := make([][]byte, uint64(blobCount)*numberOfColumns)
		for blobIdx := 0; blobIdx < blobCount; blobIdx++ {
			cellsAndProofs, err := kzg.ComputeCellsAndKZGProofs(&kzgBlobs[blobIdx])
			require.NoError(t, err)

			for colIdx := uint64(0); colIdx < numberOfColumns; colIdx++ {
				cellProofIdx := uint64(blobIdx)*numberOfColumns + colIdx
				cellProofs[cellProofIdx] = cellsAndProofs.Proofs[colIdx][:]
			}
		}

		s := &Server{}
		// Should use cell batch verification for Fulu blocks
		require.NoError(t, s.validateBlobs(b, fuluBlobs, cellProofs))
	})

	t.Run("Fulu block with invalid cell proof count", func(t *testing.T) {
		blk := util.NewBeaconBlockFulu()
		blk.Block.Slot = fs

		// Create valid commitments but wrong number of cell proofs
		blobCount := 2
		commitments := make([][]byte, blobCount)
		fuluBlobs := make([][]byte, blobCount)
		for i := 0; i < blobCount; i++ {
			blob := util.GetRandBlob(int64(i))
			fuluBlobs[i] = blob[:]

			var kzgBlob kzg.Blob
			copy(kzgBlob[:], blob[:])
			commitment, err := kzg.BlobToKZGCommitment(&kzgBlob)
			require.NoError(t, err)
			commitments[i] = commitment[:]
		}

		blk.Block.Body.BlobKzgCommitments = commitments
		b, err := blocks.NewSignedBeaconBlock(blk)
		require.NoError(t, err)

		// Wrong number of cell proofs (should be blobCount * numberOfColumns)
		wrongCellProofs := make([][]byte, 10) // Too few proofs

		s := &Server{}
		err = s.validateBlobs(b, fuluBlobs, wrongCellProofs)
		require.ErrorContains(t, "do not match", err)
	})

	t.Run("Deneb block with invalid blob proof", func(t *testing.T) {
		blob := util.GetRandBlob(123)
		invalidProof := make([]byte, 48) // All zeros - invalid proof

		sk, err := bls.RandKey()
		require.NoError(t, err)

		blk := util.NewBeaconBlockDeneb()
		blk.Block.Slot = ds
		blk.Block.Body.BlobKzgCommitments = [][]byte{sk.PublicKey().Marshal()}
		b, err := blocks.NewSignedBeaconBlock(blk)
		require.NoError(t, err)

		s := &Server{}
		err = s.validateBlobs(b, [][]byte{blob[:]}, [][]byte{invalidProof})
		require.ErrorContains(t, "could not verify blob proofs", err)
	})

	t.Run("empty blobs and proofs should pass", func(t *testing.T) {
		blk := util.NewBeaconBlockDeneb()
		blk.Block.Slot = ds
		blk.Block.Body.BlobKzgCommitments = [][]byte{}
		b, err := blocks.NewSignedBeaconBlock(blk)
		require.NoError(t, err)

		s := &Server{}
		require.NoError(t, s.validateBlobs(b, [][]byte{}, [][]byte{}))
	})

	t.Run("BlobSchedule with progressive increases (BPO)", func(t *testing.T) {
		cfg := params.BeaconConfig().Copy()
		defer params.OverrideBeaconConfig(cfg)

		// Set up config with BlobSchedule (BPO - Blob Production Optimization)
		testCfg := params.BeaconConfig().Copy()
		testCfg.DeprecatedMaxBlobsPerBlock = 6
		testCfg.DeprecatedMaxBlobsPerBlockElectra = 9
		// Define blob schedule with progressive increases
		testCfg.BlobSchedule = []params.BlobScheduleEntry{
			{Epoch: fe + 1, MaxBlobsPerBlock: 3},  // Start with 3 blobs
			{Epoch: fe + 10, MaxBlobsPerBlock: 5}, // Increase to 5 at epoch 10
			{Epoch: fe + 20, MaxBlobsPerBlock: 7}, // Increase to 7 at epoch 20
			{Epoch: fe + 30, MaxBlobsPerBlock: 9}, // Increase to 9 at epoch 30
		}
		params.OverrideBeaconConfig(testCfg)

		s := &Server{}
		t.Run("deneb under and over max", func(t *testing.T) {
			blk := util.NewBeaconBlockDeneb()
			blk.Block.Slot = ds
			blk.Block.Body.BlobKzgCommitments = commitments[:denebMax]
			b, err := blocks.NewSignedBeaconBlock(blk)
			require.NoError(t, err)
			require.NoError(t, s.validateBlobs(b, blobs[:denebMax], proofs[:denebMax]))

			// Should fail with 4 blobs
			blk.Block.Body.BlobKzgCommitments = commitments[:4]
			b, err = blocks.NewSignedBeaconBlock(blk)
			require.NoError(t, err)
			err = s.validateBlobs(b, blobs[:denebMax+1], proofs[:denebMax+1])
			require.ErrorContains(t, "number of blobs over max", err)
		})

		// Test epoch 30+: max 9 blobs
		t.Run("different max in electra", func(t *testing.T) {
			blk := util.NewBeaconBlockElectra()
			blk.Block.Slot = es
			blk.Block.Body.BlobKzgCommitments = commitments[:electraMax]
			b, err := blocks.NewSignedBeaconBlock(blk)
			require.NoError(t, err)
			require.NoError(t, s.validateBlobs(b, blobs[:electraMax], proofs[:electraMax]))

			// exceed the electra max
			blk.Block.Body.BlobKzgCommitments = commitments[:electraMax+1]
			b, err = blocks.NewSignedBeaconBlock(blk)
			require.NoError(t, err)
			err = s.validateBlobs(b, blobs[:electraMax+1], proofs[:electraMax+1])
			require.ErrorContains(t, "number of blobs over max, 10 > 9", err)
		})
	})
}

func TestGetPendingConsolidations(t *testing.T) {
	st, _ := util.DeterministicGenesisStateElectra(t, 10)

	cs := make([]*eth.PendingConsolidation, 10)
	for i := 0; i < len(cs); i += 1 {
		cs[i] = &eth.PendingConsolidation{
			SourceIndex: primitives.ValidatorIndex(i),
			TargetIndex: primitives.ValidatorIndex(i + 1),
		}
	}
	require.NoError(t, st.SetPendingConsolidations(cs))

	chainService := &chainMock.ChainService{
		Optimistic:     false,
		FinalizedRoots: map[[32]byte]bool{},
	}
	server := &Server{
		Stater: &testutil.MockStater{
			BeaconState: st,
		},
		OptimisticModeFetcher: chainService,
		FinalizationFetcher:   chainService,
	}

	t.Run("json response", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v1/beacon/states/{state_id}/pending_consolidations", nil)
		req.SetPathValue("state_id", "head")
		rec := httptest.NewRecorder()
		rec.Body = new(bytes.Buffer)

		server.GetPendingConsolidations(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		require.Equal(t, "electra", rec.Header().Get(api.VersionHeader))

		var resp structs.GetPendingConsolidationsResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

		expectedVersion := version.String(st.Version())
		require.Equal(t, expectedVersion, resp.Version)

		require.Equal(t, false, resp.ExecutionOptimistic)
		require.Equal(t, false, resp.Finalized)

		expectedConsolidations := structs.PendingConsolidationsFromConsensus(cs)
		require.DeepEqual(t, expectedConsolidations, resp.Data)
	})
	t.Run("ssz response", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v1/beacon/states/{state_id}/pending_consolidations", nil)
		req.Header.Set("Accept", "application/octet-stream")
		req.SetPathValue("state_id", "head")
		rec := httptest.NewRecorder()
		rec.Body = new(bytes.Buffer)

		server.GetPendingConsolidations(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		require.Equal(t, "electra", rec.Header().Get(api.VersionHeader))

		responseBytes := rec.Body.Bytes()
		var recoveredConsolidations []*eth.PendingConsolidation

		// Verify total size matches expected number of deposits
		consolidationSize := (&eth.PendingConsolidation{}).SizeSSZ()
		require.Equal(t, len(responseBytes), consolidationSize*len(cs))

		for i := 0; i < len(cs); i++ {
			start := i * consolidationSize
			end := start + consolidationSize

			var c eth.PendingConsolidation
			require.NoError(t, c.UnmarshalSSZ(responseBytes[start:end]))
			recoveredConsolidations = append(recoveredConsolidations, &c)
		}
		require.DeepEqual(t, cs, recoveredConsolidations)
	})
	t.Run("pre electra state", func(t *testing.T) {
		preElectraSt, _ := util.DeterministicGenesisStateDeneb(t, 1)
		preElectraServer := &Server{
			Stater: &testutil.MockStater{
				BeaconState: preElectraSt,
			},
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
		}

		// Test JSON request
		req := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v1/beacon/states/{state_id}/pending_consolidations", nil)
		req.SetPathValue("state_id", "head")
		rec := httptest.NewRecorder()
		rec.Body = new(bytes.Buffer)

		preElectraServer.GetPendingConsolidations(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code)

		var errResp struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
		require.Equal(t, "state_id is prior to electra", errResp.Message)

		// Test SSZ request
		sszReq := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v1/beacon/states/{state_id}/pending_consolidations", nil)
		sszReq.Header.Set("Accept", "application/octet-stream")
		sszReq.SetPathValue("state_id", "head")
		sszRec := httptest.NewRecorder()
		sszRec.Body = new(bytes.Buffer)

		preElectraServer.GetPendingConsolidations(sszRec, sszReq)
		require.Equal(t, http.StatusBadRequest, sszRec.Code)

		var sszErrResp struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		require.NoError(t, json.Unmarshal(sszRec.Body.Bytes(), &sszErrResp))
		require.Equal(t, "state_id is prior to electra", sszErrResp.Message)
	})
	t.Run("missing state_id parameter", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v1/beacon/states/{state_id}/pending_consolidations", nil)
		// Intentionally not setting state_id
		rec := httptest.NewRecorder()
		rec.Body = new(bytes.Buffer)

		server.GetPendingConsolidations(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code)

		var errResp struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
		require.Equal(t, "state_id is required in URL params", errResp.Message)
	})
	t.Run("optimistic node", func(t *testing.T) {
		optimisticChainService := &chainMock.ChainService{
			Optimistic:     true,
			FinalizedRoots: map[[32]byte]bool{},
		}
		optimisticServer := &Server{
			Stater:                server.Stater,
			OptimisticModeFetcher: optimisticChainService,
			FinalizationFetcher:   optimisticChainService,
		}

		req := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v1/beacon/states/{state_id}/pending_consolidations", nil)
		req.SetPathValue("state_id", "head")
		rec := httptest.NewRecorder()
		rec.Body = new(bytes.Buffer)

		optimisticServer.GetPendingConsolidations(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		var resp structs.GetPendingConsolidationsResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		require.Equal(t, true, resp.ExecutionOptimistic)
	})

	t.Run("finalized node", func(t *testing.T) {
		blockRoot, err := st.LatestBlockHeader().HashTreeRoot()
		require.NoError(t, err)

		finalizedChainService := &chainMock.ChainService{
			Optimistic:     false,
			FinalizedRoots: map[[32]byte]bool{blockRoot: true},
		}
		finalizedServer := &Server{
			Stater:                server.Stater,
			OptimisticModeFetcher: finalizedChainService,
			FinalizationFetcher:   finalizedChainService,
		}

		req := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v1/beacon/states/{state_id}/pending_consolidations", nil)
		req.SetPathValue("state_id", "head")
		rec := httptest.NewRecorder()
		rec.Body = new(bytes.Buffer)

		finalizedServer.GetPendingConsolidations(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		var resp structs.GetPendingConsolidationsResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		require.Equal(t, true, resp.Finalized)
	})
}

func TestGetPendingDeposits(t *testing.T) {
	st, _ := util.DeterministicGenesisStateElectra(t, 10)

	validators := st.Validators()
	dummySig := make([]byte, 96)
	for j := 0; j < 96; j++ {
		dummySig[j] = byte(j)
	}
	deps := make([]*eth.PendingDeposit, 10)
	for i := 0; i < len(deps); i += 1 {
		deps[i] = &eth.PendingDeposit{
			PublicKey:             validators[i].PublicKey,
			WithdrawalCredentials: validators[i].WithdrawalCredentials,
			Amount:                100,
			Slot:                  0,
			Signature:             dummySig,
		}
	}
	require.NoError(t, st.SetPendingDeposits(deps))

	chainService := &chainMock.ChainService{
		Optimistic:     false,
		FinalizedRoots: map[[32]byte]bool{},
	}
	server := &Server{
		Stater: &testutil.MockStater{
			BeaconState: st,
		},
		OptimisticModeFetcher: chainService,
		FinalizationFetcher:   chainService,
	}

	t.Run("json response", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v1/beacon/states/{state_id}/pending_deposits", nil)
		req.SetPathValue("state_id", "head")
		rec := httptest.NewRecorder()
		rec.Body = new(bytes.Buffer)

		server.GetPendingDeposits(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		require.Equal(t, "electra", rec.Header().Get(api.VersionHeader))

		var resp structs.GetPendingDepositsResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

		expectedVersion := version.String(st.Version())
		require.Equal(t, expectedVersion, resp.Version)

		require.Equal(t, false, resp.ExecutionOptimistic)
		require.Equal(t, false, resp.Finalized)

		expectedDeposits := structs.PendingDepositsFromConsensus(deps)
		require.DeepEqual(t, expectedDeposits, resp.Data)
	})
	t.Run("ssz response", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v1/beacon/states/{state_id}/pending_deposits", nil)
		req.Header.Set("Accept", "application/octet-stream")
		req.SetPathValue("state_id", "head")
		rec := httptest.NewRecorder()
		rec.Body = new(bytes.Buffer)

		server.GetPendingDeposits(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		require.Equal(t, "electra", rec.Header().Get(api.VersionHeader))

		responseBytes := rec.Body.Bytes()
		var recoveredDeposits []*eth.PendingDeposit

		// Verify total size matches expected number of deposits
		depositSize := (&eth.PendingDeposit{}).SizeSSZ()
		require.Equal(t, len(responseBytes), depositSize*len(deps))

		for i := 0; i < len(deps); i++ {
			start := i * depositSize
			end := start + depositSize

			var deposit eth.PendingDeposit
			require.NoError(t, deposit.UnmarshalSSZ(responseBytes[start:end]))
			recoveredDeposits = append(recoveredDeposits, &deposit)
		}
		require.DeepEqual(t, deps, recoveredDeposits)
	})
	t.Run("pre electra state", func(t *testing.T) {
		preElectraSt, _ := util.DeterministicGenesisStateDeneb(t, 1)
		preElectraServer := &Server{
			Stater: &testutil.MockStater{
				BeaconState: preElectraSt,
			},
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
		}

		// Test JSON request
		req := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v1/beacon/states/{state_id}/pending_deposits", nil)
		req.SetPathValue("state_id", "head")
		rec := httptest.NewRecorder()
		rec.Body = new(bytes.Buffer)

		preElectraServer.GetPendingDeposits(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code)

		var errResp struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
		require.Equal(t, "state_id is prior to electra", errResp.Message)

		// Test SSZ request
		sszReq := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v1/beacon/states/{state_id}/pending_deposits", nil)
		sszReq.Header.Set("Accept", "application/octet-stream")
		sszReq.SetPathValue("state_id", "head")
		sszRec := httptest.NewRecorder()
		sszRec.Body = new(bytes.Buffer)

		preElectraServer.GetPendingDeposits(sszRec, sszReq)
		require.Equal(t, http.StatusBadRequest, sszRec.Code)

		var sszErrResp struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		require.NoError(t, json.Unmarshal(sszRec.Body.Bytes(), &sszErrResp))
		require.Equal(t, "state_id is prior to electra", sszErrResp.Message)
	})
	t.Run("missing state_id parameter", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v1/beacon/states/{state_id}/pending_deposits", nil)
		// Intentionally not setting state_id
		rec := httptest.NewRecorder()
		rec.Body = new(bytes.Buffer)

		server.GetPendingDeposits(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code)

		var errResp struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
		require.Equal(t, "state_id is required in URL params", errResp.Message)
	})
	t.Run("optimistic node", func(t *testing.T) {
		optimisticChainService := &chainMock.ChainService{
			Optimistic:     true,
			FinalizedRoots: map[[32]byte]bool{},
		}
		optimisticServer := &Server{
			Stater:                server.Stater,
			OptimisticModeFetcher: optimisticChainService,
			FinalizationFetcher:   optimisticChainService,
		}

		req := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v1/beacon/states/{state_id}/pending_deposits", nil)
		req.SetPathValue("state_id", "head")
		rec := httptest.NewRecorder()
		rec.Body = new(bytes.Buffer)

		optimisticServer.GetPendingDeposits(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		var resp structs.GetPendingDepositsResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		require.Equal(t, true, resp.ExecutionOptimistic)
	})

	t.Run("finalized node", func(t *testing.T) {
		blockRoot, err := st.LatestBlockHeader().HashTreeRoot()
		require.NoError(t, err)

		finalizedChainService := &chainMock.ChainService{
			Optimistic:     false,
			FinalizedRoots: map[[32]byte]bool{blockRoot: true},
		}
		finalizedServer := &Server{
			Stater:                server.Stater,
			OptimisticModeFetcher: finalizedChainService,
			FinalizationFetcher:   finalizedChainService,
		}

		req := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v1/beacon/states/{state_id}/pending_deposits", nil)
		req.SetPathValue("state_id", "head")
		rec := httptest.NewRecorder()
		rec.Body = new(bytes.Buffer)

		finalizedServer.GetPendingDeposits(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		var resp structs.GetPendingDepositsResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		require.Equal(t, true, resp.Finalized)
	})
}

func TestGetPendingPartialWithdrawals(t *testing.T) {
	st, _ := util.DeterministicGenesisStateElectra(t, 10)
	for i := 0; i < 10; i += 1 {
		err := st.AppendPendingPartialWithdrawal(
			&eth.PendingPartialWithdrawal{
				Index:             primitives.ValidatorIndex(i),
				Amount:            100,
				WithdrawableEpoch: primitives.Epoch(0),
			})
		require.NoError(t, err)
	}
	withdrawals, err := st.PendingPartialWithdrawals()
	require.NoError(t, err)

	chainService := &chainMock.ChainService{
		Optimistic:     false,
		FinalizedRoots: map[[32]byte]bool{},
	}
	server := &Server{
		Stater: &testutil.MockStater{
			BeaconState: st,
		},
		OptimisticModeFetcher: chainService,
		FinalizationFetcher:   chainService,
	}

	t.Run("json response", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v1/beacon/states/{state_id}/pending_partial_withdrawals", nil)
		req.SetPathValue("state_id", "head")
		rec := httptest.NewRecorder()
		rec.Body = new(bytes.Buffer)

		server.GetPendingPartialWithdrawals(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		require.Equal(t, "electra", rec.Header().Get(api.VersionHeader))

		var resp structs.GetPendingPartialWithdrawalsResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

		expectedVersion := version.String(st.Version())
		require.Equal(t, expectedVersion, resp.Version)

		require.Equal(t, false, resp.ExecutionOptimistic)
		require.Equal(t, false, resp.Finalized)

		expectedWithdrawals := structs.PendingPartialWithdrawalsFromConsensus(withdrawals)
		require.DeepEqual(t, expectedWithdrawals, resp.Data)
	})

	t.Run("ssz response", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v1/beacon/states/{state_id}/pending_partial_withdrawals", nil)
		req.Header.Set("Accept", "application/octet-stream")
		req.SetPathValue("state_id", "head")
		rec := httptest.NewRecorder()
		rec.Body = new(bytes.Buffer)

		server.GetPendingPartialWithdrawals(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		require.Equal(t, "electra", rec.Header().Get(api.VersionHeader))

		responseBytes := rec.Body.Bytes()
		var recoveredWithdrawals []*eth.PendingPartialWithdrawal

		withdrawalSize := (&eth.PendingPartialWithdrawal{}).SizeSSZ()
		require.Equal(t, len(responseBytes), withdrawalSize*len(withdrawals))

		for i := 0; i < len(withdrawals); i++ {
			start := i * withdrawalSize
			end := start + withdrawalSize

			var withdrawal eth.PendingPartialWithdrawal
			require.NoError(t, withdrawal.UnmarshalSSZ(responseBytes[start:end]))
			recoveredWithdrawals = append(recoveredWithdrawals, &withdrawal)
		}
		require.DeepEqual(t, withdrawals, recoveredWithdrawals)
	})

	t.Run("pre electra state", func(t *testing.T) {
		preElectraSt, _ := util.DeterministicGenesisStateDeneb(t, 1)
		preElectraServer := &Server{
			Stater: &testutil.MockStater{
				BeaconState: preElectraSt,
			},
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
		}

		// Test JSON request
		req := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v1/beacon/states/{state_id}/pending_partial_withdrawals", nil)
		req.SetPathValue("state_id", "head")
		rec := httptest.NewRecorder()
		rec.Body = new(bytes.Buffer)

		preElectraServer.GetPendingPartialWithdrawals(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code)

		var errResp struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
		require.Equal(t, "state_id is prior to electra", errResp.Message)

		// Test SSZ request
		sszReq := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v1/beacon/states/{state_id}/pending_partial_withdrawals", nil)
		sszReq.Header.Set("Accept", "application/octet-stream")
		sszReq.SetPathValue("state_id", "head")
		sszRec := httptest.NewRecorder()
		sszRec.Body = new(bytes.Buffer)

		preElectraServer.GetPendingPartialWithdrawals(sszRec, sszReq)
		require.Equal(t, http.StatusBadRequest, sszRec.Code)

		var sszErrResp struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		require.NoError(t, json.Unmarshal(sszRec.Body.Bytes(), &sszErrResp))
		require.Equal(t, "state_id is prior to electra", sszErrResp.Message)
	})

	t.Run("missing state_id parameter", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v1/beacon/states/{state_id}/pending_partial_withdrawals", nil)
		// Intentionally not setting state_id
		rec := httptest.NewRecorder()
		rec.Body = new(bytes.Buffer)

		server.GetPendingPartialWithdrawals(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code)

		var errResp struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
		require.Equal(t, "state_id is required in URL params", errResp.Message)
	})

	t.Run("optimistic node", func(t *testing.T) {
		optimisticChainService := &chainMock.ChainService{
			Optimistic:     true,
			FinalizedRoots: map[[32]byte]bool{},
		}
		optimisticServer := &Server{
			Stater:                server.Stater,
			OptimisticModeFetcher: optimisticChainService,
			FinalizationFetcher:   optimisticChainService,
		}

		req := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v1/beacon/states/{state_id}/pending_partial_withdrawals", nil)
		req.SetPathValue("state_id", "head")
		rec := httptest.NewRecorder()
		rec.Body = new(bytes.Buffer)

		optimisticServer.GetPendingPartialWithdrawals(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		var resp structs.GetPendingPartialWithdrawalsResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		require.Equal(t, true, resp.ExecutionOptimistic)
	})

	t.Run("finalized node", func(t *testing.T) {
		blockRoot, err := st.LatestBlockHeader().HashTreeRoot()
		require.NoError(t, err)

		finalizedChainService := &chainMock.ChainService{
			Optimistic:     false,
			FinalizedRoots: map[[32]byte]bool{blockRoot: true},
		}
		finalizedServer := &Server{
			Stater:                server.Stater,
			OptimisticModeFetcher: finalizedChainService,
			FinalizationFetcher:   finalizedChainService,
		}

		req := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v1/beacon/states/{state_id}/pending_partial_withdrawals", nil)
		req.SetPathValue("state_id", "head")
		rec := httptest.NewRecorder()
		rec.Body = new(bytes.Buffer)

		finalizedServer.GetPendingPartialWithdrawals(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		var resp structs.GetPendingPartialWithdrawalsResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		require.Equal(t, true, resp.Finalized)
	})
}

func TestGetProposerLookahead(t *testing.T) {
	numValidators := 50
	// Create a Fulu state with proposer lookahead data
	st, _ := util.DeterministicGenesisStateFulu(t, uint64(numValidators))
	lookaheadSize := int(params.BeaconConfig().MinSeedLookahead+1) * int(params.BeaconConfig().SlotsPerEpoch)
	lookahead := make([]primitives.ValidatorIndex, lookaheadSize)
	for i := 0; i < lookaheadSize; i++ {
		lookahead[i] = primitives.ValidatorIndex(i % numValidators) // Cycle through validators
	}

	require.NoError(t, st.SetProposerLookahead(lookahead))

	chainService := &chainMock.ChainService{
		Optimistic:     false,
		FinalizedRoots: map[[32]byte]bool{},
	}
	server := &Server{
		Stater: &testutil.MockStater{
			BeaconState: st,
		},
		OptimisticModeFetcher: chainService,
		FinalizationFetcher:   chainService,
	}

	t.Run("json response", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v1/beacon/states/{state_id}/proposer_lookahead", nil)
		req.SetPathValue("state_id", "head")
		rec := httptest.NewRecorder()
		rec.Body = new(bytes.Buffer)

		server.GetProposerLookahead(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		require.Equal(t, "fulu", rec.Header().Get(api.VersionHeader))

		var resp structs.GetProposerLookaheadResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

		expectedVersion := version.String(st.Version())
		require.Equal(t, expectedVersion, resp.Version)
		require.Equal(t, false, resp.ExecutionOptimistic)
		require.Equal(t, false, resp.Finalized)

		// Verify the data
		require.Equal(t, lookaheadSize, len(resp.Data))
		for i := 0; i < lookaheadSize; i++ {
			expectedIdx := strconv.FormatUint(uint64(i%numValidators), 10)
			require.Equal(t, expectedIdx, resp.Data[i])
		}
	})

	t.Run("ssz response", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v1/beacon/states/{state_id}/proposer_lookahead", nil)
		req.Header.Set("Accept", "application/octet-stream")
		req.SetPathValue("state_id", "head")
		rec := httptest.NewRecorder()
		rec.Body = new(bytes.Buffer)

		server.GetProposerLookahead(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		require.Equal(t, "fulu", rec.Header().Get(api.VersionHeader))
		responseBytes := rec.Body.Bytes()
		validatorIndexSize := (*primitives.ValidatorIndex)(nil).SizeSSZ()
		require.Equal(t, len(responseBytes), validatorIndexSize*lookaheadSize)

		recoveredIndices := make([]primitives.ValidatorIndex, lookaheadSize)
		for i := 0; i < lookaheadSize; i++ {
			start := i * validatorIndexSize
			end := start + validatorIndexSize

			idx := ssz.UnmarshallUint64(responseBytes[start:end])
			recoveredIndices[i] = primitives.ValidatorIndex(idx)
		}
		require.DeepEqual(t, lookahead, recoveredIndices)
	})

	t.Run("pre fulu state", func(t *testing.T) {
		preEplusSt, _ := util.DeterministicGenesisStateElectra(t, 1)
		preFuluServer := &Server{
			Stater: &testutil.MockStater{
				BeaconState: preEplusSt,
			},
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
		}

		// Test JSON request
		req := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v1/beacon/states/{state_id}/proposer_lookahead", nil)
		req.SetPathValue("state_id", "head")
		rec := httptest.NewRecorder()
		rec.Body = new(bytes.Buffer)

		preFuluServer.GetProposerLookahead(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code)

		var errResp struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
		require.Equal(t, "state_id is prior to fulu", errResp.Message)

		// Test SSZ request
		sszReq := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v1/beacon/states/{state_id}/proposer_lookahead", nil)
		sszReq.Header.Set("Accept", "application/octet-stream")
		sszReq.SetPathValue("state_id", "head")
		sszRec := httptest.NewRecorder()
		sszRec.Body = new(bytes.Buffer)

		preFuluServer.GetProposerLookahead(sszRec, sszReq)
		require.Equal(t, http.StatusBadRequest, sszRec.Code)

		var sszErrResp struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		require.NoError(t, json.Unmarshal(sszRec.Body.Bytes(), &sszErrResp))
		require.Equal(t, "state_id is prior to fulu", sszErrResp.Message)
	})

	t.Run("missing state_id parameter", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v1/beacon/states/{state_id}/proposer_lookahead", nil)
		rec := httptest.NewRecorder()
		rec.Body = new(bytes.Buffer)

		server.GetProposerLookahead(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code)

		var errResp struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
		require.Equal(t, "state_id is required in URL params", errResp.Message)
	})

	t.Run("optimistic node", func(t *testing.T) {
		optimisticChainService := &chainMock.ChainService{
			Optimistic:     true,
			FinalizedRoots: map[[32]byte]bool{},
		}
		optimisticServer := &Server{
			Stater:                server.Stater,
			OptimisticModeFetcher: optimisticChainService,
			FinalizationFetcher:   optimisticChainService,
		}

		req := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v1/beacon/states/{state_id}/proposer_lookahead", nil)
		req.SetPathValue("state_id", "head")
		rec := httptest.NewRecorder()
		rec.Body = new(bytes.Buffer)

		optimisticServer.GetProposerLookahead(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		var resp structs.GetProposerLookaheadResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		require.Equal(t, true, resp.ExecutionOptimistic)
	})

	t.Run("finalized node", func(t *testing.T) {
		blockRoot, err := st.LatestBlockHeader().HashTreeRoot()
		require.NoError(t, err)

		finalizedChainService := &chainMock.ChainService{
			Optimistic:     false,
			FinalizedRoots: map[[32]byte]bool{blockRoot: true},
		}
		finalizedServer := &Server{
			Stater:                server.Stater,
			OptimisticModeFetcher: finalizedChainService,
			FinalizationFetcher:   finalizedChainService,
		}

		req := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v1/beacon/states/{state_id}/proposer_lookahead", nil)
		req.SetPathValue("state_id", "head")
		rec := httptest.NewRecorder()
		rec.Body = new(bytes.Buffer)

		finalizedServer.GetProposerLookahead(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		var resp structs.GetProposerLookaheadResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		require.Equal(t, true, resp.Finalized)
	})
}
