package execution

import (
	"bytes"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"

	pb "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/holiman/uint256"
)

func TestHeaderByHash_NotFound(t *testing.T) {
	srv := &Service{}
	srv.rpcClient = RPCClientBad{}

	_, err := srv.HeaderByHash(t.Context(), [32]byte{})
	assert.Equal(t, ethereum.NotFound, err)
}

func TestHeaderByNumber_NotFound(t *testing.T) {
	srv := &Service{}
	srv.rpcClient = RPCClientBad{}

	_, err := srv.HeaderByNumber(t.Context(), big.NewInt(100))
	assert.Equal(t, ethereum.NotFound, err)
}

// jsonRPCResultServer answers every request, batched or not, with the given result.
func jsonRPCResultServer(t *testing.T, result any) *httptest.Server {
	type request struct {
		ID json.RawMessage `json:"id"`
	}
	respond := func(id json.RawMessage) map[string]any {
		return map[string]any{"jsonrpc": "2.0", "id": id, "result": result}
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		defer func() {
			require.NoError(t, r.Body.Close())
		}()
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		body = bytes.TrimLeft(body, " \t\r\n")

		if len(body) > 0 && body[0] == '[' {
			var reqs []request
			require.NoError(t, json.Unmarshal(body, &reqs))
			resps := make([]map[string]any, len(reqs))
			for i, req := range reqs {
				resps[i] = respond(req.ID)
			}
			require.NoError(t, json.NewEncoder(w).Encode(resps))
			return
		}
		var req request
		require.NoError(t, json.Unmarshal(body, &req))
		require.NoError(t, json.NewEncoder(w).Encode(respond(req.ID)))
	}))
}

func serviceWithHTTPClient(t *testing.T, srv *httptest.Server) *Service {
	rpcClient, err := rpc.DialHTTP(srv.URL)
	require.NoError(t, err)
	t.Cleanup(rpcClient.Close)

	s := &Service{}
	s.rpcClient = rpcClient
	return s
}

func TestExecutionBlockByHash(t *testing.T) {
	t.Run("block is returned", func(t *testing.T) {
		want, ok := fixtures()["ExecutionBlock"].(*pb.ExecutionBlock)
		require.Equal(t, true, ok)
		srv := jsonRPCResultServer(t, want)
		defer srv.Close()

		blk, err := serviceWithHTTPClient(t, srv).ExecutionBlockByHash(t.Context(), common.BytesToHash([]byte("foo")), false)
		require.NoError(t, err)
		require.DeepEqual(t, want, blk)
	})
	t.Run("null result is reported as not found", func(t *testing.T) {
		srv := jsonRPCResultServer(t, nil)
		defer srv.Close()

		blk, err := serviceWithHTTPClient(t, srv).ExecutionBlockByHash(t.Context(), common.BytesToHash([]byte("foo")), false)
		require.ErrorIs(t, err, ethereum.NotFound)
		require.IsNil(t, blk)
	})
}

func TestExecutionBlocksByHashes(t *testing.T) {
	hashes := []common.Hash{common.BytesToHash([]byte("foo")), common.BytesToHash([]byte("bar"))}

	t.Run("no hashes requested", func(t *testing.T) {
		srv := jsonRPCResultServer(t, nil)
		defer srv.Close()

		blks, err := serviceWithHTTPClient(t, srv).ExecutionBlocksByHashes(t.Context(), nil, false)
		require.NoError(t, err)
		require.Equal(t, 0, len(blks))
	})
	t.Run("blocks are returned", func(t *testing.T) {
		want, ok := fixtures()["ExecutionBlock"].(*pb.ExecutionBlock)
		require.Equal(t, true, ok)
		srv := jsonRPCResultServer(t, want)
		defer srv.Close()

		blks, err := serviceWithHTTPClient(t, srv).ExecutionBlocksByHashes(t.Context(), hashes, false)
		require.NoError(t, err)
		require.Equal(t, len(hashes), len(blks))
		for _, blk := range blks {
			require.DeepEqual(t, want, blk)
		}
	})
	t.Run("null result is reported as not found", func(t *testing.T) {
		srv := jsonRPCResultServer(t, nil)
		defer srv.Close()

		blks, err := serviceWithHTTPClient(t, srv).ExecutionBlocksByHashes(t.Context(), hashes, false)
		require.ErrorIs(t, err, ethereum.NotFound)
		require.IsNil(t, blks)
	})
}

func Test_tDStringToUint256(t *testing.T) {
	i, err := tDStringToUint256("0x0")
	require.NoError(t, err)
	require.DeepEqual(t, uint256.NewInt(0), i)

	i, err = tDStringToUint256("0x10000")
	require.NoError(t, err)
	require.DeepEqual(t, uint256.NewInt(65536), i)

	_, err = tDStringToUint256("100")
	require.ErrorContains(t, "hex string without 0x prefix", err)

	_, err = tDStringToUint256("0xzzzzzz")
	require.ErrorContains(t, "invalid hex string", err)

	_, err = tDStringToUint256("0xFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF" +
		"FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF")
	require.ErrorContains(t, "hex number > 256 bits", err)
}

func TestToBlockNumArg(t *testing.T) {
	tests := []struct {
		name   string
		number *big.Int
		want   string
	}{
		{
			name:   "genesis",
			number: big.NewInt(0),
			want:   "0x0",
		},
		{
			name:   "near genesis block",
			number: big.NewInt(300),
			want:   "0x12c",
		},
		{
			name:   "current block",
			number: big.NewInt(15838075),
			want:   "0xf1ab7b",
		},
		{
			name:   "far off block",
			number: big.NewInt(12032894823020),
			want:   "0xaf1a06bea6c",
		},
		{
			name:   "latest block",
			number: nil,
			want:   "latest",
		},
		{
			name:   "pending block",
			number: big.NewInt(-1),
			want:   "pending",
		},
		{
			name:   "finalized block",
			number: big.NewInt(-3),
			want:   "finalized",
		},
		{
			name:   "safe block",
			number: big.NewInt(-4),
			want:   "safe",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := toBlockNumArg(tt.number); got != tt.want {
				t.Errorf("toBlockNumArg() = %v, want %v", got, tt.want)
			}
		})
	}
}
