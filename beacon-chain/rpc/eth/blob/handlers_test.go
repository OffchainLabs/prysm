package blob

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/prysmaticlabs/prysm/v5/api/server/structs"
	mockChain "github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain/testing"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/db/filesystem"
	testDB "github.com/prysmaticlabs/prysm/v5/beacon-chain/db/testing"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/rpc/lookup"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/rpc/testutil"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/verification"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/network/httputil"
	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
)

func TestBlobs(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.DenebForkEpoch = 1
	params.OverrideBeaconConfig(cfg)

	db := testDB.SetupDB(t)
	denebBlock, blobs := util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, 123, 4)
	require.NoError(t, db.SaveBlock(context.Background(), denebBlock))
	bs := filesystem.NewEphemeralBlobStorage(t)
	testSidecars, err := verification.BlobSidecarSliceNoop(blobs)
	require.NoError(t, err)
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
		assert.StringContains(t, "blobs are not supported for Phase 0 fork", e.Message)
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
		s.Blobs(writer, request)

		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.SidecarsResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.Equal(t, 4, len(resp.Data))

		require.Equal(t, "deneb", resp.Version)
		require.Equal(t, false, resp.ExecutionOptimistic)
		require.Equal(t, false, resp.Finalized)
	})
	t.Run("one blob only", func(t *testing.T) {
		u := "http://foo.example/123?indices=2"
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

		s.Blobs(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.SidecarsResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.Equal(t, len(resp.Data), 0)

		require.Equal(t, "deneb", resp.Version)
		require.Equal(t, false, resp.ExecutionOptimistic)
		require.Equal(t, false, resp.Finalized)
	})
	t.Run("outside retention period returns 200 w/ empty list ", func(t *testing.T) {
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

		s.Blobs(writer, request)

		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.SidecarsResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.Equal(t, 0, len(resp.Data))

		require.Equal(t, "deneb", resp.Version)
		require.Equal(t, false, resp.ExecutionOptimistic)
		require.Equal(t, false, resp.Finalized)
	})
	t.Run("block without commitments returns 200 w/empty list ", func(t *testing.T) {
		denebBlock, _ := util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, 333, 0)
		commitments, err := denebBlock.Block().Body().BlobKzgCommitments()
		require.NoError(t, err)
		require.Equal(t, len(commitments), 0)
		require.NoError(t, db.SaveBlock(context.Background(), denebBlock))

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
		u := "http://foo.example/31"
		request := httptest.NewRequest("GET", u, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		s.Blocker = &lookup.BeaconDbBlocker{}

		s.Blobs(writer, request)

		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &httputil.DefaultJsonError{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
		assert.StringContains(t, "blobs are not supported before Deneb fork", e.Message)
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

func Test_parseIndices(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		want    []uint64
		wantErr string
	}{
		{
			name:  "happy path with duplicate indices within bound and other query parameters ignored",
			query: "indices=1&indices=2&indices=1&indices=3&bar=bar",
			want:  []uint64{1, 2, 3},
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
			got, err := parseIndices(&url.URL{RawQuery: tt.query}, 0)
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
