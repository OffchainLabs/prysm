package blob

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v6/api"
	"github.com/OffchainLabs/prysm/v6/api/server/structs"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/kzg"
	mockChain "github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db/filesystem"
	testDB "github.com/OffchainLabs/prysm/v6/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/lookup"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/testutil"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/verification"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/network/httputil"
	eth "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func TestBlobs(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.FuluForkEpoch = cfg.ElectraForkEpoch + 4096*2
	params.OverrideBeaconConfig(cfg)
	es := util.SlotAtEpoch(t, cfg.ElectraForkEpoch)
	ds := util.SlotAtEpoch(t, cfg.DenebForkEpoch)

	db := testDB.SetupDB(t)
	denebBlock, blobs := util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, es, 4)
	require.NoError(t, db.SaveBlock(t.Context(), denebBlock))
	bs := filesystem.NewEphemeralBlobStorage(t)
	testSidecars := verification.FakeVerifySliceForTest(t, blobs)
	for i := range testSidecars {
		require.NoError(t, bs.Save(testSidecars[i]))
	}
	blockRoot := blobs[0].BlockRoot()

	mockChainService := &mockChain.ChainService{
		FinalizedRoots: map[[32]byte]bool{},
		Genesis:        time.Now().Add(-time.Duration(uint64(params.BeaconConfig().SlotsPerEpoch)*uint64(params.BeaconConfig().DenebForkEpoch)*params.BeaconConfig().SecondsPerSlot) * time.Second),
	}
	s := &Server{
		OptimisticModeFetcher: mockChainService,
		FinalizationFetcher:   mockChainService,
		TimeFetcher:           mockChainService,
	}

	t.Run("genesis", func(t *testing.T) {
		u := "http://foo.example/genesis"
		request := httptest.NewRequest("GET", u, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		s.Blocker = &lookup.BeaconDbBlocker{}
		s.Blobs(writer, request)

		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &httputil.DefaultJsonError{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
		assert.StringContains(t, "not supported for Phase 0 fork", e.Message)
	})
	t.Run("head", func(t *testing.T) {
		u := "http://foo.example/head"
		request := httptest.NewRequest("GET", u, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		s.Blocker = &lookup.BeaconDbBlocker{
			ChainInfoFetcher: &mockChain.ChainService{Root: blockRoot[:], Block: denebBlock},
			GenesisTimeFetcher: &testutil.MockGenesisTimeFetcher{
				Genesis: time.Now(),
			},
			BeaconDB:    db,
			BlobStorage: bs,
		}
		s.Blobs(writer, request)

		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.SidecarsResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.Equal(t, 4, len(resp.Data))
		sidecar := resp.Data[0]
		require.NotNil(t, sidecar)
		assert.Equal(t, "0", sidecar.Index)
		assert.Equal(t, hexutil.Encode(blobs[0].Blob), sidecar.Blob)
		assert.Equal(t, hexutil.Encode(blobs[0].KzgCommitment), sidecar.KzgCommitment)
		assert.Equal(t, hexutil.Encode(blobs[0].KzgProof), sidecar.KzgProof)
		sidecar = resp.Data[1]
		require.NotNil(t, sidecar)
		assert.Equal(t, "1", sidecar.Index)
		assert.Equal(t, hexutil.Encode(blobs[1].Blob), sidecar.Blob)
		assert.Equal(t, hexutil.Encode(blobs[1].KzgCommitment), sidecar.KzgCommitment)
		assert.Equal(t, hexutil.Encode(blobs[1].KzgProof), sidecar.KzgProof)
		sidecar = resp.Data[2]
		require.NotNil(t, sidecar)
		assert.Equal(t, "2", sidecar.Index)
		assert.Equal(t, hexutil.Encode(blobs[2].Blob), sidecar.Blob)
		assert.Equal(t, hexutil.Encode(blobs[2].KzgCommitment), sidecar.KzgCommitment)
		assert.Equal(t, hexutil.Encode(blobs[2].KzgProof), sidecar.KzgProof)
		sidecar = resp.Data[3]
		require.NotNil(t, sidecar)
		assert.Equal(t, "3", sidecar.Index)
		assert.Equal(t, hexutil.Encode(blobs[3].Blob), sidecar.Blob)
		assert.Equal(t, hexutil.Encode(blobs[3].KzgCommitment), sidecar.KzgCommitment)
		assert.Equal(t, hexutil.Encode(blobs[3].KzgProof), sidecar.KzgProof)

		require.Equal(t, "deneb", resp.Version)
		require.Equal(t, false, resp.ExecutionOptimistic)
		require.Equal(t, false, resp.Finalized)
	})
	t.Run("finalized", func(t *testing.T) {
		u := "http://foo.example/finalized"
		request := httptest.NewRequest("GET", u, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		s.Blocker = &lookup.BeaconDbBlocker{
			ChainInfoFetcher: &mockChain.ChainService{FinalizedCheckPoint: &eth.Checkpoint{Root: blockRoot[:]}, Block: denebBlock},
			GenesisTimeFetcher: &testutil.MockGenesisTimeFetcher{
				Genesis: time.Now(),
			},
			BeaconDB:    db,
			BlobStorage: bs,
		}
		s.Blobs(writer, request)

		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.SidecarsResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.Equal(t, 4, len(resp.Data))

		require.Equal(t, "deneb", resp.Version)
		require.Equal(t, false, resp.ExecutionOptimistic)
		require.Equal(t, false, resp.Finalized)
	})
	t.Run("root", func(t *testing.T) {
		u := "http://foo.example/" + hexutil.Encode(blockRoot[:])
		request := httptest.NewRequest("GET", u, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		s.Blocker = &lookup.BeaconDbBlocker{
			ChainInfoFetcher: &mockChain.ChainService{Block: denebBlock},
			BeaconDB:         db,
			GenesisTimeFetcher: &testutil.MockGenesisTimeFetcher{
				Genesis: time.Now(),
			},
			BlobStorage: bs,
		}
		s.Blobs(writer, request)

		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.SidecarsResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.Equal(t, 4, len(resp.Data))

		require.Equal(t, "deneb", resp.Version)
		require.Equal(t, false, resp.ExecutionOptimistic)
		require.Equal(t, false, resp.Finalized)
	})
	t.Run("slot", func(t *testing.T) {
		u := fmt.Sprintf("http://foo.example/%d", es)
		request := httptest.NewRequest("GET", u, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		s.Blocker = &lookup.BeaconDbBlocker{
			ChainInfoFetcher: &mockChain.ChainService{Block: denebBlock},
			BeaconDB:         db,
			GenesisTimeFetcher: &testutil.MockGenesisTimeFetcher{
				Genesis: time.Now(),
			},
			BlobStorage: bs,
		}
		s.Blobs(writer, request)

		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.SidecarsResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.Equal(t, 4, len(resp.Data))

		require.Equal(t, "deneb", resp.Version)
		require.Equal(t, false, resp.ExecutionOptimistic)
		require.Equal(t, false, resp.Finalized)
	})
	t.Run("slot not found", func(t *testing.T) {
		u := fmt.Sprintf("http://foo.example/%d", es-1)
		request := httptest.NewRequest("GET", u, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		s.Blocker = &lookup.BeaconDbBlocker{
			ChainInfoFetcher: &mockChain.ChainService{Block: denebBlock},
			BeaconDB:         db,
			GenesisTimeFetcher: &testutil.MockGenesisTimeFetcher{
				Genesis: time.Now(),
			},
			BlobStorage: bs,
		}
		s.Blobs(writer, request)

		assert.Equal(t, http.StatusNotFound, writer.Code)
	})
	t.Run("one blob only", func(t *testing.T) {
		u := fmt.Sprintf("http://foo.example/%d?indices=2", es)
		request := httptest.NewRequest("GET", u, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		s.Blocker = &lookup.BeaconDbBlocker{
			ChainInfoFetcher: &mockChain.ChainService{FinalizedCheckPoint: &eth.Checkpoint{Root: blockRoot[:]}, Block: denebBlock},
			GenesisTimeFetcher: &testutil.MockGenesisTimeFetcher{
				Genesis: time.Now(),
			},
			BeaconDB:    db,
			BlobStorage: bs,
		}
		s.Blobs(writer, request)

		assert.Equal(t, version.String(version.Deneb), writer.Header().Get(api.VersionHeader))
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.SidecarsResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.Equal(t, 1, len(resp.Data))
		sidecar := resp.Data[0]
		require.NotNil(t, sidecar)
		assert.Equal(t, "2", sidecar.Index)
		assert.Equal(t, hexutil.Encode(blobs[2].Blob), sidecar.Blob)
		assert.Equal(t, hexutil.Encode(blobs[2].KzgCommitment), sidecar.KzgCommitment)
		assert.Equal(t, hexutil.Encode(blobs[2].KzgProof), sidecar.KzgProof)

		require.Equal(t, "deneb", resp.Version)
		require.Equal(t, false, resp.ExecutionOptimistic)
		require.Equal(t, false, resp.Finalized)
	})
	t.Run("no blobs returns an empty array", func(t *testing.T) {
		u := fmt.Sprintf("http://foo.example/%d", es)
		request := httptest.NewRequest("GET", u, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		s.Blocker = &lookup.BeaconDbBlocker{
			ChainInfoFetcher: &mockChain.ChainService{FinalizedCheckPoint: &eth.Checkpoint{Root: blockRoot[:]}, Block: denebBlock},
			GenesisTimeFetcher: &testutil.MockGenesisTimeFetcher{
				Genesis: time.Now(),
			},
			BeaconDB:    db,
			BlobStorage: filesystem.NewEphemeralBlobStorage(t), // new ephemeral storage
		}

		s.Blobs(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.SidecarsResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.Equal(t, len(resp.Data), 0)

		require.Equal(t, "deneb", resp.Version)
		require.Equal(t, false, resp.ExecutionOptimistic)
		require.Equal(t, false, resp.Finalized)
	})
	t.Run("blob index over max", func(t *testing.T) {
		overLimit := params.BeaconConfig().MaxBlobsPerBlock(ds)
		u := fmt.Sprintf("http://foo.example/%d?indices=%d", es, overLimit)
		request := httptest.NewRequest("GET", u, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		s.Blocker = &lookup.BeaconDbBlocker{}
		s.Blobs(writer, request)

		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &httputil.DefaultJsonError{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
		assert.Equal(t, true, strings.Contains(e.Message, fmt.Sprintf("requested blob indices [%d] are invalid", overLimit)))
	})
	t.Run("outside retention period returns 200 with what we have", func(t *testing.T) {
		u := fmt.Sprintf("http://foo.example/%d", es)
		request := httptest.NewRequest("GET", u, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		moc := &mockChain.ChainService{FinalizedCheckPoint: &eth.Checkpoint{Root: blockRoot[:]}, Block: denebBlock}
		s.Blocker = &lookup.BeaconDbBlocker{
			ChainInfoFetcher:   moc,
			GenesisTimeFetcher: moc, // genesis time is set to 0 here, so it results in current epoch being extremely large
			BeaconDB:           db,
			BlobStorage:        bs,
		}

		s.Blobs(writer, request)

		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.SidecarsResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.Equal(t, 4, len(resp.Data))

		require.Equal(t, "deneb", resp.Version)
		require.Equal(t, false, resp.ExecutionOptimistic)
		require.Equal(t, false, resp.Finalized)
	})
	t.Run("block without commitments returns 200 w/empty list ", func(t *testing.T) {
		denebBlock, _ := util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, es+128, 0)
		commitments, err := denebBlock.Block().Body().BlobKzgCommitments()
		require.NoError(t, err)
		require.Equal(t, len(commitments), 0)
		require.NoError(t, db.SaveBlock(t.Context(), denebBlock))

		u := fmt.Sprintf("http://foo.example/%d", es+128)
		request := httptest.NewRequest("GET", u, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		s.Blocker = &lookup.BeaconDbBlocker{
			ChainInfoFetcher: &mockChain.ChainService{FinalizedCheckPoint: &eth.Checkpoint{Root: blockRoot[:]}, Block: denebBlock},
			GenesisTimeFetcher: &testutil.MockGenesisTimeFetcher{
				Genesis: time.Now(),
			},
			BeaconDB:    db,
			BlobStorage: bs,
		}

		s.Blobs(writer, request)

		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.SidecarsResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.Equal(t, 0, len(resp.Data))

		require.Equal(t, "deneb", resp.Version)
		require.Equal(t, false, resp.ExecutionOptimistic)
		require.Equal(t, false, resp.Finalized)
	})
	t.Run("slot before Deneb fork", func(t *testing.T) {
		// Create and save a pre-Deneb block at slot 31
		predenebBlock := util.NewBeaconBlock()
		predenebBlock.Block.Slot = 31
		util.SaveBlock(t, t.Context(), db, predenebBlock)

		u := "http://foo.example/31"
		request := httptest.NewRequest("GET", u, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		s.Blocker = &lookup.BeaconDbBlocker{
			ChainInfoFetcher: &mockChain.ChainService{},
			GenesisTimeFetcher: &testutil.MockGenesisTimeFetcher{
				Genesis: time.Now(),
			},
			BeaconDB:    db,
			BlobStorage: bs,
		}

		s.Blobs(writer, request)

		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &httputil.DefaultJsonError{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
		assert.StringContains(t, "not supported before", e.Message)
	})
	t.Run("malformed block ID", func(t *testing.T) {
		u := "http://foo.example/foo"
		request := httptest.NewRequest("GET", u, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		s.Blocker = &lookup.BeaconDbBlocker{}
		s.Blobs(writer, request)

		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &httputil.DefaultJsonError{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
		assert.Equal(t, true, strings.Contains(e.Message, "Invalid block ID"))
	})
	t.Run("ssz", func(t *testing.T) {
		u := "http://foo.example/finalized?indices=0"
		request := httptest.NewRequest("GET", u, nil)
		request.Header.Add("Accept", "application/octet-stream")
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		s.Blocker = &lookup.BeaconDbBlocker{
			ChainInfoFetcher: &mockChain.ChainService{FinalizedCheckPoint: &eth.Checkpoint{Root: blockRoot[:]}},
			GenesisTimeFetcher: &testutil.MockGenesisTimeFetcher{
				Genesis: time.Now(),
			},
			BeaconDB:    db,
			BlobStorage: bs,
		}
		s.Blobs(writer, request)
		assert.Equal(t, version.String(version.Deneb), writer.Header().Get(api.VersionHeader))
		assert.Equal(t, http.StatusOK, writer.Code)
		require.Equal(t, len(writer.Body.Bytes()), fieldparams.BlobSidecarSize) // size of each sidecar
		// can directly unmarshal to sidecar since there's only 1
		var sidecar eth.BlobSidecar
		require.NoError(t, sidecar.UnmarshalSSZ(writer.Body.Bytes()))
		require.NotNil(t, sidecar.Blob)
	})
	t.Run("ssz multiple blobs", func(t *testing.T) {
		u := "http://foo.example/finalized"
		request := httptest.NewRequest("GET", u, nil)
		request.Header.Add("Accept", "application/octet-stream")
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		s.Blocker = &lookup.BeaconDbBlocker{
			ChainInfoFetcher: &mockChain.ChainService{FinalizedCheckPoint: &eth.Checkpoint{Root: blockRoot[:]}},
			GenesisTimeFetcher: &testutil.MockGenesisTimeFetcher{
				Genesis: time.Now(),
			},
			BeaconDB:    db,
			BlobStorage: bs,
		}
		s.Blobs(writer, request)

		assert.Equal(t, http.StatusOK, writer.Code)
		require.Equal(t, len(writer.Body.Bytes()), fieldparams.BlobSidecarSize*4) // size of each sidecar
	})
}

func TestBlobs_Electra(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.FuluForkEpoch = cfg.ElectraForkEpoch + 4096*2
	cfg.BlobSchedule = []params.BlobScheduleEntry{
		{Epoch: cfg.FuluForkEpoch + 4096, MaxBlobsPerBlock: 6},
		{Epoch: cfg.FuluForkEpoch + 4096 + 128, MaxBlobsPerBlock: 9},
	}
	params.OverrideBeaconConfig(cfg)

	es := util.SlotAtEpoch(t, cfg.ElectraForkEpoch)
	db := testDB.SetupDB(t)
	overLimit := params.BeaconConfig().MaxBlobsPerBlock(es)
	electraBlock, blobs := util.GenerateTestElectraBlockWithSidecar(t, [32]byte{}, es, overLimit)
	require.NoError(t, db.SaveBlock(t.Context(), electraBlock))
	bs := filesystem.NewEphemeralBlobStorage(t)
	testSidecars := verification.FakeVerifySliceForTest(t, blobs)
	for i := range testSidecars {
		require.NoError(t, bs.Save(testSidecars[i]))
	}
	blockRoot := blobs[0].BlockRoot()

	mockChainService := &mockChain.ChainService{
		FinalizedRoots: map[[32]byte]bool{},
	}
	s := &Server{
		OptimisticModeFetcher: mockChainService,
		FinalizationFetcher:   mockChainService,
		TimeFetcher:           mockChainService,
	}
	t.Run("max blobs for electra", func(t *testing.T) {
		u := fmt.Sprintf("http://foo.example/%d", es)
		request := httptest.NewRequest("GET", u, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		s.Blocker = &lookup.BeaconDbBlocker{
			ChainInfoFetcher: &mockChain.ChainService{FinalizedCheckPoint: &eth.Checkpoint{Root: blockRoot[:]}, Block: electraBlock},
			GenesisTimeFetcher: &testutil.MockGenesisTimeFetcher{
				Genesis: time.Now(),
			},
			BeaconDB:    db,
			BlobStorage: bs,
		}
		s.Blobs(writer, request)

		assert.Equal(t, version.String(version.Electra), writer.Header().Get(api.VersionHeader))
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.SidecarsResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.Equal(t, overLimit, len(resp.Data))
		sidecar := resp.Data[0]
		require.NotNil(t, sidecar)
		assert.Equal(t, "0", sidecar.Index)
		assert.Equal(t, hexutil.Encode(blobs[0].Blob), sidecar.Blob)
		assert.Equal(t, hexutil.Encode(blobs[0].KzgCommitment), sidecar.KzgCommitment)
		assert.Equal(t, hexutil.Encode(blobs[0].KzgProof), sidecar.KzgProof)

		require.Equal(t, version.String(version.Electra), resp.Version)
		require.Equal(t, false, resp.ExecutionOptimistic)
		require.Equal(t, false, resp.Finalized)
	})
	t.Run("requested blob index at max", func(t *testing.T) {
		limit := params.BeaconConfig().MaxBlobsPerBlock(es) - 1
		u := fmt.Sprintf("http://foo.example/%d?indices=%d", es, limit)
		request := httptest.NewRequest("GET", u, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		s.Blocker = &lookup.BeaconDbBlocker{
			ChainInfoFetcher: &mockChain.ChainService{FinalizedCheckPoint: &eth.Checkpoint{Root: blockRoot[:]}, Block: electraBlock},
			GenesisTimeFetcher: &testutil.MockGenesisTimeFetcher{
				Genesis: time.Now(),
			},
			BeaconDB:    db,
			BlobStorage: bs,
		}
		s.Blobs(writer, request)

		assert.Equal(t, version.String(version.Electra), writer.Header().Get(api.VersionHeader))
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.SidecarsResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.Equal(t, 1, len(resp.Data))
		sidecar := resp.Data[0]
		require.NotNil(t, sidecar)
		assert.Equal(t, fmt.Sprintf("%d", limit), sidecar.Index)
		assert.Equal(t, hexutil.Encode(blobs[limit].Blob), sidecar.Blob)
		assert.Equal(t, hexutil.Encode(blobs[limit].KzgCommitment), sidecar.KzgCommitment)
		assert.Equal(t, hexutil.Encode(blobs[limit].KzgProof), sidecar.KzgProof)

		require.Equal(t, version.String(version.Electra), resp.Version)
		require.Equal(t, false, resp.ExecutionOptimistic)
		require.Equal(t, false, resp.Finalized)
	})
	t.Run("blob index over max", func(t *testing.T) {
		overLimit := params.BeaconConfig().MaxBlobsPerBlock(es)
		u := fmt.Sprintf("http://foo.example/%d?indices=%d", es, overLimit)
		request := httptest.NewRequest("GET", u, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		s.Blocker = &lookup.BeaconDbBlocker{}
		s.Blobs(writer, request)

		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &httputil.DefaultJsonError{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
		assert.Equal(t, true, strings.Contains(e.Message, fmt.Sprintf("requested blob indices [%d] are invalid", overLimit)))
	})
}

func Test_parseIndices(t *testing.T) {
	ds := util.SlotAtEpoch(t, params.BeaconConfig().DenebForkEpoch)
	tests := []struct {
		name    string
		query   string
		want    []int
		wantErr string
	}{
		{
			name:  "happy path with duplicate indices within bound and other query parameters ignored",
			query: "indices=1&indices=2&indices=1&indices=3&bar=bar",
			want:  []int{1, 2, 3},
		},
		{
			name:    "out of bounds indices throws error",
			query:   "indices=6&indices=7",
			wantErr: "requested blob indices [6 7] are invalid",
		},
		{
			name:    "negative indices",
			query:   "indices=-1&indices=-8",
			wantErr: "requested blob indices [-1 -8] are invalid",
		},
		{
			name:    "invalid indices",
			query:   "indices=foo&indices=bar",
			wantErr: "requested blob indices [foo bar] are invalid",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseIndices(&url.URL{RawQuery: tt.query}, ds)
			if err != nil && tt.wantErr != "" {
				require.StringContains(t, tt.wantErr, err.Error())
				return
			}
			require.NoError(t, err)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseIndices() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetBlobs(t *testing.T) {
	// Start the trusted setup for KZG operations (needed for data columns)
	require.NoError(t, kzg.Start())

	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.DenebForkEpoch = 1
	cfg.ElectraForkEpoch = 10
	cfg.FuluForkEpoch = 20
	cfg.BlobSchedule = []params.BlobScheduleEntry{
		{Epoch: 0, MaxBlobsPerBlock: 0},
		{Epoch: 1, MaxBlobsPerBlock: 6},   // Deneb
		{Epoch: 10, MaxBlobsPerBlock: 9},  // Electra
		{Epoch: 20, MaxBlobsPerBlock: 12}, // Fulu
	}
	params.OverrideBeaconConfig(cfg)
	es := util.SlotAtEpoch(t, cfg.ElectraForkEpoch)

	db := testDB.SetupDB(t)
	denebBlock, blobs := util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, 123, 4)
	require.NoError(t, db.SaveBlock(t.Context(), denebBlock))
	bs := filesystem.NewEphemeralBlobStorage(t)
	testSidecars := verification.FakeVerifySliceForTest(t, blobs)
	for i := range testSidecars {
		require.NoError(t, bs.Save(testSidecars[i]))
	}
	blockRoot := blobs[0].BlockRoot()

	mockChainService := &mockChain.ChainService{
		FinalizedRoots: map[[32]byte]bool{},
		Genesis:        time.Now().Add(-time.Duration(uint64(params.BeaconConfig().SlotsPerEpoch)*uint64(params.BeaconConfig().DenebForkEpoch)*params.BeaconConfig().SecondsPerSlot) * time.Second),
	}
	s := &Server{
		OptimisticModeFetcher: mockChainService,
		FinalizationFetcher:   mockChainService,
		TimeFetcher:           mockChainService,
	}

	t.Run("genesis", func(t *testing.T) {
		u := "http://foo.example/genesis"
		request := httptest.NewRequest("GET", u, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		s.Blocker = &lookup.BeaconDbBlocker{}
		s.GetBlobs(writer, request)

		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &httputil.DefaultJsonError{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
		assert.StringContains(t, "not supported for Phase 0 fork", e.Message)
	})
	t.Run("head", func(t *testing.T) {
		u := "http://foo.example/head"
		request := httptest.NewRequest("GET", u, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		s.Blocker = &lookup.BeaconDbBlocker{
			ChainInfoFetcher: &mockChain.ChainService{Root: blockRoot[:], Block: denebBlock},
			GenesisTimeFetcher: &testutil.MockGenesisTimeFetcher{
				Genesis: time.Now(),
			},
			BeaconDB:    db,
			BlobStorage: bs,
		}
		s.GetBlobs(writer, request)

		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetBlobsResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.Equal(t, 4, len(resp.Data))
		blob := resp.Data[0]
		require.NotNil(t, blob)
		assert.Equal(t, hexutil.Encode(blobs[0].Blob), blob)
		blob = resp.Data[1]
		require.NotNil(t, blob)
		assert.Equal(t, hexutil.Encode(blobs[1].Blob), blob)
		blob = resp.Data[2]
		require.NotNil(t, blob)
		assert.Equal(t, hexutil.Encode(blobs[2].Blob), blob)
		blob = resp.Data[3]
		require.NotNil(t, blob)
		assert.Equal(t, hexutil.Encode(blobs[3].Blob), blob)
		require.Equal(t, false, resp.ExecutionOptimistic)
		require.Equal(t, false, resp.Finalized)
	})
	t.Run("finalized", func(t *testing.T) {
		u := "http://foo.example/finalized"
		request := httptest.NewRequest("GET", u, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		s.Blocker = &lookup.BeaconDbBlocker{
			ChainInfoFetcher: &mockChain.ChainService{FinalizedCheckPoint: &eth.Checkpoint{Root: blockRoot[:]}, Block: denebBlock},
			GenesisTimeFetcher: &testutil.MockGenesisTimeFetcher{
				Genesis: time.Now(),
			},
			BeaconDB:    db,
			BlobStorage: bs,
		}
		s.GetBlobs(writer, request)

		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetBlobsResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.Equal(t, 4, len(resp.Data))

		require.Equal(t, false, resp.ExecutionOptimistic)
		require.Equal(t, false, resp.Finalized)
	})
	t.Run("root", func(t *testing.T) {
		u := "http://foo.example/" + hexutil.Encode(blockRoot[:])
		request := httptest.NewRequest("GET", u, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		s.Blocker = &lookup.BeaconDbBlocker{
			ChainInfoFetcher: &mockChain.ChainService{Block: denebBlock},
			BeaconDB:         db,
			GenesisTimeFetcher: &testutil.MockGenesisTimeFetcher{
				Genesis: time.Now(),
			},
			BlobStorage: bs,
		}
		s.GetBlobs(writer, request)

		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetBlobsResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.Equal(t, 4, len(resp.Data))

		require.Equal(t, false, resp.ExecutionOptimistic)
		require.Equal(t, false, resp.Finalized)
	})
	t.Run("slot", func(t *testing.T) {
		u := "http://foo.example/123"
		request := httptest.NewRequest("GET", u, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		s.Blocker = &lookup.BeaconDbBlocker{
			ChainInfoFetcher: &mockChain.ChainService{Block: denebBlock},
			BeaconDB:         db,
			GenesisTimeFetcher: &testutil.MockGenesisTimeFetcher{
				Genesis: time.Now(),
			},
			BlobStorage: bs,
		}
		s.GetBlobs(writer, request)

		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetBlobsResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.Equal(t, 4, len(resp.Data))

		require.Equal(t, false, resp.ExecutionOptimistic)
		require.Equal(t, false, resp.Finalized)
	})
	t.Run("slot not found", func(t *testing.T) {
		u := "http://foo.example/122"
		request := httptest.NewRequest("GET", u, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		s.Blocker = &lookup.BeaconDbBlocker{
			ChainInfoFetcher: &mockChain.ChainService{Block: denebBlock},
			BeaconDB:         db,
			GenesisTimeFetcher: &testutil.MockGenesisTimeFetcher{
				Genesis: time.Now(),
			},
			BlobStorage: bs,
		}
		s.GetBlobs(writer, request)

		assert.Equal(t, http.StatusNotFound, writer.Code)
	})
	t.Run("no blobs returns an empty array", func(t *testing.T) {
		u := "http://foo.example/123"
		request := httptest.NewRequest("GET", u, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		s.Blocker = &lookup.BeaconDbBlocker{
			ChainInfoFetcher: &mockChain.ChainService{FinalizedCheckPoint: &eth.Checkpoint{Root: blockRoot[:]}, Block: denebBlock},
			GenesisTimeFetcher: &testutil.MockGenesisTimeFetcher{
				Genesis: time.Now(),
			},
			BeaconDB:    db,
			BlobStorage: filesystem.NewEphemeralBlobStorage(t), // new ephemeral storage
		}

		s.GetBlobs(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetBlobsResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.Equal(t, len(resp.Data), 0)

		require.Equal(t, false, resp.ExecutionOptimistic)
		require.Equal(t, false, resp.Finalized)
	})
	t.Run("outside retention period still returns 200 what we have in db ", func(t *testing.T) {
		u := "http://foo.example/123"
		request := httptest.NewRequest("GET", u, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		moc := &mockChain.ChainService{FinalizedCheckPoint: &eth.Checkpoint{Root: blockRoot[:]}, Block: denebBlock}
		s.Blocker = &lookup.BeaconDbBlocker{
			ChainInfoFetcher:   moc,
			GenesisTimeFetcher: moc, // genesis time is set to 0 here, so it results in current epoch being extremely large
			BeaconDB:           db,
			BlobStorage:        bs,
		}

		s.GetBlobs(writer, request)

		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetBlobsResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.Equal(t, 4, len(resp.Data))

		require.Equal(t, false, resp.ExecutionOptimistic)
		require.Equal(t, false, resp.Finalized)
	})
	t.Run("block without commitments returns 200 w/empty list ", func(t *testing.T) {
		denebBlock, _ := util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, 333, 0)
		commitments, err := denebBlock.Block().Body().BlobKzgCommitments()
		require.NoError(t, err)
		require.Equal(t, len(commitments), 0)
		require.NoError(t, db.SaveBlock(t.Context(), denebBlock))

		u := "http://foo.example/333"
		request := httptest.NewRequest("GET", u, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		s.Blocker = &lookup.BeaconDbBlocker{
			ChainInfoFetcher: &mockChain.ChainService{FinalizedCheckPoint: &eth.Checkpoint{Root: blockRoot[:]}, Block: denebBlock},
			GenesisTimeFetcher: &testutil.MockGenesisTimeFetcher{
				Genesis: time.Now(),
			},
			BeaconDB:    db,
			BlobStorage: bs,
		}

		s.GetBlobs(writer, request)

		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetBlobsResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.Equal(t, 0, len(resp.Data))

		require.Equal(t, false, resp.ExecutionOptimistic)
		require.Equal(t, false, resp.Finalized)
	})
	t.Run("slot before Deneb fork", func(t *testing.T) {
		// Create and save a pre-Deneb block at slot 31
		predenebBlock := util.NewBeaconBlock()
		predenebBlock.Block.Slot = 31
		util.SaveBlock(t, t.Context(), db, predenebBlock)

		u := "http://foo.example/31"
		request := httptest.NewRequest("GET", u, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		s.Blocker = &lookup.BeaconDbBlocker{
			BeaconDB:         db,
			ChainInfoFetcher: &mockChain.ChainService{},
			GenesisTimeFetcher: &testutil.MockGenesisTimeFetcher{
				Genesis: time.Now(),
			},
		}

		s.GetBlobs(writer, request)

		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &httputil.DefaultJsonError{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
		assert.StringContains(t, "not supported before", e.Message)
	})
	t.Run("malformed block ID", func(t *testing.T) {
		u := "http://foo.example/foo"
		request := httptest.NewRequest("GET", u, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		s.Blocker = &lookup.BeaconDbBlocker{}
		s.GetBlobs(writer, request)

		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &httputil.DefaultJsonError{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
		assert.Equal(t, true, strings.Contains(e.Message, "could not parse block ID"))
	})
	t.Run("ssz", func(t *testing.T) {
		u := "http://foo.example/finalized"
		request := httptest.NewRequest("GET", u, nil)
		request.Header.Add("Accept", "application/octet-stream")
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		s.Blocker = &lookup.BeaconDbBlocker{
			ChainInfoFetcher: &mockChain.ChainService{FinalizedCheckPoint: &eth.Checkpoint{Root: blockRoot[:]}},
			GenesisTimeFetcher: &testutil.MockGenesisTimeFetcher{
				Genesis: time.Now(),
			},
			BeaconDB:    db,
			BlobStorage: bs,
		}
		s.GetBlobs(writer, request)
		assert.Equal(t, version.String(version.Deneb), writer.Header().Get(api.VersionHeader))
		assert.Equal(t, http.StatusOK, writer.Code)
		require.Equal(t, fieldparams.BlobSize*4, len(writer.Body.Bytes())) // size of 4 sidecars
		// unmarshal all 4 blobs
		blbs := unmarshalBlobs(t, writer.Body.Bytes())
		require.Equal(t, 4, len(blbs))
	})
	t.Run("ssz multiple blobs", func(t *testing.T) {
		u := "http://foo.example/finalized"
		request := httptest.NewRequest("GET", u, nil)
		request.Header.Add("Accept", "application/octet-stream")
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		s.Blocker = &lookup.BeaconDbBlocker{
			ChainInfoFetcher: &mockChain.ChainService{FinalizedCheckPoint: &eth.Checkpoint{Root: blockRoot[:]}},
			GenesisTimeFetcher: &testutil.MockGenesisTimeFetcher{
				Genesis: time.Now(),
			},
			BeaconDB:    db,
			BlobStorage: bs,
		}
		s.GetBlobs(writer, request)

		assert.Equal(t, http.StatusOK, writer.Code)
		blbs := unmarshalBlobs(t, writer.Body.Bytes())
		require.Equal(t, 4, len(blbs))
	})

	t.Run("versioned_hashes invalid hex", func(t *testing.T) {
		u := "http://foo.example/finalized?versioned_hashes=invalidhex,invalid2hex"
		request := httptest.NewRequest("GET", u, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		s.Blocker = &lookup.BeaconDbBlocker{
			ChainInfoFetcher: &mockChain.ChainService{FinalizedCheckPoint: &eth.Checkpoint{Root: blockRoot[:]}},
			GenesisTimeFetcher: &testutil.MockGenesisTimeFetcher{
				Genesis: time.Now(),
			},
			BeaconDB:    db,
			BlobStorage: bs,
		}
		s.GetBlobs(writer, request)

		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &httputil.DefaultJsonError{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
		assert.StringContains(t, "versioned_hashes[0] is invalid", e.Message)
		assert.StringContains(t, "hex string without 0x prefix", e.Message)
	})

	t.Run("versioned_hashes invalid length", func(t *testing.T) {
		// Using 16 bytes instead of 32
		shortHash := "0x1234567890abcdef1234567890abcdef"
		u := fmt.Sprintf("http://foo.example/finalized?versioned_hashes=%s", shortHash)
		request := httptest.NewRequest("GET", u, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		s.Blocker = &lookup.BeaconDbBlocker{
			ChainInfoFetcher: &mockChain.ChainService{FinalizedCheckPoint: &eth.Checkpoint{Root: blockRoot[:]}},
			GenesisTimeFetcher: &testutil.MockGenesisTimeFetcher{
				Genesis: time.Now(),
			},
			BeaconDB:    db,
			BlobStorage: bs,
		}
		s.GetBlobs(writer, request)

		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &httputil.DefaultJsonError{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
		assert.StringContains(t, "Invalid versioned_hashes[0]:", e.Message)
		assert.StringContains(t, "is not length 32", e.Message)
	})

	t.Run("versioned_hashes valid single hash", func(t *testing.T) {
		// Get the first blob's commitment and convert to versioned hash
		versionedHash := primitives.ConvertKzgCommitmentToVersionedHash(blobs[0].KzgCommitment)

		u := fmt.Sprintf("http://foo.example/finalized?versioned_hashes=%s", hexutil.Encode(versionedHash[:]))
		request := httptest.NewRequest("GET", u, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		s.Blocker = &lookup.BeaconDbBlocker{
			ChainInfoFetcher: &mockChain.ChainService{FinalizedCheckPoint: &eth.Checkpoint{Root: blockRoot[:]}},
			GenesisTimeFetcher: &testutil.MockGenesisTimeFetcher{
				Genesis: time.Now(),
			},
			BeaconDB:    db,
			BlobStorage: bs,
		}
		s.GetBlobs(writer, request)

		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetBlobsResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.Equal(t, 1, len(resp.Data)) // Should return only the requested blob
		blob := resp.Data[0]
		require.NotNil(t, blob)
		assert.Equal(t, hexutil.Encode(blobs[0].Blob), blob)
	})

	t.Run("versioned_hashes multiple hashes", func(t *testing.T) {
		// Get commitments for blobs 1 and 3 and convert to versioned hashes
		versionedHash1 := primitives.ConvertKzgCommitmentToVersionedHash(blobs[1].KzgCommitment)
		versionedHash3 := primitives.ConvertKzgCommitmentToVersionedHash(blobs[3].KzgCommitment)

		u := fmt.Sprintf("http://foo.example/finalized?versioned_hashes=%s&versioned_hashes=%s",
			hexutil.Encode(versionedHash1[:]), hexutil.Encode(versionedHash3[:]))
		request := httptest.NewRequest("GET", u, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		s.Blocker = &lookup.BeaconDbBlocker{
			ChainInfoFetcher: &mockChain.ChainService{FinalizedCheckPoint: &eth.Checkpoint{Root: blockRoot[:]}},
			GenesisTimeFetcher: &testutil.MockGenesisTimeFetcher{
				Genesis: time.Now(),
			},
			BeaconDB:    db,
			BlobStorage: bs,
		}
		s.GetBlobs(writer, request)

		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetBlobsResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.Equal(t, 2, len(resp.Data)) // Should return 2 requested blobs
		// Verify blobs are returned in KZG commitment order from the block (1, 3)
		// not in the requested order
		assert.Equal(t, hexutil.Encode(blobs[1].Blob), resp.Data[0])
		assert.Equal(t, hexutil.Encode(blobs[3].Blob), resp.Data[1])
	})

	// Test for Electra fork
	t.Run("electra max blobs", func(t *testing.T) {
		overLimit := params.BeaconConfig().MaxBlobsPerBlock(es)
		electraBlock, electraBlobs := util.GenerateTestElectraBlockWithSidecar(t, [32]byte{}, 323, overLimit)
		require.NoError(t, db.SaveBlock(t.Context(), electraBlock))
		electraBs := filesystem.NewEphemeralBlobStorage(t)
		electraSidecars := verification.FakeVerifySliceForTest(t, electraBlobs)
		for i := range electraSidecars {
			require.NoError(t, electraBs.Save(electraSidecars[i]))
		}
		electraBlockRoot := electraBlobs[0].BlockRoot()

		u := "http://foo.example/323"
		request := httptest.NewRequest("GET", u, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		s.Blocker = &lookup.BeaconDbBlocker{
			ChainInfoFetcher: &mockChain.ChainService{FinalizedCheckPoint: &eth.Checkpoint{Root: electraBlockRoot[:]}, Block: electraBlock},
			GenesisTimeFetcher: &testutil.MockGenesisTimeFetcher{
				Genesis: time.Now(),
			},
			BeaconDB:    db,
			BlobStorage: electraBs,
		}
		s.GetBlobs(writer, request)

		assert.Equal(t, version.String(version.Electra), writer.Header().Get(api.VersionHeader))
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetBlobsResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))

		require.Equal(t, overLimit, len(resp.Data))
		blob := resp.Data[0]
		require.NotNil(t, blob)
		assert.Equal(t, hexutil.Encode(electraBlobs[0].Blob), blob)
	})

	// Test for Fulu fork with data columns
	t.Run("fulu with data columns", func(t *testing.T) {
		// Generate a Fulu block with data columns
		fuluForkSlot := primitives.Slot(20 * params.BeaconConfig().SlotsPerEpoch) // Fulu is at epoch 20
		fuluBlock, _, verifiedRoDataColumnSidecars := util.GenerateTestFuluBlockWithSidecars(t, 3, util.WithSlot(fuluForkSlot))
		require.NoError(t, db.SaveBlock(t.Context(), fuluBlock.ReadOnlySignedBeaconBlock))
		fuluBlockRoot := fuluBlock.Root()

		// Store data columns
		_, dataColumnStorage := filesystem.NewEphemeralDataColumnStorageAndFs(t)
		require.NoError(t, dataColumnStorage.Save(verifiedRoDataColumnSidecars))

		u := fmt.Sprintf("http://foo.example/%d", fuluForkSlot)
		request := httptest.NewRequest("GET", u, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		// Create an empty blob storage (won't be used but needs to be non-nil)
		_, emptyBlobStorage := filesystem.NewEphemeralBlobStorageAndFs(t)
		s.Blocker = &lookup.BeaconDbBlocker{
			ChainInfoFetcher: &mockChain.ChainService{FinalizedCheckPoint: &eth.Checkpoint{Root: fuluBlockRoot[:]}, Block: fuluBlock.ReadOnlySignedBeaconBlock},
			GenesisTimeFetcher: &testutil.MockGenesisTimeFetcher{
				Genesis: time.Now(),
			},
			BeaconDB:          db,
			BlobStorage:       emptyBlobStorage,
			DataColumnStorage: dataColumnStorage,
		}
		s.GetBlobs(writer, request)

		assert.Equal(t, version.String(version.Fulu), writer.Header().Get(api.VersionHeader))
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetBlobsResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.Equal(t, 3, len(resp.Data))

		// Verify that we got blobs back (they were reconstructed from data columns)
		for i := range resp.Data {
			require.NotNil(t, resp.Data[i])
		}
	})

	// Test for Fulu with versioned hashes and data columns
	t.Run("fulu versioned_hashes with data columns", func(t *testing.T) {
		// Generate a Fulu block with data columns
		fuluForkSlot2 := primitives.Slot(20*params.BeaconConfig().SlotsPerEpoch + 10) // Fulu is at epoch 20
		fuluBlock2, _, verifiedRoDataColumnSidecars2 := util.GenerateTestFuluBlockWithSidecars(t, 4, util.WithSlot(fuluForkSlot2))
		require.NoError(t, db.SaveBlock(t.Context(), fuluBlock2.ReadOnlySignedBeaconBlock))
		fuluBlockRoot2 := fuluBlock2.Root()

		// Store data columns
		_, dataColumnStorage := filesystem.NewEphemeralDataColumnStorageAndFs(t)
		require.NoError(t, dataColumnStorage.Save(verifiedRoDataColumnSidecars2))

		// Get the commitments from the block to derive versioned hashes
		commitments, err := fuluBlock2.Block().Body().BlobKzgCommitments()
		require.NoError(t, err)
		require.Equal(t, true, len(commitments) >= 3)

		// Request specific blobs by versioned hashes in reverse order
		// We request commitments[2] and commitments[0], but they should be returned
		// in commitment order from the block (0, 2), not in the requested order
		versionedHash1 := primitives.ConvertKzgCommitmentToVersionedHash(commitments[2])
		versionedHash2 := primitives.ConvertKzgCommitmentToVersionedHash(commitments[0])

		u := fmt.Sprintf("http://foo.example/%d?versioned_hashes=%s&versioned_hashes=%s", fuluForkSlot2,
			hexutil.Encode(versionedHash1[:]),
			hexutil.Encode(versionedHash2[:]))
		request := httptest.NewRequest("GET", u, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		// Create an empty blob storage (won't be used but needs to be non-nil)
		_, emptyBlobStorage := filesystem.NewEphemeralBlobStorageAndFs(t)
		s.Blocker = &lookup.BeaconDbBlocker{
			ChainInfoFetcher: &mockChain.ChainService{FinalizedCheckPoint: &eth.Checkpoint{Root: fuluBlockRoot2[:]}, Block: fuluBlock2.ReadOnlySignedBeaconBlock},
			GenesisTimeFetcher: &testutil.MockGenesisTimeFetcher{
				Genesis: time.Now(),
			},
			BeaconDB:          db,
			BlobStorage:       emptyBlobStorage,
			DataColumnStorage: dataColumnStorage,
		}
		s.GetBlobs(writer, request)

		assert.Equal(t, version.String(version.Fulu), writer.Header().Get(api.VersionHeader))
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetBlobsResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.Equal(t, 2, len(resp.Data))
		// Blobs are returned in commitment order, regardless of request order
	})
}

func unmarshalBlobs(t *testing.T, response []byte) [][]byte {
	require.Equal(t, 0, len(response)%fieldparams.BlobSize)
	if len(response) == fieldparams.BlobSize {
		return [][]byte{response}
	}
	blobs := make([][]byte, len(response)/fieldparams.BlobSize)
	for i := range blobs {
		blobs[i] = response[i*fieldparams.BlobSize : (i+1)*fieldparams.BlobSize]
	}
	return blobs
}
