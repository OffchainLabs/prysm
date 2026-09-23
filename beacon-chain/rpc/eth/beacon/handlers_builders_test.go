package beacon

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	chainMock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/eth/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/testutil"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

const buildersTestFinalizedEpoch = 5

// gloasStateWithBuilders returns a Gloas state with a finalized epoch of 5 and four builders:
// index 0 is active, index 1 is pending, index 2 is exited and index 3 is active.
func gloasStateWithBuilders(t *testing.T) (state.BeaconState, []*eth.Builder) {
	farFutureEpoch := params.BeaconConfig().FarFutureEpoch
	newBuilder := func(i int, depositEpoch, withdrawableEpoch primitives.Epoch) *eth.Builder {
		pubkey := make([]byte, 48)
		pubkey[0] = byte(i + 1)
		addr := make([]byte, 20)
		addr[0] = byte(i + 1)
		return &eth.Builder{
			Pubkey:            pubkey,
			Version:           []byte{byte(i)},
			ExecutionAddress:  addr,
			Balance:           primitives.Gwei(32000000000 + i),
			DepositEpoch:      depositEpoch,
			WithdrawableEpoch: withdrawableEpoch,
		}
	}
	builders := []*eth.Builder{
		newBuilder(0, 0, farFutureEpoch),
		newBuilder(1, buildersTestFinalizedEpoch, farFutureEpoch),
		newBuilder(2, 0, 10),
		newBuilder(3, 2, farFutureEpoch),
	}
	st, err := util.NewBeaconStateGloas(func(s *eth.BeaconStateGloas) error {
		s.Builders = builders
		s.FinalizedCheckpoint = &eth.Checkpoint{Epoch: buildersTestFinalizedEpoch, Root: make([]byte, 32)}
		return nil
	})
	require.NoError(t, err)
	return st, builders
}

