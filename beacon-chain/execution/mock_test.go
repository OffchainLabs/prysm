package execution

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rpc"
	pb "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

var mockHandlerDefaultName = "__default__"

type jsonError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

type jsonrpcMessage struct {
	Version string          `json:"jsonrpc,omitempty"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Error   *jsonError      `json:"error,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
}

type mockHandler func(*jsonrpcMessage, http.ResponseWriter, *http.Request)

type mockEngine struct {
	t        *testing.T
	handlers map[string]mockHandler
	calls    map[string][]*jsonrpcMessage
}

func newMockEngine(t *testing.T) (*rpc.Client, *mockEngine) {
	s := &mockEngine{t: t, handlers: make(map[string]mockHandler), calls: make(map[string][]*jsonrpcMessage)}
	srv := httptest.NewServer(s)
	c, err := rpc.DialHTTP(srv.URL)
	require.NoError(t, err)
	return c, s
}

func (s *mockEngine) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	msg := &jsonrpcMessage{}
	defer func() {
		s.calls[msg.Method] = append(s.calls[msg.Method], msg)
	}()
	if err := json.NewDecoder(r.Body).Decode(msg); err != nil {
		http.Error(w, "failed to decode request: "+err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	defer func() {
		require.NoError(s.t, r.Body.Close())
	}()
	handler, ok := s.handlers[msg.Method]
	if !ok {
		// Fallback to default handler if it is registered.
		handler, ok = s.handlers[mockHandlerDefaultName]
		if !ok {
			s.t.Fatalf("mockEngine called with unexpected method %s", msg.Method)
		}
	}
	handler(msg, w, r)
}

func (s *mockEngine) register(method string, handler mockHandler) {
	s.handlers[method] = handler
}

func (s *mockEngine) registerDefault(handler mockHandler) {
	s.handlers[mockHandlerDefaultName] = handler
}

func (s *mockEngine) callCount(method string) int {
	return len(s.calls[method])
}

func mockParseUintList(t *testing.T, data json.RawMessage) []uint64 {
	var list []string
	if err := json.Unmarshal(data, &list); err != nil {
		t.Fatalf("failed to parse uint list: %v", err)
	}
	uints := make([]uint64, len(list))
	for i, u := range list {
		uints[i] = hexutil.MustDecodeUint64(u)
	}
	return uints
}

func mockParseHexByteList(t *testing.T, data json.RawMessage) []hexutil.Bytes {
	var list [][]hexutil.Bytes
	if err := json.Unmarshal(data, &list); err != nil {
		t.Fatalf("failed to parse hex byte list: %v", err)
	}
	require.Equal(t, 1, len(list))
	return list[0]
}

func strToHexBytes(t *testing.T, s string) hexutil.Bytes {
	b := hexutil.Bytes{}
	require.NoError(t, b.UnmarshalText([]byte(s)))
	return b
}

func mockWriteResult(t *testing.T, w http.ResponseWriter, req *jsonrpcMessage, result any) {
	marshaled, err := json.Marshal(result)
	require.NoError(t, err)
	req.Result = marshaled
	require.NoError(t, json.NewEncoder(w).Encode(req))
}

func TestParseRequest(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		method   string
		hexArgs  []string // uint64 as hex
		byteArgs []hexutil.Bytes
	}{
		{
			method: GetPayloadBodiesByHashV1,
			byteArgs: []hexutil.Bytes{
				strToHexBytes(t, "0x656d707479000000000000000000000000000000000000000000000000000000"),
				strToHexBytes(t, "0x66756c6c00000000000000000000000000000000000000000000000000000000"),
			},
		},
		{
			method:  GetPayloadBodiesByRangeV1,
			hexArgs: []string{hexutil.EncodeUint64(0), hexutil.EncodeUint64(1)},
		},
	}
	for _, c := range cases {
		t.Run(c.method, func(t *testing.T) {
			cli, srv := newMockEngine(t)
			srv.register(c.method, func(msg *jsonrpcMessage, w http.ResponseWriter, _ *http.Request) {
				require.Equal(t, c.method, msg.Method)
				nr := uint64(len(c.byteArgs))
				if len(c.byteArgs) > 0 {
					require.DeepEqual(t, c.byteArgs, mockParseHexByteList(t, msg.Params))
				}
				if len(c.hexArgs) > 0 {
					rang := mockParseUintList(t, msg.Params)
					for i, r := range rang {
						require.Equal(t, c.hexArgs[i], hexutil.EncodeUint64(r))
					}
					nr = rang[1]
				}
				mockWriteResult(t, w, msg, make([]*pb.ExecutionPayloadBody, nr))
			})

			result := make([]*pb.ExecutionPayloadBody, 0)
			var args []interface{}
			if len(c.byteArgs) > 0 {
				args = []interface{}{c.byteArgs}
			}
			if len(c.hexArgs) > 0 {
				args = make([]interface{}, len(c.hexArgs))
				for i := range c.hexArgs {
					args[i] = c.hexArgs[i]
				}
			}
			require.NoError(t, cli.CallContext(ctx, &result, c.method, args...))
			if len(c.byteArgs) > 0 {
				require.Equal(t, len(c.byteArgs), len(result))
			}
			if len(c.hexArgs) > 0 {
				require.Equal(t, int(hexutil.MustDecodeUint64(c.hexArgs[1])), len(result))
			}
		})
	}
}

func TestCallCount(t *testing.T) {
	methods := []string{
		GetPayloadBodiesByHashV1,
		GetPayloadBodiesByRangeV1,
	}
	cases := []struct {
		method string
		count  int
	}{
		{method: GetPayloadBodiesByHashV1, count: 1},
		{method: GetPayloadBodiesByHashV1, count: 2},
		{method: GetPayloadBodiesByRangeV1, count: 1},
		{method: GetPayloadBodiesByRangeV1, count: 2},
	}
	for _, c := range cases {
		t.Run(c.method, func(t *testing.T) {
			cli, srv := newMockEngine(t)
			srv.register(c.method, func(msg *jsonrpcMessage, w http.ResponseWriter, _ *http.Request) {
				mockWriteResult(t, w, msg, nil)
			})
			for i := 0; i < c.count; i++ {
				require.NoError(t, cli.CallContext(context.Background(), nil, c.method))
			}
			for _, m := range methods {
				if m == c.method {
					require.Equal(t, c.count, srv.callCount(m))
				} else {
					require.Equal(t, 0, srv.callCount(m))
				}
			}
		})
	}
}
