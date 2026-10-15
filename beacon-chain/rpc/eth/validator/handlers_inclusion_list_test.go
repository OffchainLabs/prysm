package validator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain"
	mockChain "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/operation"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	dbutil "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	mockExecution "github.com/OffchainLabs/prysm/v7/beacon-chain/execution/testing"
	p2pmock "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/core"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/testutil"
	mockSync "github.com/OffchainLabs/prysm/v7/beacon-chain/sync/initial-sync/testing"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpbalpha "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
)

// setHezeForkEpoch activates Gloas at genesis and Heze at the given epoch.
func setHezeForkEpoch(t *testing.T, epoch primitives.Epoch) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	cfg.HezeForkEpoch = epoch
	params.OverrideBeaconConfig(cfg)
	helpers.ClearCache()
}

func TestProduceInclusionList(t *testing.T) {
	setHezeForkEpoch(t, 0)
	slot := primitives.Slot(5)
	legacyTx := []byte{0xf8, 0x01, 0x02}
	dynamicFeeTx := []byte{0x02, 0x03, 0x04}
	blobTx := []byte{0x03, 0x05, 0x06}

	newServer := func(engine *mockExecution.EngineClient, syncing bool) *Server {
		chain := &mockChain.ChainService{Slot: &slot}
		return &Server{
			SyncChecker:           &mockSync.Sync{IsSyncing: syncing},
			HeadFetcher:           chain,
			TimeFetcher:           chain,
			OptimisticModeFetcher: chain,
			ExecutionEngineCaller: engine,
		}
	}
	produce := func(s *Server, query string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodGet, "http://example.com/eth/v1/validator/inclusion_list"+query, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		s.ProduceInclusionList(writer, request)
		return writer
	}
	decode := func(t *testing.T, writer *httptest.ResponseRecorder) []string {
		require.Equal(t, http.StatusOK, writer.Code, writer.Body.String())
		resp := &structs.ProduceInclusionListResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.NotNil(t, resp.Data)
		return resp.Data
	}

	t.Run("ok", func(t *testing.T) {
		s := newServer(&mockExecution.EngineClient{InclusionList: [][]byte{legacyTx, dynamicFeeTx}}, false)
		got := decode(t, produce(s, "?slot=5"))
		require.DeepEqual(t, []string{hexutil.Encode(legacyTx), hexutil.Encode(dynamicFeeTx)}, got)
	})
	t.Run("empty list is an empty array", func(t *testing.T) {
		s := newServer(&mockExecution.EngineClient{}, false)
		writer := produce(s, "?slot=5")
		require.Equal(t, 0, len(decode(t, writer)))
		require.StringContains(t, `"data":[]`, writer.Body.String())
	})
	t.Run("blob and empty transactions dropped", func(t *testing.T) {
		s := newServer(&mockExecution.EngineClient{InclusionList: [][]byte{blobTx, legacyTx, {}, dynamicFeeTx}}, false)
		got := decode(t, produce(s, "?slot=5"))
		require.DeepEqual(t, []string{hexutil.Encode(legacyTx), hexutil.Encode(dynamicFeeTx)}, got)
	})
	t.Run("oversize list keeps the prefix that fits", func(t *testing.T) {
		limit := params.BeaconConfig().MaxTransactionsBytesPerInclusionList
		first := bytes.Repeat([]byte{0x02}, int(limit)-100)
		second := bytes.Repeat([]byte{0x02}, 200)
		third := bytes.Repeat([]byte{0x02}, 50)
		s := newServer(&mockExecution.EngineClient{InclusionList: [][]byte{first, second, third}}, false)
		got := decode(t, produce(s, "?slot=5"))
		require.DeepEqual(t, []string{hexutil.Encode(first)}, got)
	})
	t.Run("exactly at the limit", func(t *testing.T) {
		limit := params.BeaconConfig().MaxTransactionsBytesPerInclusionList
		first := bytes.Repeat([]byte{0x02}, int(limit)-1)
		second := []byte{0x02}
		s := newServer(&mockExecution.EngineClient{InclusionList: [][]byte{first, second}}, false)
		require.Equal(t, 2, len(decode(t, produce(s, "?slot=5"))))
	})
	for _, tc := range []struct {
		name  string
		query string
		msg   string
	}{
		{name: "missing slot", query: "", msg: "slot is required"},
		{name: "next slot", query: "?slot=6", msg: "Requested slot 6 is not the current slot 5"},
		{name: "previous slot", query: "?slot=4", msg: "Requested slot 4 is not the current slot 5"},
		{name: "invalid slot", query: "?slot=abc", msg: "slot is invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newServer(&mockExecution.EngineClient{}, false)
			writer := produce(s, tc.query)
			require.Equal(t, http.StatusBadRequest, writer.Code)
			require.StringContains(t, tc.msg, writer.Body.String())
		})
	}
	t.Run("syncing", func(t *testing.T) {
		s := newServer(&mockExecution.EngineClient{}, true)
		require.Equal(t, http.StatusServiceUnavailable, produce(s, "?slot=5").Code)
	})
	t.Run("engine error", func(t *testing.T) {
		s := newServer(&mockExecution.EngineClient{ErrGetInclusionList: errors.New("bad")}, false)
		writer := produce(s, "?slot=5")
		require.Equal(t, http.StatusInternalServerError, writer.Code)
		require.StringContains(t, "Could not get inclusion list from the execution engine: bad", writer.Body.String())
	})
	// Last, because the config override stays for the rest of the parent test.
	t.Run("pre-Heze slot", func(t *testing.T) {
		setHezeForkEpoch(t, 10)
		s := newServer(&mockExecution.EngineClient{}, false)
		writer := produce(s, "?slot=5")
		require.Equal(t, http.StatusBadRequest, writer.Code)
		require.StringContains(t, "not available before the Heze fork", writer.Body.String())
	})
}

