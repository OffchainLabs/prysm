package execution

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	payloadattribute "github.com/OffchainLabs/prysm/v7/consensus-types/payload-attribute"
	pb "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rpc"
)

type capturedRPC struct {
	Method string            `json:"method"`
	Params []json.RawMessage `json:"params"`
}

// hezeEngineService returns a Service whose EL answers every call with result, or with errCode when it is not zero.
func hezeEngineService(t *testing.T, result any, errCode int, got *capturedRPC) *Service {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var req struct {
			capturedRPC
			ID json.RawMessage `json:"id"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		*got = req.capturedRPC
		resp := map[string]any{"jsonrpc": "2.0", "id": req.ID}
		if errCode != 0 {
			resp["error"] = map[string]any{"code": errCode, "message": "Unsupported fork"}
		} else {
			resp["result"] = result
		}
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	t.Cleanup(srv.Close)
	rpcClient, err := rpc.DialHTTP(srv.URL)
	require.NoError(t, err)
	t.Cleanup(rpcClient.Close)
	return &Service{rpcClient: rpcClient}
}

func TestNewPayloadWithInclusionList(t *testing.T) {
	payload, err := blocks.WrappedExecutionPayloadGloas(&pb.ExecutionPayloadGloas{
		ParentHash:    make([]byte, 32),
		FeeRecipient:  make([]byte, 20),
		StateRoot:     make([]byte, 32),
		ReceiptsRoot:  make([]byte, 32),
		LogsBloom:     make([]byte, 256),
		PrevRandao:    make([]byte, 32),
		BaseFeePerGas: make([]byte, 32),
		BlockHash:     make([]byte, 32),
		Transactions:  &pb.ProgressiveTransactionList{},
	})
	require.NoError(t, err)
	lvh := common.Hash{'a'}
	yes, no := true, false
	status := func(s string, satisfied any) map[string]any {
		return map[string]any{"status": s, "latestValidHash": lvh, "validationError": nil, "inclusionListSatisfied": satisfied}
	}

	for _, tc := range []struct {
		name          string
		result        any
		errCode       int
		wantErr       error
		wantLVH       []byte
		wantSatisfied *bool
	}{
		{name: "VALID and satisfied", result: status("VALID", true), wantLVH: lvh[:], wantSatisfied: &yes},
		{name: "VALID and not satisfied", result: status("VALID", false), wantLVH: lvh[:], wantSatisfied: &no},
		{name: "VALID and null", result: status("VALID", nil), wantLVH: lvh[:]},
		{name: "SYNCING", result: status("SYNCING", nil), wantErr: ErrAcceptedSyncingPayloadStatus},
		{name: "INVALID ignores the flag", result: status("INVALID", true), wantErr: ErrInvalidPayloadStatus, wantLVH: lvh[:]},
		{name: "unsupported fork", errCode: -38005, wantErr: ErrUnsupportedFork},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got capturedRPC
			s := hezeEngineService(t, tc.result, tc.errCode, &got)
			gotLVH, satisfied, err := s.NewPayloadWithInclusionList(t.Context(), payload, []common.Hash{}, &common.Hash{'b'}, &pb.ExecutionRequestsGloas{}, [][]byte{{0x01, 0x02}})
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
			}
			require.DeepEqual(t, tc.wantLVH, gotLVH)
			require.DeepEqual(t, tc.wantSatisfied, satisfied)
			require.Equal(t, NewPayloadMethodV6, got.Method)
			require.Equal(t, 5, len(got.Params))
			require.Equal(t, `["0x0102"]`, string(got.Params[4]))
		})
	}

	t.Run("nil inclusion list is sent as an empty array", func(t *testing.T) {
		var got capturedRPC
		s := hezeEngineService(t, status("VALID", true), 0, &got)
		_, _, err := s.NewPayloadWithInclusionList(t.Context(), payload, []common.Hash{}, &common.Hash{'b'}, &pb.ExecutionRequestsGloas{}, nil)
		require.NoError(t, err)
		require.Equal(t, `[]`, string(got.Params[4]))
	})

	t.Run("blob hashes, parent root, and requests are on the wire", func(t *testing.T) {
		var got capturedRPC
		s := hezeEngineService(t, status("VALID", true), 0, &got)
		requests := &pb.ExecutionRequestsGloas{
			Withdrawals: []*pb.WithdrawalRequest{{
				SourceAddress:   bytes.Repeat([]byte{0x11}, 20),
				ValidatorPubkey: bytes.Repeat([]byte{0x22}, 48),
				Amount:          5,
			}},
			Consolidations: []*pb.ConsolidationRequest{{
				SourceAddress: bytes.Repeat([]byte{0x33}, 20),
				SourcePubkey:  bytes.Repeat([]byte{0x44}, 48),
				TargetPubkey:  bytes.Repeat([]byte{0x55}, 48),
			}},
		}
		hashes := []common.Hash{{0x01, 0x02}, {0x03}}
		_, _, err := s.NewPayloadWithInclusionList(t.Context(), payload, hashes, &common.Hash{'b'}, requests, nil)
		require.NoError(t, err)
		require.Equal(t, 5, len(got.Params))

		wantHashes := `["0x0102` + strings.Repeat("00", 30) + `","0x03` + strings.Repeat("00", 31) + `"]`
		require.Equal(t, wantHashes, string(got.Params[1]))
		require.Equal(t, `"0x62`+strings.Repeat("00", 31)+`"`, string(got.Params[2]))
		withdrawal := "0x01" + strings.Repeat("11", 20) + strings.Repeat("22", 48) + "0500000000000000"
		consolidation := "0x02" + strings.Repeat("33", 20) + strings.Repeat("44", 48) + strings.Repeat("55", 48)
		require.Equal(t, `["`+withdrawal+`","`+consolidation+`"]`, string(got.Params[3]))
	})
}

func TestForkchoiceUpdated_Heze(t *testing.T) {
	fcs := &pb.ForkchoiceState{HeadBlockHash: make([]byte, 32), SafeBlockHash: make([]byte, 32), FinalizedBlockHash: make([]byte, 32)}
	lvh := common.Hash{'a'}
	result := map[string]any{
		"payloadStatus": map[string]any{"status": "VALID", "latestValidHash": lvh, "validationError": nil, "inclusionListSatisfied": true},
		"payloadId":     "0x0000000000000001",
	}
	v5 := &pb.PayloadAttributesV5{
		Timestamp:                 1,
		PrevRandao:                make([]byte, 32),
		SuggestedFeeRecipient:     make([]byte, 20),
		Withdrawals:               []*pb.Withdrawal{},
		ParentBeaconBlockRoot:     make([]byte, 32),
		SlotNumber:                2,
		TargetGasLimit:            3,
		InclusionListTransactions: [][]byte{{0x01}},
	}

	t.Run("Heze attributes use V5", func(t *testing.T) {
		var got capturedRPC
		s := hezeEngineService(t, result, 0, &got)
		attrs, err := payloadattribute.New(v5)
		require.NoError(t, err)
		id, gotLVH, satisfied, err := s.ForkchoiceUpdated(t.Context(), fcs, attrs)
		require.NoError(t, err)
		require.DeepEqual(t, &pb.PayloadIDBytes{0, 0, 0, 0, 0, 0, 0, 1}, id)
		require.DeepEqual(t, lvh[:], gotLVH)
		require.NotNil(t, satisfied)
		require.Equal(t, true, *satisfied)
		require.Equal(t, ForkchoiceUpdatedMethodV5, got.Method)
		require.Equal(t, 2, len(got.Params))
		onWire := &pb.PayloadAttributesV5{}
		require.NoError(t, json.Unmarshal(got.Params[1], onWire))
		require.DeepEqual(t, v5, onWire)
	})
	t.Run("Gloas attributes still use V4", func(t *testing.T) {
		var got capturedRPC
		s := hezeEngineService(t, result, 0, &got)
		attrs, err := payloadattribute.New(&pb.PayloadAttributesV4{Timestamp: 1, PrevRandao: make([]byte, 32), ParentBeaconBlockRoot: make([]byte, 32)})
		require.NoError(t, err)
		_, _, _, err = s.ForkchoiceUpdated(t.Context(), fcs, attrs)
		require.NoError(t, err)
		require.Equal(t, ForkchoiceUpdatedMethodV4, got.Method)
	})
	for _, tc := range []struct {
		name   string
		status map[string]any
		want   *bool
	}{
		{name: "VALID with false", status: map[string]any{"status": "VALID", "latestValidHash": lvh, "inclusionListSatisfied": false}, want: new(bool)},
		{name: "VALID with null", status: map[string]any{"status": "VALID", "latestValidHash": lvh, "inclusionListSatisfied": nil}},
		{name: "SYNCING drops the flag", status: map[string]any{"status": "SYNCING", "latestValidHash": nil, "inclusionListSatisfied": true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got capturedRPC
			s := hezeEngineService(t, map[string]any{"payloadStatus": tc.status, "payloadId": nil}, 0, &got)
			_, _, satisfied, err := s.ForkchoiceUpdated(t.Context(), fcs, payloadattribute.EmptyWithVersion(version.Heze))
			if tc.status["status"] == "SYNCING" {
				require.ErrorIs(t, err, ErrAcceptedSyncingPayloadStatus)
			} else {
				require.NoError(t, err)
			}
			require.DeepEqual(t, tc.want, satisfied)
		})
	}
	t.Run("unsupported fork", func(t *testing.T) {
		var got capturedRPC
		s := hezeEngineService(t, nil, -38005, &got)
		_, _, _, err := s.ForkchoiceUpdated(t.Context(), fcs, payloadattribute.EmptyWithVersion(version.Heze))
		require.ErrorIs(t, err, ErrUnsupportedFork)
		require.Equal(t, ForkchoiceUpdatedMethodV5, got.Method)
		require.Equal(t, `null`, string(got.Params[1]))
	})
}

func TestGetInclusionList(t *testing.T) {
	maxBytes := params.BeaconConfig().MaxTransactionsBytesPerInclusionList
	atLimit := make([]byte, maxBytes)
	atLimit[0] = 0x02

	for _, tc := range []struct {
		name        string
		result      []string
		errCode     int
		want        [][]byte
		wantErr     error
		wantContain string
	}{
		{name: "ok", result: []string{"0x01", "0x0203"}, want: [][]byte{{0x01}, {0x02, 0x03}}},
		{name: "empty list", result: []string{}, want: [][]byte{}},
		{name: "at the byte limit", result: []string{hexutil.Encode(atLimit)}, want: [][]byte{atLimit}},
		{name: "empty tx", result: []string{"0x01", "0x"}, wantContain: "inclusion list transaction 1 is empty"},
		{name: "oversize", result: []string{hexutil.Encode(atLimit), "0x01"}, wantContain: "inclusion list transactions exceed 8192 bytes"},
		{name: "unsupported fork", errCode: -38005, wantErr: ErrUnsupportedFork},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got capturedRPC
			s := hezeEngineService(t, tc.result, tc.errCode, &got)
			txs, err := s.GetInclusionList(t.Context())
			require.Equal(t, GetInclusionListV1, got.Method)
			require.Equal(t, 0, len(got.Params))
			switch {
			case tc.wantErr != nil:
				require.ErrorIs(t, err, tc.wantErr)
			case tc.wantContain != "":
				require.ErrorContains(t, tc.wantContain, err)
				require.Equal(t, true, txs == nil)
			default:
				require.NoError(t, err)
				require.DeepEqual(t, tc.want, txs)
			}
		})
	}
}