func TestGetStateBuilders(t *testing.T) {
	st, builders := gloasStateWithBuilders(t)

	newServer := func(chainService *chainMock.ChainService) *Server {
		return &Server{
			Stater: &testutil.MockStater{
				BeaconState: st,
			},
			HeadFetcher:           chainService,
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
		}
	}
	getBuilders := func(t *testing.T, s *Server, body string) *httptest.ResponseRecorder {
		var bodyReader *bytes.Buffer
		if body == "" {
			bodyReader = &bytes.Buffer{}
		} else {
			bodyReader = bytes.NewBufferString(body)
		}
		request := httptest.NewRequest(http.MethodPost, "http://example.com/eth/v1/beacon/states/{state_id}/builders", bodyReader)
		request.SetPathValue("state_id", "head")
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		s.GetStateBuilders(writer, request)
		return writer
	}

	t.Run("get all with empty body", func(t *testing.T) {
		writer := getBuilders(t, newServer(&chainMock.ChainService{}), "")
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetStateBuildersResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.Equal(t, 4, len(resp.Data))

		expectedStatuses := []string{"active", "pending", "exited", "active"}
		for i, b := range resp.Data {
			assert.Equal(t, fmt.Sprintf("%d", i), b.Index)
			assert.Equal(t, expectedStatuses[i], b.Status)
			require.NotNil(t, b.Builder)
			assert.Equal(t, hexutil.Encode(builders[i].Pubkey), b.Builder.Pubkey)
			assert.Equal(t, fmt.Sprintf("%d", i), b.Builder.Version)
			assert.Equal(t, hexutil.Encode(builders[i].ExecutionAddress), b.Builder.ExecutionAddress)
			assert.Equal(t, fmt.Sprintf("%d", builders[i].Balance), b.Builder.Balance)
			assert.Equal(t, fmt.Sprintf("%d", builders[i].DepositEpoch), b.Builder.DepositEpoch)
			assert.Equal(t, fmt.Sprintf("%d", builders[i].WithdrawableEpoch), b.Builder.WithdrawableEpoch)
		}
		assert.Equal(t, "18446744073709551615", resp.Data[0].Builder.WithdrawableEpoch)
		assert.Equal(t, "10", resp.Data[2].Builder.WithdrawableEpoch)
	})
	t.Run("get all with empty JSON object", func(t *testing.T) {
		writer := getBuilders(t, newServer(&chainMock.ChainService{}), "{}")
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetStateBuildersResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, 4, len(resp.Data))
	})
	t.Run("get by index and pubkey", func(t *testing.T) {
		body := fmt.Sprintf(`{"ids":["2","%s"]}`, hexutil.Encode(builders[0].Pubkey))
		writer := getBuilders(t, newServer(&chainMock.ChainService{}), body)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetStateBuildersResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.Equal(t, 2, len(resp.Data))
		assert.Equal(t, "2", resp.Data[0].Index)
		assert.Equal(t, "0", resp.Data[1].Index)
	})
	t.Run("unknown ids are ignored", func(t *testing.T) {
		unknownPubkey := make([]byte, 48)
		unknownPubkey[0] = 0xff
		body := fmt.Sprintf(`{"ids":["99","%s"]}`, hexutil.Encode(unknownPubkey))
		writer := getBuilders(t, newServer(&chainMock.ChainService{}), body)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetStateBuildersResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, 0, len(resp.Data))
	})
	t.Run("invalid index", func(t *testing.T) {
		writer := getBuilders(t, newServer(&chainMock.ChainService{}), `{"ids":["foo"]}`)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &httputil.DefaultJsonError{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
		assert.StringContains(t, "Invalid builder ID foo", e.Message)
	})
	t.Run("invalid pubkey length", func(t *testing.T) {
		writer := getBuilders(t, newServer(&chainMock.ChainService{}), `{"ids":["0x1234"]}`)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &httputil.DefaultJsonError{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
		assert.StringContains(t, "Pubkey length is 2 instead of 48", e.Message)
	})
	t.Run("malformed body", func(t *testing.T) {
		writer := getBuilders(t, newServer(&chainMock.ChainService{}), `{"ids":"0"}`)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &httputil.DefaultJsonError{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.StringContains(t, "Could not decode request body", e.Message)
	})
	t.Run("status filter", func(t *testing.T) {
		writer := getBuilders(t, newServer(&chainMock.ChainService{}), `{"statuses":["active"]}`)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetStateBuildersResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.Equal(t, 2, len(resp.Data))
		assert.Equal(t, "0", resp.Data[0].Index)
		assert.Equal(t, "3", resp.Data[1].Index)
	})
	t.Run("multiple statuses with mixed case", func(t *testing.T) {
		writer := getBuilders(t, newServer(&chainMock.ChainService{}), `{"statuses":["PENDING","exited"]}`)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetStateBuildersResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.Equal(t, 2, len(resp.Data))
		assert.Equal(t, "1", resp.Data[0].Index)
		assert.Equal(t, "pending", resp.Data[0].Status)
		assert.Equal(t, "2", resp.Data[1].Index)
		assert.Equal(t, "exited", resp.Data[1].Status)
	})
	t.Run("invalid status", func(t *testing.T) {
		writer := getBuilders(t, newServer(&chainMock.ChainService{}), `{"statuses":["slashed"]}`)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &httputil.DefaultJsonError{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.StringContains(t, "Invalid status slashed", e.Message)
	})
	t.Run("ids and statuses combined", func(t *testing.T) {
		writer := getBuilders(t, newServer(&chainMock.ChainService{}), `{"ids":["0","1"],"statuses":["pending"]}`)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetStateBuildersResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.Equal(t, 1, len(resp.Data))
		assert.Equal(t, "1", resp.Data[0].Index)
	})
	t.Run("execution optimistic", func(t *testing.T) {
		writer := getBuilders(t, newServer(&chainMock.ChainService{Optimistic: true}), "")
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetStateBuildersResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, true, resp.ExecutionOptimistic)
	})
	t.Run("finalized", func(t *testing.T) {
		headerRoot, err := helpers.BlockRootFromState(t.Context(), st)
		require.NoError(t, err)
		chainService := &chainMock.ChainService{
			FinalizedRoots: map[[32]byte]bool{
				headerRoot: true,
			},
		}
		writer := getBuilders(t, newServer(chainService), "")
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetStateBuildersResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, true, resp.Finalized)
	})
	t.Run("state prior to gloas", func(t *testing.T) {
		fuluSt, err := util.NewBeaconStateFulu()
		require.NoError(t, err)
		chainService := &chainMock.ChainService{}
		s := &Server{
			Stater: &testutil.MockStater{
				BeaconState: fuluSt,
			},
			HeadFetcher:           chainService,
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
		}
		writer := getBuilders(t, s, "")
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &httputil.DefaultJsonError{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.StringContains(t, "state_id is prior to gloas", e.Message)
	})
	t.Run("no state_id", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPost, "http://example.com/eth/v1/beacon/states/{state_id}/builders", &bytes.Buffer{})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		newServer(&chainMock.ChainService{}).GetStateBuilders(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &httputil.DefaultJsonError{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.StringContains(t, "state_id is required in URL params", e.Message)
	})
}