func testSignedInclusionList(t *testing.T) *ethpbalpha.SignedInclusionList {
	txs, err := enginev1.NewProgressiveTransactionList([][]byte{{0x02, 0x01}, {0xf8, 0x02, 0x03}})
	require.NoError(t, err)
	return &ethpbalpha.SignedInclusionList{
		Message: &ethpbalpha.InclusionList{
			Slot:           5,
			ValidatorIndex: 7,
			DependentRoot:  bytesutil.PadTo([]byte{0xaa}, fieldparams.RootLength),
			Transactions:   txs,
		},
		Signature: bytesutil.PadTo([]byte{0xbb}, fieldparams.BLSSignatureLength),
	}
}

func TestPublishInclusionList(t *testing.T) {
	setHezeForkEpoch(t, 0)
	signed := testSignedInclusionList(t)
	jsonBody, err := json.Marshal(&structs.PublishInclusionListRequest{Data: structs.SignedInclusionListFromConsensus(signed)})
	require.NoError(t, err)
	sszBody, err := signed.MarshalSSZ()
	require.NoError(t, err)

	type setup struct {
		s           *Server
		chain       *mockChain.ChainService
		broadcaster *p2pmock.MockBroadcaster
		events      chan *feed.Event
	}
	newSetup := func(t *testing.T, onErr error) setup {
		chain := &mockChain.ChainService{OnInclusionListErr: onErr}
		broadcaster := &p2pmock.MockBroadcaster{}
		notifier := chain.OperationNotifier()
		events := make(chan *feed.Event, 1)
		sub := notifier.OperationFeed().Subscribe(events)
		t.Cleanup(sub.Unsubscribe)
		return setup{
			s: &Server{
				InclusionListReceiver: chain,
				Broadcaster:           broadcaster,
				OperationNotifier:     notifier,
			},
			chain:       chain,
			broadcaster: broadcaster,
			events:      events,
		}
	}
	publish := func(s *Server, body []byte, contentType, consensusVersion string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "http://example.com/eth/v1/validator/inclusion_list", bytes.NewReader(body))
		request.Header.Set("Content-Type", contentType)
		if consensusVersion != "" {
			request.Header.Set(api.VersionHeader, consensusVersion)
		}
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		s.PublishInclusionList(writer, request)
		return writer
	}

	for _, tc := range []struct {
		name        string
		body        []byte
		contentType string
	}{
		{name: "json", body: jsonBody, contentType: api.JsonMediaType},
		{name: "ssz", body: sszBody, contentType: api.OctetStreamMediaType},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := newSetup(t, nil)
			writer := publish(st.s, tc.body, tc.contentType, version.String(version.Heze))
			require.Equal(t, http.StatusOK, writer.Code, writer.Body.String())

			require.Equal(t, 1, len(st.chain.InclusionListsReceived))
			require.DeepEqual(t, signed, st.chain.InclusionListsReceived[0])
			require.Equal(t, true, st.broadcaster.BroadcastCalled.Load())
			require.Equal(t, 1, len(st.broadcaster.BroadcastMessages))
			require.DeepEqual(t, signed, st.broadcaster.BroadcastMessages[0])

			select {
			case ev := <-st.events:
				require.Equal(t, feed.EventType(operation.InclusionListReceived), ev.Type)
				data, ok := ev.Data.(*operation.InclusionListReceivedData)
				require.Equal(t, true, ok)
				require.DeepEqual(t, signed, data.SignedInclusionList)
			default:
				t.Fatal("no inclusion list event")
			}
		})
	}

	for _, tc := range []struct {
		name             string
		body             []byte
		contentType      string
		consensusVersion string
		onErr            error
		msg              string
	}{
		{name: "missing version header", body: jsonBody, contentType: api.JsonMediaType, msg: "header is required"},
		{name: "invalid version header", body: jsonBody, contentType: api.JsonMediaType, consensusVersion: "foo", msg: "Invalid version"},
		{name: "gloas version header", body: jsonBody, contentType: api.JsonMediaType, consensusVersion: "gloas", msg: "only supported from the heze fork"},
		{name: "empty body", body: nil, contentType: api.JsonMediaType, consensusVersion: "heze", msg: "No data submitted"},
		{name: "no data field", body: []byte(`{}`), contentType: api.JsonMediaType, consensusVersion: "heze", msg: "No data submitted"},
		{name: "invalid json", body: []byte(`{"data":{"message":{"slot":"x"}}}`), contentType: api.JsonMediaType, consensusVersion: "heze", msg: "Could not decode request body"},
		{name: "invalid ssz", body: []byte{0x01, 0x02}, contentType: api.OctetStreamMediaType, consensusVersion: "heze", msg: "Could not decode request body"},
		{name: "validation error", body: jsonBody, contentType: api.JsonMediaType, consensusVersion: "heze", onErr: blockchain.ErrInclusionListNotInCommittee, msg: "Invalid inclusion list: " + blockchain.ErrInclusionListNotInCommittee.Error()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := newSetup(t, tc.onErr)
			writer := publish(st.s, tc.body, tc.contentType, tc.consensusVersion)
			require.Equal(t, http.StatusBadRequest, writer.Code)
			require.StringContains(t, tc.msg, writer.Body.String())
			require.Equal(t, false, st.broadcaster.BroadcastCalled.Load())
			require.Equal(t, 0, len(st.events))
		})
	}
}

