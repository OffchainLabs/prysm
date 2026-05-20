package debug

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	blockchainmock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	dbtest "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	doublylinkedtree "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/doubly-linked-tree"
	forkchoicetypes "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/types"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/core"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/testutil"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func TestGetBeaconStateV2(t *testing.T) {
	ctx := t.Context()
	db := dbtest.SetupDB(t)

	t.Run("phase0", func(t *testing.T) {
		fakeState, err := util.NewBeaconState()
		require.NoError(t, err)
		require.NoError(t, fakeState.SetSlot(123))
		chainService := &blockchainmock.ChainService{}
		s := &Server{
			Stater: &testutil.MockStater{
				BeaconState: fakeState,
			},
			HeadFetcher:           chainService,
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
		}

		request := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v2/debug/beacon/states/{state_id}", nil)
		request.SetPathValue("state_id", "head")
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetBeaconStateV2(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetBeaconStateV2Response{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, version.String(version.Phase0), resp.Version)
		st := &structs.BeaconState{}
		require.NoError(t, json.Unmarshal(resp.Data, st))
		assert.Equal(t, "123", st.Slot)
	})
	t.Run("Altair", func(t *testing.T) {
		fakeState, err := util.NewBeaconStateAltair()
		require.NoError(t, err)
		require.NoError(t, fakeState.SetSlot(123))
		chainService := &blockchainmock.ChainService{}
		s := &Server{
			Stater: &testutil.MockStater{
				BeaconState: fakeState,
			},
			HeadFetcher:           chainService,
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
		}

		request := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v2/debug/beacon/states/{state_id}", nil)
		request.SetPathValue("state_id", "head")
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetBeaconStateV2(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetBeaconStateV2Response{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, version.String(version.Altair), resp.Version)
		st := &structs.BeaconStateAltair{}
		require.NoError(t, json.Unmarshal(resp.Data, st))
		assert.Equal(t, "123", st.Slot)
	})
	t.Run("Bellatrix", func(t *testing.T) {
		fakeState, err := util.NewBeaconStateBellatrix()
		require.NoError(t, err)
		require.NoError(t, fakeState.SetSlot(123))
		chainService := &blockchainmock.ChainService{}
		s := &Server{
			Stater: &testutil.MockStater{
				BeaconState: fakeState,
			},
			HeadFetcher:           chainService,
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
		}

		request := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v2/debug/beacon/states/{state_id}", nil)
		request.SetPathValue("state_id", "head")
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetBeaconStateV2(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetBeaconStateV2Response{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, version.String(version.Bellatrix), resp.Version)
		st := &structs.BeaconStateBellatrix{}
		require.NoError(t, json.Unmarshal(resp.Data, st))
		assert.Equal(t, "123", st.Slot)
	})
	t.Run("Capella", func(t *testing.T) {
		fakeState, err := util.NewBeaconStateCapella()
		require.NoError(t, err)
		require.NoError(t, fakeState.SetSlot(123))
		chainService := &blockchainmock.ChainService{}
		s := &Server{
			Stater: &testutil.MockStater{
				BeaconState: fakeState,
			},
			HeadFetcher:           chainService,
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
		}

		request := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v2/debug/beacon/states/{state_id}", nil)
		request.SetPathValue("state_id", "head")
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetBeaconStateV2(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetBeaconStateV2Response{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, version.String(version.Capella), resp.Version)
		st := &structs.BeaconStateCapella{}
		require.NoError(t, json.Unmarshal(resp.Data, st))
		assert.Equal(t, "123", st.Slot)
	})
	t.Run("Deneb", func(t *testing.T) {
		fakeState, err := util.NewBeaconStateDeneb()
		require.NoError(t, err)
		require.NoError(t, fakeState.SetSlot(123))
		chainService := &blockchainmock.ChainService{}
		s := &Server{
			Stater: &testutil.MockStater{
				BeaconState: fakeState,
			},
			HeadFetcher:           chainService,
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
		}

		request := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v2/debug/beacon/states/{state_id}", nil)
		request.SetPathValue("state_id", "head")
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetBeaconStateV2(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetBeaconStateV2Response{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, version.String(version.Deneb), resp.Version)
		st := &structs.BeaconStateDeneb{}
		require.NoError(t, json.Unmarshal(resp.Data, st))
		assert.Equal(t, "123", st.Slot)
	})
	t.Run("Electra", func(t *testing.T) {
		fakeState, err := util.NewBeaconStateElectra()
		require.NoError(t, err)
		require.NoError(t, fakeState.SetSlot(123))
		chainService := &blockchainmock.ChainService{}
		s := &Server{
			Stater: &testutil.MockStater{
				BeaconState: fakeState,
			},
			HeadFetcher:           chainService,
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
		}

		request := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v2/debug/beacon/states/{state_id}", nil)
		request.SetPathValue("state_id", "head")
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetBeaconStateV2(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetBeaconStateV2Response{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, version.String(version.Electra), resp.Version)
		st := &structs.BeaconStateElectra{}
		require.NoError(t, json.Unmarshal(resp.Data, st))
		assert.Equal(t, "123", st.Slot)
	})
	t.Run("Fulu", func(t *testing.T) {
		fakeState, err := util.NewBeaconStateFulu()
		require.NoError(t, err)
		require.NoError(t, fakeState.SetSlot(123))
		chainService := &blockchainmock.ChainService{}
		s := &Server{
			Stater: &testutil.MockStater{
				BeaconState: fakeState,
			},
			HeadFetcher:           chainService,
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
		}

		request := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v2/debug/beacon/states/{state_id}", nil)
		request.SetPathValue("state_id", "head")
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetBeaconStateV2(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetBeaconStateV2Response{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, version.String(version.Fulu), resp.Version)
		st := &structs.BeaconStateFulu{}
		require.NoError(t, json.Unmarshal(resp.Data, st))
		assert.Equal(t, "123", st.Slot)
		assert.Equal(t, int(params.BeaconConfig().MinSeedLookahead+1)*int(params.BeaconConfig().SlotsPerEpoch), len(st.ProposerLookahead))
	})
	t.Run("Gloas", func(t *testing.T) {
		fakeState, err := util.NewBeaconStateGloas()
		require.NoError(t, err)
		require.NoError(t, fakeState.SetSlot(123))
		chainService := &blockchainmock.ChainService{}
		s := &Server{
			Stater: &testutil.MockStater{
				BeaconState: fakeState,
			},
			HeadFetcher:           chainService,
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
		}

		request := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v2/debug/beacon/states/{state_id}", nil)
		request.SetPathValue("state_id", "head")
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetBeaconStateV2(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetBeaconStateV2Response{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, version.String(version.Gloas), resp.Version)
		st := &structs.BeaconStateGloas{}
		require.NoError(t, json.Unmarshal(resp.Data, st))
		assert.Equal(t, "123", st.Slot)
		assert.Equal(t, int(params.BeaconConfig().MinSeedLookahead+1)*int(params.BeaconConfig().SlotsPerEpoch), len(st.ProposerLookahead))
	})
	t.Run("execution optimistic", func(t *testing.T) {
		parentRoot := [32]byte{'a'}
		blk := util.NewBeaconBlock()
		blk.Block.ParentRoot = parentRoot[:]
		root, err := blk.Block.HashTreeRoot()
		require.NoError(t, err)
		util.SaveBlock(t, ctx, db, blk)
		require.NoError(t, db.SaveGenesisBlockRoot(ctx, root))

		fakeState, err := util.NewBeaconStateBellatrix()
		require.NoError(t, err)
		chainService := &blockchainmock.ChainService{Optimistic: true}
		s := &Server{
			Stater: &testutil.MockStater{
				BeaconState: fakeState,
			},
			HeadFetcher:           chainService,
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
			BeaconDB:              db,
		}

		request := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v2/debug/beacon/states/{state_id}", nil)
		request.SetPathValue("state_id", "head")
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetBeaconStateV2(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetBeaconStateV2Response{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, true, resp.ExecutionOptimistic)
	})
	t.Run("finalized", func(t *testing.T) {
		parentRoot := [32]byte{'a'}
		blk := util.NewBeaconBlock()
		blk.Block.ParentRoot = parentRoot[:]
		root, err := blk.Block.HashTreeRoot()
		require.NoError(t, err)
		util.SaveBlock(t, ctx, db, blk)
		require.NoError(t, db.SaveGenesisBlockRoot(ctx, root))

		fakeState, err := util.NewBeaconStateBellatrix()
		require.NoError(t, err)
		headerRoot, err := fakeState.LatestBlockHeader().HashTreeRoot()
		require.NoError(t, err)
		chainService := &blockchainmock.ChainService{
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
			BeaconDB:              db,
		}

		request := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v2/debug/beacon/states/{state_id}", nil)
		request.SetPathValue("state_id", "head")
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetBeaconStateV2(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetBeaconStateV2Response{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, true, resp.Finalized)
	})
}

func TestGetBeaconStateSSZV2(t *testing.T) {
	t.Run("Phase 0", func(t *testing.T) {
		fakeState, err := util.NewBeaconState()
		require.NoError(t, err)
		require.NoError(t, fakeState.SetSlot(123))

		s := &Server{
			Stater: &testutil.MockStater{
				BeaconState: fakeState,
			},
		}

		request := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v2/debug/beacon/states/{state_id}", nil)
		request.SetPathValue("state_id", "head")
		request.Header.Set("Accept", api.OctetStreamMediaType)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetBeaconStateV2(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		assert.Equal(t, version.String(version.Phase0), writer.Header().Get(api.VersionHeader))
		sszExpected, err := fakeState.MarshalSSZ()
		require.NoError(t, err)
		assert.DeepEqual(t, sszExpected, writer.Body.Bytes())
	})
	t.Run("Altair", func(t *testing.T) {
		fakeState, err := util.NewBeaconStateAltair()
		require.NoError(t, err)
		require.NoError(t, fakeState.SetSlot(123))

		s := &Server{
			Stater: &testutil.MockStater{
				BeaconState: fakeState,
			},
		}

		request := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v2/debug/beacon/states/{state_id}", nil)
		request.SetPathValue("state_id", "head")
		request.Header.Set("Accept", api.OctetStreamMediaType)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetBeaconStateV2(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		assert.Equal(t, version.String(version.Altair), writer.Header().Get(api.VersionHeader))
		sszExpected, err := fakeState.MarshalSSZ()
		require.NoError(t, err)
		assert.DeepEqual(t, sszExpected, writer.Body.Bytes())
	})
	t.Run("Bellatrix", func(t *testing.T) {
		fakeState, err := util.NewBeaconStateBellatrix()
		require.NoError(t, err)
		require.NoError(t, fakeState.SetSlot(123))

		s := &Server{
			Stater: &testutil.MockStater{
				BeaconState: fakeState,
			},
		}

		request := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v2/debug/beacon/states/{state_id}", nil)
		request.SetPathValue("state_id", "head")
		request.Header.Set("Accept", api.OctetStreamMediaType)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetBeaconStateV2(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		assert.Equal(t, version.String(version.Bellatrix), writer.Header().Get(api.VersionHeader))
		sszExpected, err := fakeState.MarshalSSZ()
		require.NoError(t, err)
		assert.DeepEqual(t, sszExpected, writer.Body.Bytes())
	})
	t.Run("Capella", func(t *testing.T) {
		fakeState, err := util.NewBeaconStateCapella()
		require.NoError(t, err)
		require.NoError(t, fakeState.SetSlot(123))

		s := &Server{
			Stater: &testutil.MockStater{
				BeaconState: fakeState,
			},
		}

		request := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v2/debug/beacon/states/{state_id}", nil)
		request.SetPathValue("state_id", "head")
		request.Header.Set("Accept", api.OctetStreamMediaType)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetBeaconStateV2(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		assert.Equal(t, version.String(version.Capella), writer.Header().Get(api.VersionHeader))
		sszExpected, err := fakeState.MarshalSSZ()
		require.NoError(t, err)
		assert.DeepEqual(t, sszExpected, writer.Body.Bytes())
	})
	t.Run("Deneb", func(t *testing.T) {
		fakeState, err := util.NewBeaconStateDeneb()
		require.NoError(t, err)
		require.NoError(t, fakeState.SetSlot(123))

		s := &Server{
			Stater: &testutil.MockStater{
				BeaconState: fakeState,
			},
		}

		request := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v2/debug/beacon/states/{state_id}", nil)
		request.SetPathValue("state_id", "head")
		request.Header.Set("Accept", api.OctetStreamMediaType)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetBeaconStateV2(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		assert.Equal(t, version.String(version.Deneb), writer.Header().Get(api.VersionHeader))
		sszExpected, err := fakeState.MarshalSSZ()
		require.NoError(t, err)
		assert.DeepEqual(t, sszExpected, writer.Body.Bytes())
	})
	t.Run("Electra", func(t *testing.T) {
		fakeState, err := util.NewBeaconStateElectra()
		require.NoError(t, err)
		require.NoError(t, fakeState.SetSlot(123))

		s := &Server{
			Stater: &testutil.MockStater{
				BeaconState: fakeState,
			},
		}

		request := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v2/debug/beacon/states/{state_id}", nil)
		request.SetPathValue("state_id", "head")
		request.Header.Set("Accept", api.OctetStreamMediaType)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetBeaconStateV2(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		assert.Equal(t, version.String(version.Electra), writer.Header().Get(api.VersionHeader))
		sszExpected, err := fakeState.MarshalSSZ()
		require.NoError(t, err)
		assert.DeepEqual(t, sszExpected, writer.Body.Bytes())
	})
	t.Run("Fulu", func(t *testing.T) {
		fakeState, err := util.NewBeaconStateFulu()
		require.NoError(t, err)
		require.NoError(t, fakeState.SetSlot(123))

		s := &Server{
			Stater: &testutil.MockStater{
				BeaconState: fakeState,
			},
		}

		request := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v2/debug/beacon/states/{state_id}", nil)
		request.SetPathValue("state_id", "head")
		request.Header.Set("Accept", api.OctetStreamMediaType)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetBeaconStateV2(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		assert.Equal(t, version.String(version.Fulu), writer.Header().Get(api.VersionHeader))
		sszExpected, err := fakeState.MarshalSSZ()
		require.NoError(t, err)
		assert.DeepEqual(t, sszExpected, writer.Body.Bytes())
	})
	t.Run("Gloas", func(t *testing.T) {
		fakeState, err := util.NewBeaconStateGloas()
		require.NoError(t, err)
		require.NoError(t, fakeState.SetSlot(123))

		s := &Server{
			Stater: &testutil.MockStater{
				BeaconState: fakeState,
			},
		}

		request := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v2/debug/beacon/states/{state_id}", nil)
		request.SetPathValue("state_id", "head")
		request.Header.Set("Accept", api.OctetStreamMediaType)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetBeaconStateV2(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		assert.Equal(t, version.String(version.Gloas), writer.Header().Get(api.VersionHeader))
		sszExpected, err := fakeState.MarshalSSZ()
		require.NoError(t, err)
		assert.DeepEqual(t, sszExpected, writer.Body.Bytes())
	})
}

func TestGetForkChoiceHeadsV2(t *testing.T) {
	expectedSlotsAndRoots := []struct {
		Slot string
		Root string
	}{{
		Slot: "0",
		Root: hexutil.Encode(bytesutil.PadTo([]byte("foo"), 32)),
	}, {
		Slot: "1",
		Root: hexutil.Encode(bytesutil.PadTo([]byte("bar"), 32)),
	}}

	chainService := &blockchainmock.ChainService{}
	s := &Server{
		HeadFetcher:           chainService,
		OptimisticModeFetcher: chainService,
	}

	request := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v2/debug/beacon/heads", nil)
	writer := httptest.NewRecorder()
	writer.Body = &bytes.Buffer{}

	s.GetForkChoiceHeadsV2(writer, request)
	require.Equal(t, http.StatusOK, writer.Code)
	resp := &structs.GetForkChoiceHeadsV2Response{}
	require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
	assert.Equal(t, 2, len(resp.Data))
	for _, sr := range expectedSlotsAndRoots {
		found := false
		for _, h := range resp.Data {
			if h.Slot == sr.Slot {
				found = true
				assert.Equal(t, sr.Root, h.Root)
			}
			assert.Equal(t, false, h.ExecutionOptimistic)
		}
		assert.Equal(t, true, found, "Expected head not found")
	}

	t.Run("optimistic head", func(t *testing.T) {
		chainService := &blockchainmock.ChainService{
			Optimistic:      true,
			OptimisticRoots: make(map[[32]byte]bool),
		}
		for _, sr := range expectedSlotsAndRoots {
			b, err := hexutil.Decode(sr.Root)
			require.NoError(t, err)
			chainService.OptimisticRoots[bytesutil.ToBytes32(b)] = true
		}
		s := &Server{
			HeadFetcher:           chainService,
			OptimisticModeFetcher: chainService,
		}

		request := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v2/debug/beacon/heads", nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetForkChoiceHeadsV2(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetForkChoiceHeadsV2Response{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, 2, len(resp.Data))
		for _, sr := range expectedSlotsAndRoots {
			found := false
			for _, h := range resp.Data {
				if h.Slot == sr.Slot {
					found = true
					assert.Equal(t, sr.Root, h.Root)
				}
				assert.Equal(t, true, h.ExecutionOptimistic)
			}
			assert.Equal(t, true, found, "Expected head not found")
		}
	})
}

func TestGetForkChoice(t *testing.T) {
	store := doublylinkedtree.New()
	fRoot := [32]byte{'a'}
	fc := &forkchoicetypes.Checkpoint{Epoch: 2, Root: fRoot}
	require.NoError(t, store.UpdateFinalizedCheckpoint(fc))
	s := &Server{ForkchoiceFetcher: &blockchainmock.ChainService{ForkChoiceStore: store}}

	request := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v1/debug/fork_choice", nil)
	writer := httptest.NewRecorder()
	writer.Body = &bytes.Buffer{}

	s.GetForkChoice(writer, request)
	require.Equal(t, http.StatusOK, writer.Code)
	resp := &structs.GetForkChoiceDumpResponse{}
	require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
	require.Equal(t, "2", resp.FinalizedCheckpoint.Epoch)
}

func TestDataColumnSidecars(t *testing.T) {
	t.Run("Fulu fork not configured", func(t *testing.T) {
		// Save the original config
		originalConfig := params.BeaconConfig()
		defer func() { params.OverrideBeaconConfig(originalConfig) }()

		// Set Fulu fork epoch to MaxUint64 (unconfigured)
		config := params.BeaconConfig().Copy()
		config.FuluForkEpoch = math.MaxUint64
		params.OverrideBeaconConfig(config)

		chainService := &blockchainmock.ChainService{}

		// Create a mock blocker to avoid nil pointer
		mockBlocker := &testutil.MockBlocker{}

		s := &Server{
			GenesisTimeFetcher: chainService,
			Blocker:            mockBlocker,
		}

		request := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v1/debug/beacon/data_column_sidecars/head", nil)
		request.SetPathValue("block_id", "head")
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.DataColumnSidecars(writer, request)
		require.Equal(t, http.StatusBadRequest, writer.Code)
		assert.StringContains(t, "Data columns are not supported - Fulu fork not configured", writer.Body.String())
	})

	t.Run("Before Fulu fork", func(t *testing.T) {
		// Save the original config
		originalConfig := params.BeaconConfig()
		defer func() { params.OverrideBeaconConfig(originalConfig) }()

		// Set Fulu fork epoch to 100
		config := params.BeaconConfig().Copy()
		config.FuluForkEpoch = 100
		params.OverrideBeaconConfig(config)

		chainService := &blockchainmock.ChainService{}
		currentSlot := primitives.Slot(0) // Current slot 0 (epoch 0, before epoch 100)
		chainService.Slot = &currentSlot

		// Create a mock blocker to avoid nil pointer
		mockBlocker := &testutil.MockBlocker{
			DataColumnsFunc: func(ctx context.Context, id string, indices []int) ([]blocks.VerifiedRODataColumn, *core.RpcError) {
				return nil, &core.RpcError{Err: errors.New("before Fulu fork"), Reason: core.BadRequest}
			},
		}

		s := &Server{
			GenesisTimeFetcher: chainService,
			Blocker:            mockBlocker,
		}

		request := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v1/debug/beacon/data_column_sidecars/head", nil)
		request.SetPathValue("block_id", "head")
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.DataColumnSidecars(writer, request)
		require.Equal(t, http.StatusBadRequest, writer.Code)
		assert.StringContains(t, "Data columns are not supported - before Fulu fork", writer.Body.String())
	})

	t.Run("Invalid indices", func(t *testing.T) {
		// Save the original config
		originalConfig := params.BeaconConfig()
		defer func() { params.OverrideBeaconConfig(originalConfig) }()

		// Set Fulu fork epoch to 0 (already activated)
		config := params.BeaconConfig().Copy()
		config.FuluForkEpoch = 0
		params.OverrideBeaconConfig(config)

		chainService := &blockchainmock.ChainService{}
		currentSlot := primitives.Slot(0) // Current slot 0 (epoch 0, at fork)
		chainService.Slot = &currentSlot

		// Create a mock blocker to avoid nil pointer
		mockBlocker := &testutil.MockBlocker{
			DataColumnsFunc: func(ctx context.Context, id string, indices []int) ([]blocks.VerifiedRODataColumn, *core.RpcError) {
				return nil, &core.RpcError{Err: errors.New("invalid index"), Reason: core.BadRequest}
			},
		}

		s := &Server{
			GenesisTimeFetcher: chainService,
			Blocker:            mockBlocker,
		}

		// Test with invalid index (out of range)
		request := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v1/debug/beacon/data_column_sidecars/head?indices=9999", nil)
		request.SetPathValue("block_id", "head")
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.DataColumnSidecars(writer, request)
		require.Equal(t, http.StatusBadRequest, writer.Code)
		assert.StringContains(t, "requested data column indices [9999] are invalid", writer.Body.String())
	})

	t.Run("Block not found", func(t *testing.T) {
		// Save the original config
		originalConfig := params.BeaconConfig()
		defer func() { params.OverrideBeaconConfig(originalConfig) }()

		// Set Fulu fork epoch to 0 (already activated)
		config := params.BeaconConfig().Copy()
		config.FuluForkEpoch = 0
		params.OverrideBeaconConfig(config)

		chainService := &blockchainmock.ChainService{}
		currentSlot := primitives.Slot(0) // Current slot 0
		chainService.Slot = &currentSlot

		// Create a mock blocker that returns block not found
		mockBlocker := &testutil.MockBlocker{
			DataColumnsFunc: func(ctx context.Context, id string, indices []int) ([]blocks.VerifiedRODataColumn, *core.RpcError) {
				return nil, &core.RpcError{Err: errors.New("block not found"), Reason: core.NotFound}
			},
			BlockToReturn: nil, // Block not found
		}

		s := &Server{
			GenesisTimeFetcher: chainService,
			Blocker:            mockBlocker,
		}

		request := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v1/debug/beacon/data_column_sidecars/head", nil)
		request.SetPathValue("block_id", "head")
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.DataColumnSidecars(writer, request)
		require.Equal(t, http.StatusNotFound, writer.Code)
	})

	t.Run("Empty data columns", func(t *testing.T) {
		// Save the original config
		originalConfig := params.BeaconConfig()
		defer func() { params.OverrideBeaconConfig(originalConfig) }()

		// Set Fulu fork epoch to 0
		config := params.BeaconConfig().Copy()
		config.FuluForkEpoch = 0
		params.OverrideBeaconConfig(config)

		// Create a simple test block
		signedTestBlock := util.NewBeaconBlock()
		roBlock, err := blocks.NewSignedBeaconBlock(signedTestBlock)
		require.NoError(t, err)

		chainService := &blockchainmock.ChainService{}
		currentSlot := primitives.Slot(0) // Current slot 0
		chainService.Slot = &currentSlot
		chainService.OptimisticRoots = make(map[[32]byte]bool)
		chainService.FinalizedRoots = make(map[[32]byte]bool)

		mockBlocker := &testutil.MockBlocker{
			DataColumnsFunc: func(ctx context.Context, id string, indices []int) ([]blocks.VerifiedRODataColumn, *core.RpcError) {
				return []blocks.VerifiedRODataColumn{}, nil // Empty data columns
			},
			BlockToReturn: roBlock,
		}

		s := &Server{
			GenesisTimeFetcher:    chainService,
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
			Blocker:               mockBlocker,
		}

		request := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v1/debug/beacon/data_column_sidecars/head", nil)
		request.SetPathValue("block_id", "head")
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.DataColumnSidecars(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)

		resp := &structs.GetDebugDataColumnSidecarsResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.Equal(t, 0, len(resp.Data))
	})

	t.Run("Gloas block - returns Gloas-shaped sidecars", func(t *testing.T) {
		originalConfig := params.BeaconConfig()
		defer func() { params.OverrideBeaconConfig(originalConfig) }()

		config := params.BeaconConfig().Copy()
		config.FuluForkEpoch = 0
		config.GloasForkEpoch = 0
		params.OverrideBeaconConfig(config)

		signedGloasBlock := util.NewBeaconBlockGloas()
		roBlock, err := blocks.NewSignedBeaconBlock(signedGloasBlock)
		require.NoError(t, err)
		require.Equal(t, version.Gloas, roBlock.Version())

		blockRoot := bytesutil.ToBytes32([]byte{0xAB})
		gloasDC := &ethpb.DataColumnSidecarGloas{
			Index:           7,
			Column:          [][]byte{{0x10}, {0x11}},
			KzgProofs:       [][]byte{bytesutil.PadTo([]byte{0xAA}, 48), bytesutil.PadTo([]byte{0xBB}, 48)},
			Slot:            123,
			BeaconBlockRoot: blockRoot[:],
		}
		roCol, err := blocks.NewRODataColumnGloasWithRoot(gloasDC, blockRoot)
		require.NoError(t, err)
		verified := blocks.NewVerifiedRODataColumn(roCol)

		chainService := &blockchainmock.ChainService{}
		currentSlot := primitives.Slot(0)
		chainService.Slot = &currentSlot
		chainService.OptimisticRoots = make(map[[32]byte]bool)
		chainService.FinalizedRoots = make(map[[32]byte]bool)

		mockBlocker := &testutil.MockBlocker{
			DataColumnsFunc: func(ctx context.Context, id string, indices []int) ([]blocks.VerifiedRODataColumn, *core.RpcError) {
				return []blocks.VerifiedRODataColumn{verified}, nil
			},
			BlockToReturn: roBlock,
		}

		s := &Server{
			GenesisTimeFetcher:    chainService,
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
			Blocker:               mockBlocker,
		}

		request := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v1/debug/beacon/data_column_sidecars/head", nil)
		request.SetPathValue("block_id", "head")
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.DataColumnSidecars(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		require.Equal(t, version.String(version.Gloas), writer.Header().Get(api.VersionHeader))

		resp := &structs.GetDebugDataColumnSidecarsResponseGloas{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.Equal(t, version.String(version.Gloas), resp.Version)
		require.Equal(t, 1, len(resp.Data))
		require.Equal(t, "7", resp.Data[0].Index)
		require.Equal(t, "123", resp.Data[0].Slot)
		require.Equal(t, hexutil.Encode(blockRoot[:]), resp.Data[0].BeaconBlockRoot)
		require.Equal(t, 2, len(resp.Data[0].Column))
		require.Equal(t, 2, len(resp.Data[0].KzgProofs))

		// Fulu-only fields must not appear on the wire.
		body := writer.Body.String()
		assert.StringNotContains(t, "kzg_commitments", body)
		assert.StringNotContains(t, "signed_block_header", body)
		assert.StringNotContains(t, "kzg_commitments_inclusion_proof", body)
	})
}

// TestBuildDataColumnSidecarsJsonResponse_GloasFails pins the broken-before behavior:
// the original (Fulu) JSON builder cannot serialize a Gloas-backed RODataColumn because
// it reads KzgCommitments/SignedBlockHeader/KzgCommitmentsInclusionProof — all Fulu-only.
func TestBuildDataColumnSidecarsJsonResponse_GloasFails(t *testing.T) {
	blockRoot := bytesutil.ToBytes32([]byte{0xAA})
	gloasDC := &ethpb.DataColumnSidecarGloas{
		Index:           3,
		Column:          [][]byte{{0x01}},
		KzgProofs:       [][]byte{bytesutil.PadTo([]byte{0x02}, 48)},
		Slot:            42,
		BeaconBlockRoot: blockRoot[:],
	}
	roCol, err := blocks.NewRODataColumnGloasWithRoot(gloasDC, blockRoot)
	require.NoError(t, err)
	verified := blocks.NewVerifiedRODataColumn(roCol)

	_, err = buildDataColumnSidecarsJsonResponse([]blocks.VerifiedRODataColumn{verified})
	require.NotNil(t, err)
	assert.StringContains(t, "fulu", err.Error())
}

func TestBuildDataColumnSidecarsJsonResponseGloas(t *testing.T) {
	blockRoot := bytesutil.ToBytes32([]byte{0xCD})
	gloasDC := &ethpb.DataColumnSidecarGloas{
		Index:           11,
		Column:          [][]byte{{0xDE, 0xAD}, {0xBE, 0xEF}},
		KzgProofs:       [][]byte{bytesutil.PadTo([]byte{0x55}, 48), bytesutil.PadTo([]byte{0x66}, 48)},
		Slot:            777,
		BeaconBlockRoot: blockRoot[:],
	}
	roCol, err := blocks.NewRODataColumnGloasWithRoot(gloasDC, blockRoot)
	require.NoError(t, err)
	verified := blocks.NewVerifiedRODataColumn(roCol)

	sidecars, err := buildDataColumnSidecarsJsonResponseGloas([]blocks.VerifiedRODataColumn{verified})
	require.NoError(t, err)
	require.Equal(t, 1, len(sidecars))
	require.Equal(t, "11", sidecars[0].Index)
	require.Equal(t, "777", sidecars[0].Slot)
	require.Equal(t, hexutil.Encode(blockRoot[:]), sidecars[0].BeaconBlockRoot)
	require.Equal(t, hexutil.Encode([]byte{0xDE, 0xAD}), sidecars[0].Column[0])
	require.Equal(t, hexutil.Encode([]byte{0xBE, 0xEF}), sidecars[0].Column[1])
	require.Equal(t, hexutil.Encode(bytesutil.PadTo([]byte{0x55}, 48)), sidecars[0].KzgProofs[0])
	require.Equal(t, hexutil.Encode(bytesutil.PadTo([]byte{0x66}, 48)), sidecars[0].KzgProofs[1])

	bz, err := json.Marshal(sidecars[0])
	require.NoError(t, err)
	body := string(bz)
	assert.StringContains(t, `"index":"11"`, body)
	assert.StringContains(t, `"slot":"777"`, body)
	assert.StringContains(t, `"beacon_block_root":`, body)
	assert.StringNotContains(t, "kzg_commitments", body)
	assert.StringNotContains(t, "signed_block_header", body)
}

func TestParseDataColumnIndices(t *testing.T) {
	tests := []struct {
		name        string
		queryParams map[string][]string
		expected    []int
		expectError bool
	}{
		{
			name:        "no indices",
			queryParams: map[string][]string{},
			expected:    []int{},
			expectError: false,
		},
		{
			name:        "valid indices",
			queryParams: map[string][]string{"indices": {"0", "1", "127"}},
			expected:    []int{0, 1, 127},
			expectError: false,
		},
		{
			name:        "duplicate indices",
			queryParams: map[string][]string{"indices": {"0", "1", "0"}},
			expected:    []int{0, 1},
			expectError: false,
		},
		{
			name:        "invalid string index",
			queryParams: map[string][]string{"indices": {"abc"}},
			expected:    nil,
			expectError: true,
		},
		{
			name:        "negative index",
			queryParams: map[string][]string{"indices": {"-1"}},
			expected:    nil,
			expectError: true,
		},
		{
			name:        "index too large",
			queryParams: map[string][]string{"indices": {"128"}}, // 128 >= NumberOfColumns (128)
			expected:    nil,
			expectError: true,
		},
		{
			name:        "mixed valid and invalid",
			queryParams: map[string][]string{"indices": {"0", "abc", "1"}},
			expected:    nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := url.Parse("http://example.com/test")
			require.NoError(t, err)

			q := u.Query()
			for key, values := range tt.queryParams {
				for _, value := range values {
					q.Add(key, value)
				}
			}
			u.RawQuery = q.Encode()

			result, err := parseDataColumnIndices(u)

			if tt.expectError {
				assert.NotNil(t, err)
			} else {
				require.NoError(t, err)
				assert.DeepEqual(t, tt.expected, result)
			}
		})
	}
}

func TestBuildDataColumnSidecarsSSZResponse(t *testing.T) {
	t.Run("empty data columns", func(t *testing.T) {
		result, err := buildDataColumnSidecarsSSZResponse([]blocks.VerifiedRODataColumn{})
		require.NoError(t, err)
		require.DeepEqual(t, []byte{}, result)
	})

	t.Run("get SSZ size", func(t *testing.T) {
		size := (&ethpb.DataColumnSidecar{}).SizeSSZ()
		assert.Equal(t, true, size > 0)
	})
}