func TestGetInclusionListDuties(t *testing.T) {
	setHezeForkEpoch(t, 0)
	spe := params.BeaconConfig().SlotsPerEpoch
	numVals := uint64(spe) * 8
	st, _ := util.DeterministicGenesisStateGloas(t, numVals)
	genesisRoot := [32]byte{1, 2, 3}
	db := dbutil.SetupDB(t)
	require.NoError(t, db.SaveGenesisBlockRoot(t.Context(), genesisRoot))

	newServer := func(t *testing.T, currentSlot primitives.Slot, syncing bool) *Server {
		slot := currentSlot
		chain := &mockChain.ChainService{Genesis: time.Now(), State: st, Slot: &slot, Optimistic: true}
		return &Server{
			Stater:                &testutil.MockStater{BeaconState: st},
			SyncChecker:           &mockSync.Sync{IsSyncing: syncing},
			TimeFetcher:           chain,
			HeadFetcher:           chain,
			OptimisticModeFetcher: chain,
			BeaconDB:              db,
			CoreService:           &core.Service{},
		}
	}
	duties := func(s *Server, epoch string, indices []string) *httptest.ResponseRecorder {
		body, err := json.Marshal(indices)
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://example.com/eth/v1/validator/duties/inclusion_list/{epoch}", bytes.NewReader(body))
		request.SetPathValue("epoch", epoch)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		s.GetInclusionListDuties(writer, request)
		return writer
	}
	allIndices := make([]string, numVals)
	for i := range allIndices {
		allIndices[i] = strconv.Itoa(i)
	}

	t.Run("ok matches the spec assignment", func(t *testing.T) {
		writer := duties(newServer(t, 0, false), "0", allIndices)
		require.Equal(t, http.StatusOK, writer.Code, writer.Body.String())
		resp := &structs.GetInclusionListDutiesResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, hexutil.Encode(genesisRoot[:]), resp.DependentRoot)
		assert.Equal(t, true, resp.ExecutionOptimistic)

		want := make(map[string]string)
		for i := range numVals {
			slot, ok, err := helpers.InclusionListCommitteeAssignment(t.Context(), st, 0, primitives.ValidatorIndex(i))
			require.NoError(t, err)
			if ok {
				want[strconv.FormatUint(i, 10)] = strconv.FormatUint(uint64(slot), 10)
			}
		}
		require.NotEqual(t, 0, len(want))
		got := make(map[string]string)
		for _, d := range resp.Data {
			pubkey := st.PubkeyAtIndex(primitives.ValidatorIndex(mustParseUint(t, d.ValidatorIndex)))
			assert.Equal(t, hexutil.Encode(pubkey[:]), d.Pubkey)
			got[d.ValidatorIndex] = d.Slot
		}
		require.DeepEqual(t, want, got)
	})
	t.Run("next epoch", func(t *testing.T) {
		writer := duties(newServer(t, 0, false), "1", []string{"0", "1", "2"})
		require.Equal(t, http.StatusOK, writer.Code, writer.Body.String())
	})
	t.Run("dependent root after epoch 1", func(t *testing.T) {
		stLater := st.Copy()
		require.NoError(t, stLater.SetSlot(2*spe))
		roots := make([][]byte, params.BeaconConfig().SlotsPerHistoricalRoot)
		for i := range roots {
			roots[i] = bytesutil.PadTo([]byte{byte(i), 0x11}, fieldparams.RootLength)
		}
		require.NoError(t, stLater.SetBlockRoots(roots))
		s := newServer(t, 2*spe, false)
		s.Stater = &testutil.MockStater{BeaconState: stLater}
		writer := duties(s, "2", []string{"0"})
		require.Equal(t, http.StatusOK, writer.Code, writer.Body.String())
		resp := &structs.GetInclusionListDutiesResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		// get_block_root_at_slot(state, compute_start_slot_at_epoch(epoch - 1) - 1)
		assert.Equal(t, hexutil.Encode(roots[spe-1]), resp.DependentRoot)
	})
	for _, tc := range []struct {
		name    string
		epoch   string
		indices []string
		msg     string
	}{
		{name: "epoch too far", epoch: "2", indices: []string{"0"}, msg: "Request epoch 2 can not be greater than next epoch 1"},
		{name: "invalid index", epoch: "0", indices: []string{strconv.FormatUint(numVals+1, 10)}, msg: fmt.Sprintf("Invalid validator index %d", numVals+1)},
		{name: "no indices", epoch: "0", indices: []string{}, msg: "No data submitted"},
		{name: "bad index", epoch: "0", indices: []string{"x"}, msg: "ValidatorIndices[0]"},
		{name: "bad epoch", epoch: "x", indices: []string{"0"}, msg: "epoch is invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writer := duties(newServer(t, 0, false), tc.epoch, tc.indices)
			require.Equal(t, http.StatusBadRequest, writer.Code)
			require.StringContains(t, tc.msg, writer.Body.String())
		})
	}
	t.Run("syncing", func(t *testing.T) {
		require.Equal(t, http.StatusServiceUnavailable, duties(newServer(t, 0, true), "0", []string{"0"}).Code)
	})
	// Last, because the config override stays for the rest of the parent test.
	t.Run("pre-Heze epoch", func(t *testing.T) {
		setHezeForkEpoch(t, 1)
		writer := duties(newServer(t, 0, false), "0", []string{"0"})
		require.Equal(t, http.StatusBadRequest, writer.Code)
		require.StringContains(t, "not available before the Heze fork", writer.Body.String())
	})
}

func mustParseUint(t *testing.T, s string) uint64 {
	v, err := strconv.ParseUint(s, 10, 64)
	require.NoError(t, err)
	return v
}

func TestInclusionListTransactions(t *testing.T) {
	setHezeForkEpoch(t, 0)
	require.Equal(t, 0, len(inclusionListTransactions(nil)))
	got := inclusionListTransactions([][]byte{{0x03}, {}, {0x01}})
	require.DeepEqual(t, [][]byte{{0x01}}, got)
	require.Equal(t, uint64(8192), params.BeaconConfig().MaxTransactionsBytesPerInclusionList)
}
