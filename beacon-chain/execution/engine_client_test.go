package execution

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/holiman/uint256"
	"github.com/pkg/errors"
	mocks "github.com/prysmaticlabs/prysm/v5/beacon-chain/execution/testing"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/verification"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	payloadattribute "github.com/prysmaticlabs/prysm/v5/consensus-types/payload-attribute"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	pb "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
	logTest "github.com/sirupsen/logrus/hooks/test"
)

var (
	_ = Reconstructor(&Service{})
	_ = EngineCaller(&Service{})
	_ = Reconstructor(&Service{})
	_ = EngineCaller(&mocks.EngineClient{})
)

type RPCClientBad struct {
}

func (RPCClientBad) Close() {}
func (RPCClientBad) BatchCall([]rpc.BatchElem) error {
	return errors.New("rpc client is not initialized")
}

func (RPCClientBad) CallContext(context.Context, interface{}, string, ...interface{}) error {
	return ethereum.NotFound
}

func TestClient_IPC(t *testing.T) {
	t.Skip("Skipping IPC test to support Capella devnet-3")
	server := newTestIPCServer(t)
	defer server.Stop()
	rpcClient := rpc.DialInProc(server)
	defer rpcClient.Close()
	srv := &Service{}
	srv.rpcClient = rpcClient
	ctx := context.Background()
	fix := fixtures()

	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.CapellaForkEpoch = 1
	params.OverrideBeaconConfig(cfg)

	t.Run(GetPayloadMethod, func(t *testing.T) {
		want, ok := fix["ExecutionPayload"].(*pb.ExecutionPayload)
		require.Equal(t, true, ok)
		payloadId := [8]byte{1}
		resp, err := srv.GetPayload(ctx, payloadId, 1)
		require.NoError(t, err)
		require.Equal(t, false, resp.OverrideBuilder)
		pbs := resp.ExecutionData.Proto()
		resPb, ok := pbs.(*pb.ExecutionPayload)
		require.Equal(t, true, ok)
		require.NoError(t, err)
		require.DeepEqual(t, want, resPb)
	})
	t.Run(GetPayloadMethodV2, func(t *testing.T) {
		want, ok := fix["ExecutionPayloadCapellaWithValue"].(*pb.ExecutionPayloadCapellaWithValue)
		require.Equal(t, true, ok)
		payloadId := [8]byte{1}
		resp, err := srv.GetPayload(ctx, payloadId, params.BeaconConfig().SlotsPerEpoch)
		require.NoError(t, err)
		require.Equal(t, false, resp.OverrideBuilder)
		pbs := resp.ExecutionData.Proto()
		resPb, ok := pbs.(*pb.ExecutionPayloadCapella)
		require.Equal(t, true, ok)
		require.DeepEqual(t, want, resPb)
	})
	t.Run(ForkchoiceUpdatedMethod, func(t *testing.T) {
		want, ok := fix["ForkchoiceUpdatedResponse"].(*ForkchoiceUpdatedResponse)
		require.Equal(t, true, ok)
		p, err := payloadattribute.New(&pb.PayloadAttributes{})
		require.NoError(t, err)
		payloadID, validHash, err := srv.ForkchoiceUpdated(ctx, &pb.ForkchoiceState{}, p)
		require.NoError(t, err)
		require.DeepEqual(t, want.Status.LatestValidHash, validHash)
		require.DeepEqual(t, want.PayloadId, payloadID)
	})
	t.Run(ForkchoiceUpdatedMethodV2, func(t *testing.T) {
		want, ok := fix["ForkchoiceUpdatedResponse"].(*ForkchoiceUpdatedResponse)
		require.Equal(t, true, ok)
		p, err := payloadattribute.New(&pb.PayloadAttributesV2{})
		require.NoError(t, err)
		payloadID, validHash, err := srv.ForkchoiceUpdated(ctx, &pb.ForkchoiceState{}, p)
		require.NoError(t, err)
		require.DeepEqual(t, want.Status.LatestValidHash, validHash)
		require.DeepEqual(t, want.PayloadId, payloadID)
	})
	t.Run(NewPayloadMethod, func(t *testing.T) {
		want, ok := fix["ValidPayloadStatus"].(*pb.PayloadStatus)
		require.Equal(t, true, ok)
		req, ok := fix["ExecutionPayload"].(*pb.ExecutionPayload)
		require.Equal(t, true, ok)
		wrappedPayload, err := blocks.WrappedExecutionPayload(req)
		require.NoError(t, err)
		latestValidHash, err := srv.NewPayload(ctx, wrappedPayload, []common.Hash{}, &common.Hash{}, nil)
		require.NoError(t, err)
		require.DeepEqual(t, bytesutil.ToBytes32(want.LatestValidHash), bytesutil.ToBytes32(latestValidHash))
	})
	t.Run(NewPayloadMethodV2, func(t *testing.T) {
		want, ok := fix["ValidPayloadStatus"].(*pb.PayloadStatus)
		require.Equal(t, true, ok)
		req, ok := fix["ExecutionPayloadCapella"].(*pb.ExecutionPayloadCapella)
		require.Equal(t, true, ok)
		wrappedPayload, err := blocks.WrappedExecutionPayloadCapella(req)
		require.NoError(t, err)
		latestValidHash, err := srv.NewPayload(ctx, wrappedPayload, []common.Hash{}, &common.Hash{}, nil)
		require.NoError(t, err)
		require.DeepEqual(t, bytesutil.ToBytes32(want.LatestValidHash), bytesutil.ToBytes32(latestValidHash))
	})
	t.Run(BlockByNumberMethod, func(t *testing.T) {
		want, ok := fix["ExecutionBlock"].(*pb.ExecutionBlock)
		require.Equal(t, true, ok)
		resp, err := srv.LatestExecutionBlock(ctx)
		require.NoError(t, err)
		require.DeepEqual(t, want, resp)
	})
	t.Run(BlockByHashMethod, func(t *testing.T) {
		want, ok := fix["ExecutionBlock"].(*pb.ExecutionBlock)
		require.Equal(t, true, ok)
		arg := common.BytesToHash([]byte("foo"))
		resp, err := srv.ExecutionBlockByHash(ctx, arg, true /* with txs */)
		require.NoError(t, err)
		require.DeepEqual(t, want, resp)
	})
}

func TestClient_HTTP(t *testing.T) {
	ctx := context.Background()
	fix := fixtures()

	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.CapellaForkEpoch = 1
	cfg.DenebForkEpoch = 2
	cfg.ElectraForkEpoch = 3
	params.OverrideBeaconConfig(cfg)

	t.Run(GetPayloadMethod, func(t *testing.T) {
		payloadId := [8]byte{1}
		want, ok := fix["ExecutionPayload"].(*pb.ExecutionPayload)
		require.Equal(t, true, ok)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			defer func() {
				require.NoError(t, r.Body.Close())
			}()
			enc, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			jsonRequestString := string(enc)

			reqArg, err := json.Marshal(pb.PayloadIDBytes(payloadId))
			require.NoError(t, err)

			// We expect the JSON string RPC request contains the right arguments.
			require.Equal(t, true, strings.Contains(
				jsonRequestString, string(reqArg),
			))
			resp := map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      1,
				"result":  want,
			}
			err = json.NewEncoder(w).Encode(resp)
			require.NoError(t, err)
		}))
		defer srv.Close()

		rpcClient, err := rpc.DialHTTP(srv.URL)
		require.NoError(t, err)
		defer rpcClient.Close()

		client := &Service{}
		client.rpcClient = rpcClient

		// We call the RPC method via HTTP and expect a proper result.
		resp, err := client.GetPayload(ctx, payloadId, 1)
		require.NoError(t, err)
		require.Equal(t, false, resp.OverrideBuilder)
		pbs := resp.ExecutionData.Proto()
		pbStruct, ok := pbs.(*pb.ExecutionPayload)
		require.Equal(t, true, ok)
		require.DeepEqual(t, want, pbStruct)
	})
	t.Run(GetPayloadMethodV2, func(t *testing.T) {
		payloadId := [8]byte{1}
		want, ok := fix["ExecutionPayloadCapellaWithValue"].(*pb.GetPayloadV2ResponseJson)
		require.Equal(t, true, ok)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			defer func() {
				require.NoError(t, r.Body.Close())
			}()
			enc, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			jsonRequestString := string(enc)

			reqArg, err := json.Marshal(pb.PayloadIDBytes(payloadId))
			require.NoError(t, err)

			// We expect the JSON string RPC request contains the right arguments.
			require.Equal(t, true, strings.Contains(
				jsonRequestString, string(reqArg),
			))
			resp := map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      1,
				"result":  want,
			}
			err = json.NewEncoder(w).Encode(resp)
			require.NoError(t, err)
		}))
		defer srv.Close()

		rpcClient, err := rpc.DialHTTP(srv.URL)
		require.NoError(t, err)
		defer rpcClient.Close()

		client := &Service{}
		client.rpcClient = rpcClient

		// We call the RPC method via HTTP and expect a proper result.
		resp, err := client.GetPayload(ctx, payloadId, params.BeaconConfig().SlotsPerEpoch)
		require.NoError(t, err)
		require.Equal(t, false, resp.OverrideBuilder)
		pbs := resp.ExecutionData.Proto()
		ep, ok := pbs.(*pb.ExecutionPayloadCapella)
		require.Equal(t, true, ok)
		require.DeepEqual(t, want.ExecutionPayload.BlockHash.Bytes(), ep.BlockHash)
		require.DeepEqual(t, want.ExecutionPayload.StateRoot.Bytes(), ep.StateRoot)
		require.DeepEqual(t, want.ExecutionPayload.ParentHash.Bytes(), ep.ParentHash)
		require.DeepEqual(t, want.ExecutionPayload.FeeRecipient.Bytes(), ep.FeeRecipient)
		require.DeepEqual(t, want.ExecutionPayload.PrevRandao.Bytes(), ep.PrevRandao)
		require.DeepEqual(t, want.ExecutionPayload.ParentHash.Bytes(), ep.ParentHash)

		require.Equal(t, primitives.Gwei(1236), primitives.WeiToGwei(resp.Bid))
	})
	t.Run(GetPayloadMethodV3, func(t *testing.T) {
		payloadId := [8]byte{1}
		want, ok := fix["ExecutionPayloadDenebWithValue"].(*pb.GetPayloadV3ResponseJson)
		require.Equal(t, true, ok)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			defer func() {
				require.NoError(t, r.Body.Close())
			}()
			enc, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			jsonRequestString := string(enc)

			reqArg, err := json.Marshal(pb.PayloadIDBytes(payloadId))
			require.NoError(t, err)

			// We expect the JSON string RPC request contains the right arguments.
			require.Equal(t, true, strings.Contains(
				jsonRequestString, string(reqArg),
			))
			resp := map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      1,
				"result":  want,
			}
			err = json.NewEncoder(w).Encode(resp)
			require.NoError(t, err)
		}))
		defer srv.Close()

		rpcClient, err := rpc.DialHTTP(srv.URL)
		require.NoError(t, err)
		defer rpcClient.Close()

		client := &Service{}
		client.rpcClient = rpcClient

		// We call the RPC method via HTTP and expect a proper result.
		resp, err := client.GetPayload(ctx, payloadId, 2*params.BeaconConfig().SlotsPerEpoch)
		require.NoError(t, err)
		require.Equal(t, true, resp.OverrideBuilder)
		g, err := resp.ExecutionData.ExcessBlobGas()
		require.NoError(t, err)
		require.DeepEqual(t, uint64(3), g)
		g, err = resp.ExecutionData.BlobGasUsed()
		require.NoError(t, err)
		require.DeepEqual(t, uint64(2), g)

		commitments := [][]byte{bytesutil.PadTo([]byte("commitment1"), fieldparams.BLSPubkeyLength), bytesutil.PadTo([]byte("commitment2"), fieldparams.BLSPubkeyLength)}
		require.DeepEqual(t, commitments, resp.BlobsBundle.KzgCommitments)
		proofs := [][]byte{bytesutil.PadTo([]byte("proof1"), fieldparams.BLSPubkeyLength), bytesutil.PadTo([]byte("proof2"), fieldparams.BLSPubkeyLength)}
		require.DeepEqual(t, proofs, resp.BlobsBundle.Proofs)
		blobs := [][]byte{bytesutil.PadTo([]byte("a"), fieldparams.BlobLength), bytesutil.PadTo([]byte("b"), fieldparams.BlobLength)}
		require.DeepEqual(t, blobs, resp.BlobsBundle.Blobs)
	})
	t.Run(GetPayloadMethodV4, func(t *testing.T) {
		payloadId := [8]byte{1}
		want, ok := fix["ExecutionBundleElectra"].(*pb.GetPayloadV4ResponseJson)
		require.Equal(t, true, ok)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			defer func() {
				require.NoError(t, r.Body.Close())
			}()
			enc, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			jsonRequestString := string(enc)

			reqArg, err := json.Marshal(pb.PayloadIDBytes(payloadId))
			require.NoError(t, err)

			// We expect the JSON string RPC request contains the right arguments.
			require.Equal(t, true, strings.Contains(
				jsonRequestString, string(reqArg),
			))
			resp := map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      1,
				"result":  want,
			}
			err = json.NewEncoder(w).Encode(resp)
			require.NoError(t, err)
		}))
		defer srv.Close()

		rpcClient, err := rpc.DialHTTP(srv.URL)
		require.NoError(t, err)
		defer rpcClient.Close()

		client := &Service{}
		client.rpcClient = rpcClient

		// We call the RPC method via HTTP and expect a proper result.
		resp, err := client.GetPayload(ctx, payloadId, 3*params.BeaconConfig().SlotsPerEpoch)
		require.NoError(t, err)
		require.Equal(t, true, resp.OverrideBuilder)
		g, err := resp.ExecutionData.ExcessBlobGas()
		require.NoError(t, err)
		require.DeepEqual(t, uint64(3), g)
		g, err = resp.ExecutionData.BlobGasUsed()
		require.NoError(t, err)
		require.DeepEqual(t, uint64(2), g)

		commitments := [][]byte{bytesutil.PadTo([]byte("commitment1"), fieldparams.BLSPubkeyLength), bytesutil.PadTo([]byte("commitment2"), fieldparams.BLSPubkeyLength)}
		require.DeepEqual(t, commitments, resp.BlobsBundle.KzgCommitments)
		proofs := [][]byte{bytesutil.PadTo([]byte("proof1"), fieldparams.BLSPubkeyLength), bytesutil.PadTo([]byte("proof2"), fieldparams.BLSPubkeyLength)}
		require.DeepEqual(t, proofs, resp.BlobsBundle.Proofs)
		blobs := [][]byte{bytesutil.PadTo([]byte("a"), fieldparams.BlobLength), bytesutil.PadTo([]byte("b"), fieldparams.BlobLength)}
		require.DeepEqual(t, blobs, resp.BlobsBundle.Blobs)
		requests := &pb.ExecutionRequests{
			Deposits: []*pb.DepositRequest{
				{
					Pubkey:                bytesutil.PadTo([]byte{byte('a')}, fieldparams.BLSPubkeyLength),
					WithdrawalCredentials: bytesutil.PadTo([]byte{byte('b')}, fieldparams.RootLength),
					Amount:                params.BeaconConfig().MinActivationBalance,
					Signature:             bytesutil.PadTo([]byte{byte('c')}, fieldparams.BLSSignatureLength),
					Index:                 0,
				},
			},
			Withdrawals: []*pb.WithdrawalRequest{
				{
					SourceAddress:   bytesutil.PadTo([]byte{byte('d')}, common.AddressLength),
					ValidatorPubkey: bytesutil.PadTo([]byte{byte('e')}, fieldparams.BLSPubkeyLength),
					Amount:          params.BeaconConfig().MinActivationBalance,
				},
			},
			Consolidations: []*pb.ConsolidationRequest{
				{
					SourceAddress: bytesutil.PadTo([]byte{byte('f')}, common.AddressLength),
					SourcePubkey:  bytesutil.PadTo([]byte{byte('g')}, fieldparams.BLSPubkeyLength),
					TargetPubkey:  bytesutil.PadTo([]byte{byte('h')}, fieldparams.BLSPubkeyLength),
				},
			},
		}

		require.DeepEqual(t, requests, resp.ExecutionRequests)
	})

	t.Run(ForkchoiceUpdatedMethod+" VALID status", func(t *testing.T) {
		forkChoiceState := &pb.ForkchoiceState{
			HeadBlockHash:      []byte("head"),
			SafeBlockHash:      []byte("safe"),
			FinalizedBlockHash: []byte("finalized"),
		}
		payloadAttributes := &pb.PayloadAttributes{
			Timestamp:             1,
			PrevRandao:            []byte("random"),
			SuggestedFeeRecipient: []byte("suggestedFeeRecipient"),
		}
		p, err := payloadattribute.New(payloadAttributes)
		require.NoError(t, err)
		want, ok := fix["ForkchoiceUpdatedResponse"].(*ForkchoiceUpdatedResponse)
		require.Equal(t, true, ok)
		srv := forkchoiceUpdateSetup(t, forkChoiceState, payloadAttributes, want)

		// We call the RPC method via HTTP and expect a proper result.
		payloadID, validHash, err := srv.ForkchoiceUpdated(ctx, forkChoiceState, p)
		require.NoError(t, err)
		require.DeepEqual(t, want.Status.LatestValidHash, validHash)
		require.DeepEqual(t, want.PayloadId, payloadID)
	})
	t.Run(ForkchoiceUpdatedMethodV2+" VALID status", func(t *testing.T) {
		forkChoiceState := &pb.ForkchoiceState{
			HeadBlockHash:      []byte("head"),
			SafeBlockHash:      []byte("safe"),
			FinalizedBlockHash: []byte("finalized"),
		}
		payloadAttributes := &pb.PayloadAttributesV2{
			Timestamp:             1,
			PrevRandao:            []byte("random"),
			SuggestedFeeRecipient: []byte("suggestedFeeRecipient"),
			Withdrawals:           []*pb.Withdrawal{{ValidatorIndex: 1, Amount: 1}},
		}
		p, err := payloadattribute.New(payloadAttributes)
		require.NoError(t, err)
		want, ok := fix["ForkchoiceUpdatedResponse"].(*ForkchoiceUpdatedResponse)
		require.Equal(t, true, ok)
		srv := forkchoiceUpdateSetupV2(t, forkChoiceState, payloadAttributes, want)

		// We call the RPC method via HTTP and expect a proper result.
		payloadID, validHash, err := srv.ForkchoiceUpdated(ctx, forkChoiceState, p)
		require.NoError(t, err)
		require.DeepEqual(t, want.Status.LatestValidHash, validHash)
		require.DeepEqual(t, want.PayloadId, payloadID)
	})
	t.Run(ForkchoiceUpdatedMethod+" SYNCING status", func(t *testing.T) {
		forkChoiceState := &pb.ForkchoiceState{
			HeadBlockHash:      []byte("head"),
			SafeBlockHash:      []byte("safe"),
			FinalizedBlockHash: []byte("finalized"),
		}
		payloadAttributes := &pb.PayloadAttributes{
			Timestamp:             1,
			PrevRandao:            []byte("random"),
			SuggestedFeeRecipient: []byte("suggestedFeeRecipient"),
		}
		p, err := payloadattribute.New(payloadAttributes)
		require.NoError(t, err)
		want, ok := fix["ForkchoiceUpdatedSyncingResponse"].(*ForkchoiceUpdatedResponse)
		require.Equal(t, true, ok)
		client := forkchoiceUpdateSetup(t, forkChoiceState, payloadAttributes, want)

		// We call the RPC method via HTTP and expect a proper result.
		payloadID, validHash, err := client.ForkchoiceUpdated(ctx, forkChoiceState, p)
		require.ErrorIs(t, err, ErrAcceptedSyncingPayloadStatus)
		require.DeepEqual(t, (*pb.PayloadIDBytes)(nil), payloadID)
		require.DeepEqual(t, []byte(nil), validHash)
	})
	t.Run(ForkchoiceUpdatedMethodV2+" SYNCING status", func(t *testing.T) {
		forkChoiceState := &pb.ForkchoiceState{
			HeadBlockHash:      []byte("head"),
			SafeBlockHash:      []byte("safe"),
			FinalizedBlockHash: []byte("finalized"),
		}
		payloadAttributes := &pb.PayloadAttributesV2{
			Timestamp:             1,
			PrevRandao:            []byte("random"),
			SuggestedFeeRecipient: []byte("suggestedFeeRecipient"),
			Withdrawals:           []*pb.Withdrawal{{ValidatorIndex: 1, Amount: 1}},
		}
		p, err := payloadattribute.New(payloadAttributes)
		require.NoError(t, err)
		want, ok := fix["ForkchoiceUpdatedSyncingResponse"].(*ForkchoiceUpdatedResponse)
		require.Equal(t, true, ok)
		srv := forkchoiceUpdateSetupV2(t, forkChoiceState, payloadAttributes, want)

		// We call the RPC method via HTTP and expect a proper result.
		payloadID, validHash, err := srv.ForkchoiceUpdated(ctx, forkChoiceState, p)
		require.ErrorIs(t, err, ErrAcceptedSyncingPayloadStatus)
		require.DeepEqual(t, (*pb.PayloadIDBytes)(nil), payloadID)
		require.DeepEqual(t, []byte(nil), validHash)
	})
	t.Run(ForkchoiceUpdatedMethod+" INVALID status", func(t *testing.T) {
		forkChoiceState := &pb.ForkchoiceState{
			HeadBlockHash:      []byte("head"),
			SafeBlockHash:      []byte("safe"),
			FinalizedBlockHash: []byte("finalized"),
		}
		payloadAttributes := &pb.PayloadAttributes{
			Timestamp:             1,
			PrevRandao:            []byte("random"),
			SuggestedFeeRecipient: []byte("suggestedFeeRecipient"),
		}
		p, err := payloadattribute.New(payloadAttributes)
		require.NoError(t, err)
		want, ok := fix["ForkchoiceUpdatedInvalidResponse"].(*ForkchoiceUpdatedResponse)
		require.Equal(t, true, ok)
		client := forkchoiceUpdateSetup(t, forkChoiceState, payloadAttributes, want)

		// We call the RPC method via HTTP and expect a proper result.
		payloadID, validHash, err := client.ForkchoiceUpdated(ctx, forkChoiceState, p)
		require.ErrorIs(t, err, ErrInvalidPayloadStatus)
		require.DeepEqual(t, (*pb.PayloadIDBytes)(nil), payloadID)
		require.DeepEqual(t, want.Status.LatestValidHash, validHash)
	})
	t.Run(ForkchoiceUpdatedMethod+" UNKNOWN status", func(t *testing.T) {
		forkChoiceState := &pb.ForkchoiceState{
			HeadBlockHash:      []byte("head"),
			SafeBlockHash:      []byte("safe"),
			FinalizedBlockHash: []byte("finalized"),
		}
		payloadAttributes := &pb.PayloadAttributes{
			Timestamp:             1,
			PrevRandao:            []byte("random"),
			SuggestedFeeRecipient: []byte("suggestedFeeRecipient"),
		}
		p, err := payloadattribute.New(payloadAttributes)
		require.NoError(t, err)
		want, ok := fix["ForkchoiceUpdatedAcceptedResponse"].(*ForkchoiceUpdatedResponse)
		require.Equal(t, true, ok)
		client := forkchoiceUpdateSetup(t, forkChoiceState, payloadAttributes, want)

		// We call the RPC method via HTTP and expect a proper result.
		payloadID, validHash, err := client.ForkchoiceUpdated(ctx, forkChoiceState, p)
		require.ErrorIs(t, err, ErrUnknownPayloadStatus)
		require.DeepEqual(t, (*pb.PayloadIDBytes)(nil), payloadID)
		require.DeepEqual(t, []byte(nil), validHash)
	})
	t.Run(NewPayloadMethod+" VALID status", func(t *testing.T) {
		execPayload, ok := fix["ExecutionPayload"].(*pb.ExecutionPayload)
		require.Equal(t, true, ok)
		want, ok := fix["ValidPayloadStatus"].(*pb.PayloadStatus)
		require.Equal(t, true, ok)
		client := newPayloadSetup(t, want, execPayload)

		// We call the RPC method via HTTP and expect a proper result.
		wrappedPayload, err := blocks.WrappedExecutionPayload(execPayload)
		require.NoError(t, err)
		resp, err := client.NewPayload(ctx, wrappedPayload, []common.Hash{}, &common.Hash{}, nil)
		require.NoError(t, err)
		require.DeepEqual(t, want.LatestValidHash, resp)
	})
	t.Run(NewPayloadMethodV2+" VALID status", func(t *testing.T) {
		execPayload, ok := fix["ExecutionPayloadCapella"].(*pb.ExecutionPayloadCapella)
		require.Equal(t, true, ok)
		want, ok := fix["ValidPayloadStatus"].(*pb.PayloadStatus)
		require.Equal(t, true, ok)
		client := newPayloadV2Setup(t, want, execPayload)

		// We call the RPC method via HTTP and expect a proper result.
		wrappedPayload, err := blocks.WrappedExecutionPayloadCapella(execPayload)
		require.NoError(t, err)
		resp, err := client.NewPayload(ctx, wrappedPayload, []common.Hash{}, &common.Hash{}, nil)
		require.NoError(t, err)
		require.DeepEqual(t, want.LatestValidHash, resp)
	})
	t.Run(NewPayloadMethodV3+" VALID status", func(t *testing.T) {
		execPayload, ok := fix["ExecutionPayloadDeneb"].(*pb.ExecutionPayloadDeneb)
		require.Equal(t, true, ok)
		want, ok := fix["ValidPayloadStatus"].(*pb.PayloadStatus)
		require.Equal(t, true, ok)
		client := newPayloadV3Setup(t, want, execPayload)

		// We call the RPC method via HTTP and expect a proper result.
		wrappedPayload, err := blocks.WrappedExecutionPayloadDeneb(execPayload)
		require.NoError(t, err)
		resp, err := client.NewPayload(ctx, wrappedPayload, []common.Hash{}, &common.Hash{'a'}, nil)
		require.NoError(t, err)
		require.DeepEqual(t, want.LatestValidHash, resp)
	})
	t.Run(NewPayloadMethodV4+" VALID status", func(t *testing.T) {
		execPayload, ok := fix["ExecutionPayloadDeneb"].(*pb.ExecutionPayloadDeneb)
		require.Equal(t, true, ok)
		want, ok := fix["ValidPayloadStatus"].(*pb.PayloadStatus)
		require.Equal(t, true, ok)

		// We call the RPC method via HTTP and expect a proper result.
		wrappedPayload, err := blocks.WrappedExecutionPayloadDeneb(execPayload)
		require.NoError(t, err)
		requests := &pb.ExecutionRequests{
			Deposits: []*pb.DepositRequest{
				{
					Pubkey:                bytesutil.PadTo([]byte{byte('a')}, fieldparams.BLSPubkeyLength),
					WithdrawalCredentials: bytesutil.PadTo([]byte{byte('b')}, fieldparams.RootLength),
					Amount:                params.BeaconConfig().MinActivationBalance,
					Signature:             bytesutil.PadTo([]byte{byte('c')}, fieldparams.BLSSignatureLength),
					Index:                 0,
				},
			},
			Withdrawals: []*pb.WithdrawalRequest{
				{
					SourceAddress:   bytesutil.PadTo([]byte{byte('d')}, common.AddressLength),
					ValidatorPubkey: bytesutil.PadTo([]byte{byte('e')}, fieldparams.BLSPubkeyLength),
					Amount:          params.BeaconConfig().MinActivationBalance,
				},
			},
			Consolidations: []*pb.ConsolidationRequest{
				{
					SourceAddress: bytesutil.PadTo([]byte{byte('f')}, common.AddressLength),
					SourcePubkey:  bytesutil.PadTo([]byte{byte('g')}, fieldparams.BLSPubkeyLength),
					TargetPubkey:  bytesutil.PadTo([]byte{byte('h')}, fieldparams.BLSPubkeyLength),
				},
			},
		}
		client := newPayloadV4Setup(t, want, execPayload, requests)
		resp, err := client.NewPayload(ctx, wrappedPayload, []common.Hash{}, &common.Hash{'a'}, requests)
		require.NoError(t, err)
		require.DeepEqual(t, want.LatestValidHash, resp)
	})
	t.Run(NewPayloadMethod+" SYNCING status", func(t *testing.T) {
		execPayload, ok := fix["ExecutionPayload"].(*pb.ExecutionPayload)
		require.Equal(t, true, ok)
		want, ok := fix["SyncingStatus"].(*pb.PayloadStatus)
		require.Equal(t, true, ok)
		client := newPayloadSetup(t, want, execPayload)

		// We call the RPC method via HTTP and expect a proper result.
		wrappedPayload, err := blocks.WrappedExecutionPayload(execPayload)
		require.NoError(t, err)
		resp, err := client.NewPayload(ctx, wrappedPayload, []common.Hash{}, &common.Hash{}, nil)
		require.ErrorIs(t, ErrAcceptedSyncingPayloadStatus, err)
		require.DeepEqual(t, []uint8(nil), resp)
	})
	t.Run(NewPayloadMethodV2+" SYNCING status", func(t *testing.T) {
		execPayload, ok := fix["ExecutionPayloadCapella"].(*pb.ExecutionPayloadCapella)
		require.Equal(t, true, ok)
		want, ok := fix["SyncingStatus"].(*pb.PayloadStatus)
		require.Equal(t, true, ok)
		client := newPayloadV2Setup(t, want, execPayload)

		// We call the RPC method via HTTP and expect a proper result.
		wrappedPayload, err := blocks.WrappedExecutionPayloadCapella(execPayload)
		require.NoError(t, err)
		resp, err := client.NewPayload(ctx, wrappedPayload, []common.Hash{}, &common.Hash{}, nil)
		require.ErrorIs(t, ErrAcceptedSyncingPayloadStatus, err)
		require.DeepEqual(t, []uint8(nil), resp)
	})
	t.Run(NewPayloadMethodV3+" SYNCING status", func(t *testing.T) {
		execPayload, ok := fix["ExecutionPayloadDeneb"].(*pb.ExecutionPayloadDeneb)
		require.Equal(t, true, ok)
		want, ok := fix["SyncingStatus"].(*pb.PayloadStatus)
		require.Equal(t, true, ok)
		client := newPayloadV3Setup(t, want, execPayload)

		// We call the RPC method via HTTP and expect a proper result.
		wrappedPayload, err := blocks.WrappedExecutionPayloadDeneb(execPayload)
		require.NoError(t, err)
		resp, err := client.NewPayload(ctx, wrappedPayload, []common.Hash{}, &common.Hash{'a'}, nil)
		require.ErrorIs(t, ErrAcceptedSyncingPayloadStatus, err)
		require.DeepEqual(t, []uint8(nil), resp)
	})
	t.Run(NewPayloadMethodV4+" SYNCING status", func(t *testing.T) {
		execPayload, ok := fix["ExecutionPayloadDeneb"].(*pb.ExecutionPayloadDeneb)
		require.Equal(t, true, ok)
		want, ok := fix["SyncingStatus"].(*pb.PayloadStatus)
		require.Equal(t, true, ok)

		// We call the RPC method via HTTP and expect a proper result.
		wrappedPayload, err := blocks.WrappedExecutionPayloadDeneb(execPayload)
		require.NoError(t, err)
		requests := &pb.ExecutionRequests{
			Deposits: []*pb.DepositRequest{
				{
					Pubkey:                bytesutil.PadTo([]byte{byte('a')}, fieldparams.BLSPubkeyLength),
					WithdrawalCredentials: bytesutil.PadTo([]byte{byte('b')}, fieldparams.RootLength),
					Amount:                params.BeaconConfig().MinActivationBalance,
					Signature:             bytesutil.PadTo([]byte{byte('c')}, fieldparams.BLSSignatureLength),
					Index:                 0,
				},
			},
			Withdrawals: []*pb.WithdrawalRequest{
				{
					SourceAddress:   bytesutil.PadTo([]byte{byte('d')}, common.AddressLength),
					ValidatorPubkey: bytesutil.PadTo([]byte{byte('e')}, fieldparams.BLSPubkeyLength),
					Amount:          params.BeaconConfig().MinActivationBalance,
				},
			},
			Consolidations: []*pb.ConsolidationRequest{
				{
					SourceAddress: bytesutil.PadTo([]byte{byte('f')}, common.AddressLength),
					SourcePubkey:  bytesutil.PadTo([]byte{byte('g')}, fieldparams.BLSPubkeyLength),
					TargetPubkey:  bytesutil.PadTo([]byte{byte('h')}, fieldparams.BLSPubkeyLength),
				},
			},
		}
		client := newPayloadV4Setup(t, want, execPayload, requests)
		resp, err := client.NewPayload(ctx, wrappedPayload, []common.Hash{}, &common.Hash{'a'}, requests)
		require.ErrorIs(t, ErrAcceptedSyncingPayloadStatus, err)
		require.DeepEqual(t, []uint8(nil), resp)
	})
	t.Run(NewPayloadMethod+" INVALID_BLOCK_HASH status", func(t *testing.T) {
		execPayload, ok := fix["ExecutionPayload"].(*pb.ExecutionPayload)
		require.Equal(t, true, ok)
		want, ok := fix["InvalidBlockHashStatus"].(*pb.PayloadStatus)
		require.Equal(t, true, ok)
		client := newPayloadSetup(t, want, execPayload)

		// We call the RPC method via HTTP and expect a proper result.
		wrappedPayload, err := blocks.WrappedExecutionPayload(execPayload)
		require.NoError(t, err)
		resp, err := client.NewPayload(ctx, wrappedPayload, []common.Hash{}, &common.Hash{}, nil)
		require.ErrorIs(t, ErrInvalidBlockHashPayloadStatus, err)
		require.DeepEqual(t, []uint8(nil), resp)
	})
	t.Run(NewPayloadMethodV2+" INVALID_BLOCK_HASH status", func(t *testing.T) {
		execPayload, ok := fix["ExecutionPayloadCapella"].(*pb.ExecutionPayloadCapella)
		require.Equal(t, true, ok)
		want, ok := fix["InvalidBlockHashStatus"].(*pb.PayloadStatus)
		require.Equal(t, true, ok)
		client := newPayloadV2Setup(t, want, execPayload)

		// We call the RPC method via HTTP and expect a proper result.
		wrappedPayload, err := blocks.WrappedExecutionPayloadCapella(execPayload)
		require.NoError(t, err)
		resp, err := client.NewPayload(ctx, wrappedPayload, []common.Hash{}, &common.Hash{}, nil)
		require.ErrorIs(t, ErrInvalidBlockHashPayloadStatus, err)
		require.DeepEqual(t, []uint8(nil), resp)
	})
	t.Run(NewPayloadMethodV3+" INVALID_BLOCK_HASH status", func(t *testing.T) {
		execPayload, ok := fix["ExecutionPayloadDeneb"].(*pb.ExecutionPayloadDeneb)
		require.Equal(t, true, ok)
		want, ok := fix["InvalidBlockHashStatus"].(*pb.PayloadStatus)
		require.Equal(t, true, ok)
		client := newPayloadV3Setup(t, want, execPayload)

		// We call the RPC method via HTTP and expect a proper result.
		wrappedPayload, err := blocks.WrappedExecutionPayloadDeneb(execPayload)
		require.NoError(t, err)
		resp, err := client.NewPayload(ctx, wrappedPayload, []common.Hash{}, &common.Hash{'a'}, nil)
		require.ErrorIs(t, ErrInvalidBlockHashPayloadStatus, err)
		require.DeepEqual(t, []uint8(nil), resp)
	})
	t.Run(NewPayloadMethodV4+" INVALID_BLOCK_HASH status", func(t *testing.T) {
		execPayload, ok := fix["ExecutionPayloadDeneb"].(*pb.ExecutionPayloadDeneb)
		require.Equal(t, true, ok)
		want, ok := fix["InvalidBlockHashStatus"].(*pb.PayloadStatus)
		require.Equal(t, true, ok)
		// We call the RPC method via HTTP and expect a proper result.
		wrappedPayload, err := blocks.WrappedExecutionPayloadDeneb(execPayload)
		require.NoError(t, err)
		requests := &pb.ExecutionRequests{
			Deposits: []*pb.DepositRequest{
				{
					Pubkey:                bytesutil.PadTo([]byte{byte('a')}, fieldparams.BLSPubkeyLength),
					WithdrawalCredentials: bytesutil.PadTo([]byte{byte('b')}, fieldparams.RootLength),
					Amount:                params.BeaconConfig().MinActivationBalance,
					Signature:             bytesutil.PadTo([]byte{byte('c')}, fieldparams.BLSSignatureLength),
					Index:                 0,
				},
			},
			Withdrawals: []*pb.WithdrawalRequest{
				{
					SourceAddress:   bytesutil.PadTo([]byte{byte('d')}, common.AddressLength),
					ValidatorPubkey: bytesutil.PadTo([]byte{byte('e')}, fieldparams.BLSPubkeyLength),
					Amount:          params.BeaconConfig().MinActivationBalance,
				},
			},
			Consolidations: []*pb.ConsolidationRequest{
				{
					SourceAddress: bytesutil.PadTo([]byte{byte('f')}, common.AddressLength),
					SourcePubkey:  bytesutil.PadTo([]byte{byte('g')}, fieldparams.BLSPubkeyLength),
					TargetPubkey:  bytesutil.PadTo([]byte{byte('h')}, fieldparams.BLSPubkeyLength),
				},
			},
		}
		client := newPayloadV4Setup(t, want, execPayload, requests)
		resp, err := client.NewPayload(ctx, wrappedPayload, []common.Hash{}, &common.Hash{'a'}, requests)
		require.ErrorIs(t, ErrInvalidBlockHashPayloadStatus, err)
		require.DeepEqual(t, []uint8(nil), resp)
	})
	t.Run(NewPayloadMethod+" INVALID status", func(t *testing.T) {
		execPayload, ok := fix["ExecutionPayload"].(*pb.ExecutionPayload)
		require.Equal(t, true, ok)
		want, ok := fix["InvalidStatus"].(*pb.PayloadStatus)
		require.Equal(t, true, ok)
		client := newPayloadSetup(t, want, execPayload)

		// We call the RPC method via HTTP and expect a proper result.
		wrappedPayload, err := blocks.WrappedExecutionPayload(execPayload)
		require.NoError(t, err)
		resp, err := client.NewPayload(ctx, wrappedPayload, []common.Hash{}, &common.Hash{}, nil)
		require.ErrorIs(t, ErrInvalidPayloadStatus, err)
		require.DeepEqual(t, want.LatestValidHash, resp)
	})
	t.Run(NewPayloadMethodV2+" INVALID status", func(t *testing.T) {
		execPayload, ok := fix["ExecutionPayloadCapella"].(*pb.ExecutionPayloadCapella)
		require.Equal(t, true, ok)
		want, ok := fix["InvalidStatus"].(*pb.PayloadStatus)
		require.Equal(t, true, ok)
		client := newPayloadV2Setup(t, want, execPayload)

		// We call the RPC method via HTTP and expect a proper result.
		wrappedPayload, err := blocks.WrappedExecutionPayloadCapella(execPayload)
		require.NoError(t, err)
		resp, err := client.NewPayload(ctx, wrappedPayload, []common.Hash{}, &common.Hash{}, nil)
		require.ErrorIs(t, ErrInvalidPayloadStatus, err)
		require.DeepEqual(t, want.LatestValidHash, resp)
	})
	t.Run(NewPayloadMethodV3+" INVALID status", func(t *testing.T) {
		execPayload, ok := fix["ExecutionPayloadDeneb"].(*pb.ExecutionPayloadDeneb)
		require.Equal(t, true, ok)
		want, ok := fix["InvalidStatus"].(*pb.PayloadStatus)
		require.Equal(t, true, ok)
		client := newPayloadV3Setup(t, want, execPayload)

		// We call the RPC method via HTTP and expect a proper result.
		wrappedPayload, err := blocks.WrappedExecutionPayloadDeneb(execPayload)
		require.NoError(t, err)
		resp, err := client.NewPayload(ctx, wrappedPayload, []common.Hash{}, &common.Hash{'a'}, nil)
		require.ErrorIs(t, ErrInvalidPayloadStatus, err)
		require.DeepEqual(t, want.LatestValidHash, resp)
	})
	t.Run(NewPayloadMethodV4+" INVALID status", func(t *testing.T) {
		execPayload, ok := fix["ExecutionPayloadDeneb"].(*pb.ExecutionPayloadDeneb)
		require.Equal(t, true, ok)
		want, ok := fix["InvalidStatus"].(*pb.PayloadStatus)
		require.Equal(t, true, ok)

		// We call the RPC method via HTTP and expect a proper result.
		wrappedPayload, err := blocks.WrappedExecutionPayloadDeneb(execPayload)
		require.NoError(t, err)
		requests := &pb.ExecutionRequests{
			Deposits: []*pb.DepositRequest{
				{
					Pubkey:                bytesutil.PadTo([]byte{byte('a')}, fieldparams.BLSPubkeyLength),
					WithdrawalCredentials: bytesutil.PadTo([]byte{byte('b')}, fieldparams.RootLength),
					Amount:                params.BeaconConfig().MinActivationBalance,
					Signature:             bytesutil.PadTo([]byte{byte('c')}, fieldparams.BLSSignatureLength),
					Index:                 0,
				},
			},
			Withdrawals: []*pb.WithdrawalRequest{
				{
					SourceAddress:   bytesutil.PadTo([]byte{byte('d')}, common.AddressLength),
					ValidatorPubkey: bytesutil.PadTo([]byte{byte('e')}, fieldparams.BLSPubkeyLength),
					Amount:          params.BeaconConfig().MinActivationBalance,
				},
			},
			Consolidations: []*pb.ConsolidationRequest{
				{
					SourceAddress: bytesutil.PadTo([]byte{byte('f')}, common.AddressLength),
					SourcePubkey:  bytesutil.PadTo([]byte{byte('g')}, fieldparams.BLSPubkeyLength),
					TargetPubkey:  bytesutil.PadTo([]byte{byte('h')}, fieldparams.BLSPubkeyLength),
				},
			},
		}
		client := newPayloadV4Setup(t, want, execPayload, requests)
		resp, err := client.NewPayload(ctx, wrappedPayload, []common.Hash{}, &common.Hash{'a'}, requests)
		require.ErrorIs(t, ErrInvalidPayloadStatus, err)
		require.DeepEqual(t, want.LatestValidHash, resp)
	})
	t.Run(NewPayloadMethod+" UNKNOWN status", func(t *testing.T) {
		execPayload, ok := fix["ExecutionPayload"].(*pb.ExecutionPayload)
		require.Equal(t, true, ok)
		want, ok := fix["UnknownStatus"].(*pb.PayloadStatus)
		require.Equal(t, true, ok)
		client := newPayloadSetup(t, want, execPayload)

		// We call the RPC method via HTTP and expect a proper result.
		wrappedPayload, err := blocks.WrappedExecutionPayload(execPayload)
		require.NoError(t, err)
		resp, err := client.NewPayload(ctx, wrappedPayload, []common.Hash{}, &common.Hash{}, nil)
		require.ErrorIs(t, ErrUnknownPayloadStatus, err)
		require.DeepEqual(t, []uint8(nil), resp)
	})
	t.Run(BlockByNumberMethod, func(t *testing.T) {
		want, ok := fix["ExecutionBlock"].(*pb.ExecutionBlock)
		require.Equal(t, true, ok)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			defer func() {
				require.NoError(t, r.Body.Close())
			}()
			resp := map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      1,
				"result":  want,
			}
			err := json.NewEncoder(w).Encode(resp)
			require.NoError(t, err)
		}))
		defer srv.Close()

		rpcClient, err := rpc.DialHTTP(srv.URL)
		require.NoError(t, err)
		defer rpcClient.Close()

		service := &Service{}
		service.rpcClient = rpcClient

		// We call the RPC method via HTTP and expect a proper result.
		resp, err := service.LatestExecutionBlock(ctx)
		require.NoError(t, err)
		require.DeepEqual(t, want, resp)
	})
	t.Run(BlockByHashMethod, func(t *testing.T) {
		arg := common.BytesToHash([]byte("foo"))
		want, ok := fix["ExecutionBlock"].(*pb.ExecutionBlock)
		require.Equal(t, true, ok)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			defer func() {
				require.NoError(t, r.Body.Close())
			}()
			enc, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			jsonRequestString := string(enc)
			// We expect the JSON string RPC request contains the right arguments.
			require.Equal(t, true, strings.Contains(
				jsonRequestString, fmt.Sprintf("%#x", arg),
			))
			resp := map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      1,
				"result":  want,
			}
			err = json.NewEncoder(w).Encode(resp)
			require.NoError(t, err)
		}))
		defer srv.Close()

		rpcClient, err := rpc.DialHTTP(srv.URL)
		require.NoError(t, err)
		defer rpcClient.Close()

		service := &Service{}
		service.rpcClient = rpcClient

		// We call the RPC method via HTTP and expect a proper result.
		resp, err := service.ExecutionBlockByHash(ctx, arg, true /* with txs */)
		require.NoError(t, err)
		require.DeepEqual(t, want, resp)
	})
}

func TestReconstructFullBellatrixBlock(t *testing.T) {
	ctx := context.Background()
	t.Run("nil block", func(t *testing.T) {
		service := &Service{}

		_, err := service.ReconstructFullBlock(ctx, nil)
		require.ErrorContains(t, "nil data", err)
	})
	t.Run("only blinded block", func(t *testing.T) {
		want := "can only reconstruct block from blinded block format"
		service := &Service{}
		bellatrixBlock := util.NewBeaconBlockBellatrix()
		wrapped, err := blocks.NewSignedBeaconBlock(bellatrixBlock)
		require.NoError(t, err)
		_, err = service.ReconstructFullBlock(ctx, wrapped)
		require.ErrorContains(t, want, err)
	})
	t.Run("pre-merge execution payload", func(t *testing.T) {
		service := &Service{}
		bellatrixBlock := util.NewBlindedBeaconBlockBellatrix()
		wanted := util.NewBeaconBlockBellatrix()
		wanted.Block.Slot = 1
		// Make sure block hash is the zero hash.
		bellatrixBlock.Block.Body.ExecutionPayloadHeader.BlockHash = make([]byte, 32)
		bellatrixBlock.Block.Slot = 1
		wrapped, err := blocks.NewSignedBeaconBlock(bellatrixBlock)
		require.NoError(t, err)
		wantedWrapped, err := blocks.NewSignedBeaconBlock(wanted)
		require.NoError(t, err)
		reconstructed, err := service.ReconstructFullBlock(ctx, wrapped)
		require.NoError(t, err)
		require.DeepEqual(t, wantedWrapped, reconstructed)
	})
	t.Run("properly reconstructs block with correct payload", func(t *testing.T) {
		fix := fixtures()
		payload, ok := fix["ExecutionPayload"].(*pb.ExecutionPayload)
		require.Equal(t, true, ok)

		jsonPayload := make(map[string]interface{})
		tx := gethtypes.NewTransaction(
			0,
			common.HexToAddress("095e7baea6a6c7c4c2dfeb977efac326af552d87"),
			big.NewInt(0), 0, big.NewInt(0),
			nil,
		)
		txs := []*gethtypes.Transaction{tx}
		encodedBinaryTxs := make([][]byte, 1)
		var err error
		encodedBinaryTxs[0], err = txs[0].MarshalBinary()
		require.NoError(t, err)
		payload.Transactions = encodedBinaryTxs
		jsonPayload["transactions"] = []hexutil.Bytes{encodedBinaryTxs[0]}

		wrappedPayload, err := blocks.WrappedExecutionPayload(payload)
		require.NoError(t, err)
		header, err := blocks.PayloadToHeader(wrappedPayload)
		require.NoError(t, err)

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			defer func() {
				require.NoError(t, r.Body.Close())
			}()
			respJSON := map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      1,
				"result":  []map[string]interface{}{jsonPayload},
			}
			require.NoError(t, json.NewEncoder(w).Encode(respJSON))
		}))
		defer srv.Close()

		rpcClient, err := rpc.DialHTTP(srv.URL)
		require.NoError(t, err)
		defer rpcClient.Close()

		service := &Service{}
		service.rpcClient = rpcClient
		blindedBlock := util.NewBlindedBeaconBlockBellatrix()

		blindedBlock.Block.Body.ExecutionPayloadHeader = header
		wrapped, err := blocks.NewSignedBeaconBlock(blindedBlock)
		require.NoError(t, err)
		reconstructed, err := service.ReconstructFullBlock(ctx, wrapped)
		require.NoError(t, err)

		got, err := reconstructed.Block().Body().Execution()
		require.NoError(t, err)
		require.DeepEqual(t, payload, got.Proto())
	})
}

func TestReconstructFullBellatrixBlockBatch(t *testing.T) {
	ctx := context.Background()
	t.Run("nil block", func(t *testing.T) {
		service := &Service{}

		_, err := service.ReconstructFullBellatrixBlockBatch(ctx, []interfaces.ReadOnlySignedBeaconBlock{nil})
		require.ErrorContains(t, "nil data", err)
	})
	t.Run("only blinded block", func(t *testing.T) {
		want := "can only reconstruct block from blinded block format"
		service := &Service{}
		bellatrixBlock := util.NewBeaconBlockBellatrix()
		wrapped, err := blocks.NewSignedBeaconBlock(bellatrixBlock)
		require.NoError(t, err)
		_, err = service.ReconstructFullBellatrixBlockBatch(ctx, []interfaces.ReadOnlySignedBeaconBlock{wrapped})
		require.ErrorContains(t, want, err)
	})
	t.Run("pre-merge execution payload", func(t *testing.T) {
		service := &Service{}
		bellatrixBlock := util.NewBlindedBeaconBlockBellatrix()
		wanted := util.NewBeaconBlockBellatrix()
		wanted.Block.Slot = 1
		// Make sure block hash is the zero hash.
		bellatrixBlock.Block.Body.ExecutionPayloadHeader.BlockHash = make([]byte, 32)
		bellatrixBlock.Block.Slot = 1
		wrapped, err := blocks.NewSignedBeaconBlock(bellatrixBlock)
		require.NoError(t, err)
		wantedWrapped, err := blocks.NewSignedBeaconBlock(wanted)
		require.NoError(t, err)
		reconstructed, err := service.ReconstructFullBellatrixBlockBatch(ctx, []interfaces.ReadOnlySignedBeaconBlock{wrapped})
		require.NoError(t, err)
		require.DeepEqual(t, []interfaces.SignedBeaconBlock{wantedWrapped}, reconstructed)
	})
	t.Run("properly reconstructs block batch with correct payload", func(t *testing.T) {
		fix := fixtures()
		payload, ok := fix["ExecutionPayload"].(*pb.ExecutionPayload)
		require.Equal(t, true, ok)

		jsonPayload := make(map[string]interface{})
		tx := gethtypes.NewTransaction(
			0,
			common.HexToAddress("095e7baea6a6c7c4c2dfeb977efac326af552d87"),
			big.NewInt(0), 0, big.NewInt(0),
			nil,
		)
		txs := []*gethtypes.Transaction{tx}
		encodedBinaryTxs := make([][]byte, 1)
		var err error
		encodedBinaryTxs[0], err = txs[0].MarshalBinary()
		require.NoError(t, err)
		payload.Transactions = encodedBinaryTxs
		jsonPayload["transactions"] = []hexutil.Bytes{encodedBinaryTxs[0]}

		wrappedPayload, err := blocks.WrappedExecutionPayload(payload)
		require.NoError(t, err)
		header, err := blocks.PayloadToHeader(wrappedPayload)
		require.NoError(t, err)

		bellatrixBlock := util.NewBlindedBeaconBlockBellatrix()
		wanted := util.NewBeaconBlockBellatrix()
		wanted.Block.Slot = 1
		// Make sure block hash is the zero hash.
		bellatrixBlock.Block.Body.ExecutionPayloadHeader.BlockHash = make([]byte, 32)
		bellatrixBlock.Block.Slot = 1
		wrappedEmpty, err := blocks.NewSignedBeaconBlock(bellatrixBlock)
		require.NoError(t, err)
		wantedWrappedEmpty, err := blocks.NewSignedBeaconBlock(wanted)
		require.NoError(t, err)

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			defer func() {
				require.NoError(t, r.Body.Close())
			}()

			respJSON := map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      1,
				"result":  []map[string]interface{}{jsonPayload},
			}
			require.NoError(t, json.NewEncoder(w).Encode(respJSON))

		}))
		defer srv.Close()

		rpcClient, err := rpc.DialHTTP(srv.URL)
		require.NoError(t, err)
		defer rpcClient.Close()

		service := &Service{}
		service.rpcClient = rpcClient
		blindedBlock := util.NewBlindedBeaconBlockBellatrix()

		blindedBlock.Block.Body.ExecutionPayloadHeader = header
		wrapped, err := blocks.NewSignedBeaconBlock(blindedBlock)
		require.NoError(t, err)

		reconstructed, err := service.ReconstructFullBellatrixBlockBatch(ctx, []interfaces.ReadOnlySignedBeaconBlock{wrappedEmpty, wrapped})
		require.NoError(t, err)

		// Make sure empty blocks are handled correctly
		require.DeepEqual(t, wantedWrappedEmpty, reconstructed[0])

		// Handle normal execution blocks correctly
		got, err := reconstructed[1].Block().Body().Execution()
		require.NoError(t, err)
		require.DeepEqual(t, payload, got.Proto())
	})
	t.Run("handles invalid response from EL", func(t *testing.T) {
		fix := fixtures()
		payload, ok := fix["ExecutionPayload"].(*pb.ExecutionPayload)
		require.Equal(t, true, ok)

		jsonPayload := make(map[string]interface{})
		tx := gethtypes.NewTransaction(
			0,
			common.HexToAddress("095e7baea6a6c7c4c2dfeb977efac326af552d87"),
			big.NewInt(0), 0, big.NewInt(0),
			nil,
		)
		txs := []*gethtypes.Transaction{tx}
		encodedBinaryTxs := make([][]byte, 1)
		var err error
		encodedBinaryTxs[0], err = txs[0].MarshalBinary()
		require.NoError(t, err)
		payload.Transactions = encodedBinaryTxs
		jsonPayload["transactions"] = []hexutil.Bytes{encodedBinaryTxs[0]}

		wrappedPayload, err := blocks.WrappedExecutionPayload(payload)
		require.NoError(t, err)
		header, err := blocks.PayloadToHeader(wrappedPayload)
		require.NoError(t, err)

		bellatrixBlock := util.NewBlindedBeaconBlockBellatrix()
		wanted := util.NewBeaconBlockBellatrix()
		wanted.Block.Slot = 1
		// Make sure block hash is the zero hash.
		bellatrixBlock.Block.Body.ExecutionPayloadHeader.BlockHash = make([]byte, 32)
		bellatrixBlock.Block.Slot = 1
		wrappedEmpty, err := blocks.NewSignedBeaconBlock(bellatrixBlock)
		require.NoError(t, err)

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			defer func() {
				require.NoError(t, r.Body.Close())
			}()

			respJSON := map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      1,
				"result":  []map[string]interface{}{},
			}
			require.NoError(t, json.NewEncoder(w).Encode(respJSON))

		}))
		defer srv.Close()

		rpcClient, err := rpc.DialHTTP(srv.URL)
		require.NoError(t, err)
		defer rpcClient.Close()

		service := &Service{}
		service.rpcClient = rpcClient
		blindedBlock := util.NewBlindedBeaconBlockBellatrix()

		blindedBlock.Block.Body.ExecutionPayloadHeader = header
		wrapped, err := blocks.NewSignedBeaconBlock(blindedBlock)
		require.NoError(t, err)
		copiedWrapped, err := wrapped.Copy()
		require.NoError(t, err)

		_, err = service.ReconstructFullBellatrixBlockBatch(ctx, []interfaces.ReadOnlySignedBeaconBlock{wrappedEmpty, wrapped, copiedWrapped})
		require.ErrorIs(t, err, errInvalidPayloadBodyResponse)
	})
}

func TestServer_getPowBlockHashAtTerminalTotalDifficulty(t *testing.T) {
	tests := []struct {
		name                  string
		paramsTd              string
		currentPowBlock       *pb.ExecutionBlock
		parentPowBlock        *pb.ExecutionBlock
		errLatestExecutionBlk error
		wantTerminalBlockHash []byte
		wantExists            bool
		errString             string
	}{
		{
			name:      "config td overflows",
			paramsTd:  "1115792089237316195423570985008687907853269984665640564039457584007913129638912",
			errString: "could not convert terminal total difficulty to uint256",
		},
		{
			name:                  "could not get latest execution block",
			paramsTd:              "1",
			errLatestExecutionBlk: errors.New("blah"),
			errString:             "could not get latest execution block",
		},
		{
			name:      "nil latest execution block",
			paramsTd:  "1",
			errString: "latest execution block is nil",
		},
		{
			name:     "current execution block invalid TD",
			paramsTd: "1",
			currentPowBlock: &pb.ExecutionBlock{
				Hash:            common.BytesToHash([]byte("a")),
				TotalDifficulty: "1115792089237316195423570985008687907853269984665640564039457584007913129638912",
			},
			errString: "could not convert total difficulty to uint256",
		},
		{
			name:     "current execution block has zero hash parent",
			paramsTd: "2",
			currentPowBlock: &pb.ExecutionBlock{
				Hash: common.BytesToHash([]byte("a")),
				Header: gethtypes.Header{
					ParentHash: common.BytesToHash(params.BeaconConfig().ZeroHash[:]),
				},
				TotalDifficulty: "0x3",
			},
		},
		{
			name:     "could not get parent block",
			paramsTd: "2",
			currentPowBlock: &pb.ExecutionBlock{
				Hash: common.BytesToHash([]byte("a")),
				Header: gethtypes.Header{
					ParentHash: common.BytesToHash([]byte("b")),
				},
				TotalDifficulty: "0x3",
			},
			errString: "could not get parent execution block",
		},
		{
			name:     "parent execution block invalid TD",
			paramsTd: "2",
			currentPowBlock: &pb.ExecutionBlock{
				Hash: common.BytesToHash([]byte("a")),
				Header: gethtypes.Header{
					ParentHash: common.BytesToHash([]byte("b")),
				},
				TotalDifficulty: "0x3",
			},
			parentPowBlock: &pb.ExecutionBlock{
				Hash: common.BytesToHash([]byte("b")),
				Header: gethtypes.Header{
					ParentHash: common.BytesToHash([]byte("c")),
				},
				TotalDifficulty: "1",
			},
			errString: "could not convert total difficulty to uint256",
		},
		{
			name:     "happy case",
			paramsTd: "2",
			currentPowBlock: &pb.ExecutionBlock{
				Hash: common.BytesToHash([]byte("a")),
				Header: gethtypes.Header{
					ParentHash: common.BytesToHash([]byte("b")),
				},
				TotalDifficulty: "0x3",
			},
			parentPowBlock: &pb.ExecutionBlock{
				Hash: common.BytesToHash([]byte("b")),
				Header: gethtypes.Header{
					ParentHash: common.BytesToHash([]byte("c")),
				},
				TotalDifficulty: "0x1",
			},
			wantExists:            true,
			wantTerminalBlockHash: common.BytesToHash([]byte("a")).Bytes(),
		},
		{
			name:     "happy case, but invalid timestamp",
			paramsTd: "2",
			currentPowBlock: &pb.ExecutionBlock{
				Hash: common.BytesToHash([]byte("a")),
				Header: gethtypes.Header{
					ParentHash: common.BytesToHash([]byte("b")),
					Time:       1,
				},
				TotalDifficulty: "0x3",
			},
			parentPowBlock: &pb.ExecutionBlock{
				Hash: common.BytesToHash([]byte("b")),
				Header: gethtypes.Header{
					ParentHash: common.BytesToHash([]byte("c")),
				},
				TotalDifficulty: "0x1",
			},
		},
		{
			name:     "ttd not reached",
			paramsTd: "3",
			currentPowBlock: &pb.ExecutionBlock{
				Hash: common.BytesToHash([]byte("a")),
				Header: gethtypes.Header{
					ParentHash: common.BytesToHash([]byte("b")),
				},
				TotalDifficulty: "0x2",
			},
			parentPowBlock: &pb.ExecutionBlock{
				Hash: common.BytesToHash([]byte("b")),
				Header: gethtypes.Header{
					ParentHash: common.BytesToHash([]byte("c")),
				},
				TotalDifficulty: "0x1",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := params.BeaconConfig().Copy()
			cfg.TerminalTotalDifficulty = tt.paramsTd
			params.OverrideBeaconConfig(cfg)
			var m map[[32]byte]*pb.ExecutionBlock
			if tt.parentPowBlock != nil {
				m = map[[32]byte]*pb.ExecutionBlock{
					tt.parentPowBlock.Hash: tt.parentPowBlock,
				}
			}
			client := mocks.EngineClient{
				ErrLatestExecBlock: tt.errLatestExecutionBlk,
				ExecutionBlock:     tt.currentPowBlock,
				BlockByHashMap:     m,
			}
			b, e, err := client.GetTerminalBlockHash(context.Background(), 1)
			if tt.errString != "" {
				require.ErrorContains(t, tt.errString, err)
			} else {
				require.NoError(t, err)
				require.DeepEqual(t, tt.wantExists, e)
				require.DeepEqual(t, tt.wantTerminalBlockHash, b)
			}
		})
	}
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

type customError struct {
	code    int
	timeout bool
}

func (c *customError) ErrorCode() int {
	return c.code
}

func (*customError) Error() string {
	return "something went wrong"
}

func (c *customError) Timeout() bool {
	return c.timeout
}

type dataError struct {
	code int
	data interface{}
}

func (c *dataError) ErrorCode() int {
	return c.code
}

func (*dataError) Error() string {
	return "something went wrong"
}

func (c *dataError) ErrorData() interface{} {
	return c.data
}

func Test_handleRPCError(t *testing.T) {
	got := handleRPCError(nil)
	require.Equal(t, true, got == nil)

	var tests = []struct {
		name             string
		expected         error
		expectedContains string
		given            error
	}{
		{
			name:             "not an rpc error",
			expectedContains: "got an unexpected error",
			given:            errors.New("foo"),
		},
		{
			name:             "HTTP times out",
			expectedContains: ErrHTTPTimeout.Error(),
			given:            &customError{timeout: true},
		},
		{
			name:             "ErrParse",
			expectedContains: ErrParse.Error(),
			given:            &customError{code: -32700},
		},
		{
			name:             "ErrInvalidRequest",
			expectedContains: ErrInvalidRequest.Error(),
			given:            &customError{code: -32600},
		},
		{
			name:             "ErrMethodNotFound",
			expectedContains: ErrMethodNotFound.Error(),
			given:            &customError{code: -32601},
		},
		{
			name:             "ErrInvalidParams",
			expectedContains: ErrInvalidParams.Error(),
			given:            &customError{code: -32602},
		},
		{
			name:             "ErrInternal",
			expectedContains: ErrInternal.Error(),
			given:            &customError{code: -32603},
		},
		{
			name:             "ErrUnknownPayload",
			expectedContains: ErrUnknownPayload.Error(),
			given:            &customError{code: -38001},
		},
		{
			name:             "ErrInvalidForkchoiceState",
			expectedContains: ErrInvalidForkchoiceState.Error(),
			given:            &customError{code: -38002},
		},
		{
			name:             "ErrInvalidPayloadAttributes",
			expectedContains: ErrInvalidPayloadAttributes.Error(),
			given:            &customError{code: -38003},
		},
		{
			name:             "ErrServer unexpected no data",
			expectedContains: "got an unexpected error",
			given:            &customError{code: -32000},
		},
		{
			name:             "ErrServer with data",
			expectedContains: ErrServer.Error(),
			given:            &dataError{code: -32000, data: 5},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := handleRPCError(tt.given)
			require.ErrorContains(t, tt.expectedContains, got)
		})
	}
}

func newTestIPCServer(t *testing.T) *rpc.Server {
	server := rpc.NewServer()
	err := server.RegisterName("engine", new(testEngineService))
	require.NoError(t, err)
	err = server.RegisterName("eth", new(testEngineService))
	require.NoError(t, err)
	return server
}

func fixtures() map[string]interface{} {
	s := fixturesStruct()
	return map[string]interface{}{
		"ExecutionBlock":                    s.ExecutionBlock,
		"ExecutionPayloadBody":              s.ExecutionPayloadBody,
		"ExecutionPayload":                  s.ExecutionPayload,
		"ExecutionPayloadCapella":           s.ExecutionPayloadCapella,
		"ExecutionPayloadDeneb":             s.ExecutionPayloadDeneb,
		"ExecutionPayloadCapellaWithValue":  s.ExecutionPayloadWithValueCapella,
		"ExecutionPayloadDenebWithValue":    s.ExecutionPayloadWithValueDeneb,
		"ExecutionBundleElectra":            s.ExecutionBundleElectra,
		"ValidPayloadStatus":                s.ValidPayloadStatus,
		"InvalidBlockHashStatus":            s.InvalidBlockHashStatus,
		"AcceptedStatus":                    s.AcceptedStatus,
		"SyncingStatus":                     s.SyncingStatus,
		"InvalidStatus":                     s.InvalidStatus,
		"UnknownStatus":                     s.UnknownStatus,
		"ForkchoiceUpdatedResponse":         s.ForkchoiceUpdatedResponse,
		"ForkchoiceUpdatedSyncingResponse":  s.ForkchoiceUpdatedSyncingResponse,
		"ForkchoiceUpdatedAcceptedResponse": s.ForkchoiceUpdatedAcceptedResponse,
		"ForkchoiceUpdatedInvalidResponse":  s.ForkchoiceUpdatedInvalidResponse,
	}
}

func fixturesStruct() *payloadFixtures {
	foo := bytesutil.ToBytes32([]byte("foo"))
	bar := bytesutil.PadTo([]byte("bar"), 20)
	baz := bytesutil.PadTo([]byte("baz"), 256)
	baseFeePerGas := big.NewInt(12345)
	executionPayloadFixture := &pb.ExecutionPayload{
		ParentHash:    foo[:],
		FeeRecipient:  bar,
		StateRoot:     foo[:],
		ReceiptsRoot:  foo[:],
		LogsBloom:     baz,
		PrevRandao:    foo[:],
		BlockNumber:   1,
		GasLimit:      1,
		GasUsed:       1,
		Timestamp:     1,
		ExtraData:     foo[:],
		BaseFeePerGas: bytesutil.PadTo(baseFeePerGas.Bytes(), fieldparams.RootLength),
		BlockHash:     foo[:],
		Transactions:  [][]byte{foo[:]},
	}
	executionPayloadBodyFixture := &pb.ExecutionPayloadBody{
		Transactions: []hexutil.Bytes{foo[:]},
		Withdrawals:  []*pb.Withdrawal{},
	}
	executionPayloadFixtureCapella := &pb.ExecutionPayloadCapella{
		ParentHash:    foo[:],
		FeeRecipient:  bar,
		StateRoot:     foo[:],
		ReceiptsRoot:  foo[:],
		LogsBloom:     baz,
		PrevRandao:    foo[:],
		BlockNumber:   1,
		GasLimit:      1,
		GasUsed:       1,
		Timestamp:     1,
		ExtraData:     foo[:],
		BaseFeePerGas: bytesutil.PadTo(baseFeePerGas.Bytes(), fieldparams.RootLength),
		BlockHash:     foo[:],
		Transactions:  [][]byte{foo[:]},
		Withdrawals:   []*pb.Withdrawal{},
	}
	emptyExecutionPayloadDeneb := &pb.ExecutionPayloadDeneb{
		ParentHash:    foo[:],
		FeeRecipient:  bar,
		StateRoot:     foo[:],
		ReceiptsRoot:  foo[:],
		LogsBloom:     baz,
		PrevRandao:    foo[:],
		BlockNumber:   1,
		GasLimit:      1,
		GasUsed:       1,
		Timestamp:     1,
		ExtraData:     foo[:],
		BaseFeePerGas: bytesutil.PadTo(baseFeePerGas.Bytes(), fieldparams.RootLength),
		BlockHash:     foo[:],
		BlobGasUsed:   2,
		ExcessBlobGas: 3,
	}
	executionPayloadFixtureDeneb := &pb.ExecutionPayloadDeneb{
		ParentHash:    emptyExecutionPayloadDeneb.ParentHash,
		FeeRecipient:  emptyExecutionPayloadDeneb.FeeRecipient,
		StateRoot:     emptyExecutionPayloadDeneb.StateRoot,
		ReceiptsRoot:  emptyExecutionPayloadDeneb.ReceiptsRoot,
		LogsBloom:     emptyExecutionPayloadDeneb.LogsBloom,
		PrevRandao:    emptyExecutionPayloadDeneb.PrevRandao,
		BlockNumber:   emptyExecutionPayloadDeneb.BlockNumber,
		GasLimit:      emptyExecutionPayloadDeneb.GasLimit,
		GasUsed:       emptyExecutionPayloadDeneb.GasUsed,
		Timestamp:     emptyExecutionPayloadDeneb.Timestamp,
		ExtraData:     emptyExecutionPayloadDeneb.ExtraData,
		BaseFeePerGas: emptyExecutionPayloadDeneb.BaseFeePerGas,
		BlockHash:     emptyExecutionPayloadDeneb.BlockHash,
		BlobGasUsed:   emptyExecutionPayloadDeneb.BlobGasUsed,
		ExcessBlobGas: emptyExecutionPayloadDeneb.ExcessBlobGas,
		// added on top of the empty payload
		Transactions: [][]byte{foo[:]},
		Withdrawals:  []*pb.Withdrawal{},
	}
	withdrawalRequests := make([]pb.WithdrawalRequestV1, 3)
	for i := range withdrawalRequests {
		amount := hexutil.Uint64(i)
		address := &common.Address{}
		address.SetBytes([]byte{0, 0, byte(i)})
		pubkey := pb.BlsPubkey{}
		copy(pubkey[:], []byte{0, byte(i)})
		withdrawalRequests[i] = pb.WithdrawalRequestV1{
			SourceAddress:   address,
			ValidatorPubkey: &pubkey,
			Amount:          &amount,
		}
	}
	depositRequests := make([]pb.DepositRequestV1, 3)
	for i := range depositRequests {
		amount := hexutil.Uint64(math.MaxUint16 - i)
		creds := &common.Hash{}
		creds.SetBytes([]byte{0, 0, byte(i)})
		pubkey := pb.BlsPubkey{}
		copy(pubkey[:], []byte{0, byte(i)})
		sig := pb.BlsSig{}
		copy(sig[:], []byte{0, 0, 0, byte(i)})
		idx := hexutil.Uint64(i)
		depositRequests[i] = pb.DepositRequestV1{
			PubKey:                &pubkey,
			WithdrawalCredentials: creds,
			Amount:                &amount,
			Signature:             &sig,
			Index:                 &idx,
		}
	}
	consolidationRequests := make([]pb.ConsolidationRequestV1, 1)
	for i := range consolidationRequests {
		address := &common.Address{}
		address.SetBytes([]byte{0, 0, byte(i)})
		sPubkey := pb.BlsPubkey{}
		copy(sPubkey[:], []byte{0, byte(i)})
		tPubkey := pb.BlsPubkey{}
		copy(tPubkey[:], []byte{0, byte(i)})
		consolidationRequests[i] = pb.ConsolidationRequestV1{
			SourceAddress: address,
			SourcePubkey:  &sPubkey,
			TargetPubkey:  &tPubkey,
		}
	}
	hexUint := hexutil.Uint64(1)
	executionPayloadWithValueFixtureCapella := &pb.GetPayloadV2ResponseJson{
		ExecutionPayload: &pb.ExecutionPayloadCapellaJSON{
			ParentHash:    &common.Hash{'a'},
			FeeRecipient:  &common.Address{'b'},
			StateRoot:     &common.Hash{'c'},
			ReceiptsRoot:  &common.Hash{'d'},
			LogsBloom:     &hexutil.Bytes{'e'},
			PrevRandao:    &common.Hash{'f'},
			BaseFeePerGas: "0x123",
			BlockHash:     &common.Hash{'g'},
			Transactions:  []hexutil.Bytes{{'h'}},
			Withdrawals:   []*pb.Withdrawal{},
			BlockNumber:   &hexUint,
			GasLimit:      &hexUint,
			GasUsed:       &hexUint,
			Timestamp:     &hexUint,
		},
		BlockValue: "0x11fffffffff",
	}
	bgu := hexutil.Uint64(2)
	ebg := hexutil.Uint64(3)
	executionPayloadWithValueFixtureDeneb := &pb.GetPayloadV3ResponseJson{
		ShouldOverrideBuilder: true,
		ExecutionPayload: &pb.ExecutionPayloadDenebJSON{
			ParentHash:    &common.Hash{'a'},
			FeeRecipient:  &common.Address{'b'},
			StateRoot:     &common.Hash{'c'},
			ReceiptsRoot:  &common.Hash{'d'},
			LogsBloom:     &hexutil.Bytes{'e'},
			PrevRandao:    &common.Hash{'f'},
			BaseFeePerGas: "0x123",
			BlockHash:     &common.Hash{'g'},
			Transactions:  []hexutil.Bytes{{'h'}},
			Withdrawals:   []*pb.Withdrawal{},
			BlockNumber:   &hexUint,
			GasLimit:      &hexUint,
			GasUsed:       &hexUint,
			Timestamp:     &hexUint,
			BlobGasUsed:   &bgu,
			ExcessBlobGas: &ebg,
		},
		BlockValue: "0x11fffffffff",
		BlobsBundle: &pb.BlobBundleJSON{
			Commitments: []hexutil.Bytes{[]byte("commitment1"), []byte("commitment2")},
			Proofs:      []hexutil.Bytes{[]byte("proof1"), []byte("proof2")},
			Blobs:       []hexutil.Bytes{{'a'}, {'b'}},
		},
	}

	depositRequestBytes, err := hexutil.Decode("0x610000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000" +
		"620000000000000000000000000000000000000000000000000000000000000000" +
		"4059730700000063000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000" +
		"00000000000000000000000000000000000000000000000000000000000000000000000000000000")
	if err != nil {
		panic("failed to decode deposit request")
	}
	withdrawalRequestBytes, err := hexutil.Decode("0x6400000000000000000000000000000000000000" +
		"6500000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000040597307000000")
	if err != nil {
		panic("failed to decode withdrawal request")
	}
	consolidationRequestBytes, err := hexutil.Decode("0x6600000000000000000000000000000000000000" +
		"670000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000" +
		"680000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000")
	if err != nil {
		panic("failed to decode consolidation request")
	}
	executionBundleFixtureElectra := &pb.GetPayloadV4ResponseJson{
		ShouldOverrideBuilder: true,
		ExecutionPayload: &pb.ExecutionPayloadDenebJSON{
			ParentHash:    &common.Hash{'a'},
			FeeRecipient:  &common.Address{'b'},
			StateRoot:     &common.Hash{'c'},
			ReceiptsRoot:  &common.Hash{'d'},
			LogsBloom:     &hexutil.Bytes{'e'},
			PrevRandao:    &common.Hash{'f'},
			BaseFeePerGas: "0x123",
			BlockHash:     &common.Hash{'g'},
			Transactions:  []hexutil.Bytes{{'h'}},
			Withdrawals:   []*pb.Withdrawal{},
			BlockNumber:   &hexUint,
			GasLimit:      &hexUint,
			GasUsed:       &hexUint,
			Timestamp:     &hexUint,
			BlobGasUsed:   &bgu,
			ExcessBlobGas: &ebg,
		},
		BlockValue: "0x11fffffffff",
		BlobsBundle: &pb.BlobBundleJSON{
			Commitments: []hexutil.Bytes{[]byte("commitment1"), []byte("commitment2")},
			Proofs:      []hexutil.Bytes{[]byte("proof1"), []byte("proof2")},
			Blobs:       []hexutil.Bytes{{'a'}, {'b'}},
		},
		ExecutionRequests: []hexutil.Bytes{depositRequestBytes, withdrawalRequestBytes, consolidationRequestBytes},
	}
	parent := bytesutil.PadTo([]byte("parentHash"), fieldparams.RootLength)
	sha3Uncles := bytesutil.PadTo([]byte("sha3Uncles"), fieldparams.RootLength)
	miner := bytesutil.PadTo([]byte("miner"), fieldparams.FeeRecipientLength)
	stateRoot := bytesutil.PadTo([]byte("stateRoot"), fieldparams.RootLength)
	transactionsRoot := bytesutil.PadTo([]byte("transactionsRoot"), fieldparams.RootLength)
	receiptsRoot := bytesutil.PadTo([]byte("receiptsRoot"), fieldparams.RootLength)
	logsBloom := bytesutil.PadTo([]byte("logs"), fieldparams.LogsBloomLength)
	executionBlock := &pb.ExecutionBlock{
		Version: version.Bellatrix,
		Header: gethtypes.Header{
			ParentHash:  common.BytesToHash(parent),
			UncleHash:   common.BytesToHash(sha3Uncles),
			Coinbase:    common.BytesToAddress(miner),
			Root:        common.BytesToHash(stateRoot),
			TxHash:      common.BytesToHash(transactionsRoot),
			ReceiptHash: common.BytesToHash(receiptsRoot),
			Bloom:       gethtypes.BytesToBloom(logsBloom),
			Difficulty:  big.NewInt(1),
			Number:      big.NewInt(2),
			GasLimit:    3,
			GasUsed:     4,
			Time:        5,
			Extra:       []byte("extra"),
			MixDigest:   common.BytesToHash([]byte("mix")),
			Nonce:       gethtypes.EncodeNonce(6),
			BaseFee:     big.NewInt(7),
		},
	}
	status := &pb.PayloadStatus{
		Status:          pb.PayloadStatus_VALID,
		LatestValidHash: foo[:],
		ValidationError: "",
	}
	id := pb.PayloadIDBytes([8]byte{1, 0, 0, 0, 0, 0, 0, 0})
	forkChoiceResp := &ForkchoiceUpdatedResponse{
		Status:    status,
		PayloadId: &id,
	}
	forkChoiceSyncingResp := &ForkchoiceUpdatedResponse{
		Status: &pb.PayloadStatus{
			Status:          pb.PayloadStatus_SYNCING,
			LatestValidHash: nil,
		},
		PayloadId: &id,
	}
	forkChoiceAcceptedResp := &ForkchoiceUpdatedResponse{
		Status: &pb.PayloadStatus{
			Status:          pb.PayloadStatus_ACCEPTED,
			LatestValidHash: nil,
		},
		PayloadId: &id,
	}
	forkChoiceInvalidResp := &ForkchoiceUpdatedResponse{
		Status: &pb.PayloadStatus{
			Status:          pb.PayloadStatus_INVALID,
			LatestValidHash: bytesutil.PadTo([]byte("latestValidHash"), 32),
		},
		PayloadId: &id,
	}
	validStatus := &pb.PayloadStatus{
		Status:          pb.PayloadStatus_VALID,
		LatestValidHash: foo[:],
		ValidationError: "",
	}
	inValidBlockHashStatus := &pb.PayloadStatus{
		Status:          pb.PayloadStatus_INVALID_BLOCK_HASH,
		LatestValidHash: nil,
	}
	acceptedStatus := &pb.PayloadStatus{
		Status:          pb.PayloadStatus_ACCEPTED,
		LatestValidHash: nil,
	}
	syncingStatus := &pb.PayloadStatus{
		Status:          pb.PayloadStatus_SYNCING,
		LatestValidHash: nil,
	}
	invalidStatus := &pb.PayloadStatus{
		Status:          pb.PayloadStatus_INVALID,
		LatestValidHash: foo[:],
	}
	unknownStatus := &pb.PayloadStatus{
		Status:          pb.PayloadStatus_UNKNOWN,
		LatestValidHash: foo[:],
	}
	return &payloadFixtures{
		ExecutionBlock:                    executionBlock,
		ExecutionPayloadBody:              executionPayloadBodyFixture,
		ExecutionPayload:                  executionPayloadFixture,
		ExecutionPayloadCapella:           executionPayloadFixtureCapella,
		ExecutionPayloadDeneb:             executionPayloadFixtureDeneb,
		EmptyExecutionPayloadDeneb:        emptyExecutionPayloadDeneb,
		ExecutionPayloadWithValueCapella:  executionPayloadWithValueFixtureCapella,
		ExecutionPayloadWithValueDeneb:    executionPayloadWithValueFixtureDeneb,
		ExecutionBundleElectra:            executionBundleFixtureElectra,
		ValidPayloadStatus:                validStatus,
		InvalidBlockHashStatus:            inValidBlockHashStatus,
		AcceptedStatus:                    acceptedStatus,
		SyncingStatus:                     syncingStatus,
		InvalidStatus:                     invalidStatus,
		UnknownStatus:                     unknownStatus,
		ForkchoiceUpdatedResponse:         forkChoiceResp,
		ForkchoiceUpdatedSyncingResponse:  forkChoiceSyncingResp,
		ForkchoiceUpdatedAcceptedResponse: forkChoiceAcceptedResp,
		ForkchoiceUpdatedInvalidResponse:  forkChoiceInvalidResp,
	}

}

type payloadFixtures struct {
	ExecutionBlock                    *pb.ExecutionBlock
	ExecutionPayloadBody              *pb.ExecutionPayloadBody
	ExecutionPayload                  *pb.ExecutionPayload
	ExecutionPayloadCapella           *pb.ExecutionPayloadCapella
	EmptyExecutionPayloadDeneb        *pb.ExecutionPayloadDeneb
	ExecutionPayloadDeneb             *pb.ExecutionPayloadDeneb
	ExecutionPayloadWithValueCapella  *pb.GetPayloadV2ResponseJson
	ExecutionPayloadWithValueDeneb    *pb.GetPayloadV3ResponseJson
	ExecutionBundleElectra            *pb.GetPayloadV4ResponseJson
	ValidPayloadStatus                *pb.PayloadStatus
	InvalidBlockHashStatus            *pb.PayloadStatus
	AcceptedStatus                    *pb.PayloadStatus
	SyncingStatus                     *pb.PayloadStatus
	InvalidStatus                     *pb.PayloadStatus
	UnknownStatus                     *pb.PayloadStatus
	ForkchoiceUpdatedResponse         *ForkchoiceUpdatedResponse
	ForkchoiceUpdatedSyncingResponse  *ForkchoiceUpdatedResponse
	ForkchoiceUpdatedAcceptedResponse *ForkchoiceUpdatedResponse
	ForkchoiceUpdatedInvalidResponse  *ForkchoiceUpdatedResponse
}

func TestHeaderByHash_NotFound(t *testing.T) {
	srv := &Service{}
	srv.rpcClient = RPCClientBad{}

	_, err := srv.HeaderByHash(context.Background(), [32]byte{})
	assert.Equal(t, ethereum.NotFound, err)
}

func TestHeaderByNumber_NotFound(t *testing.T) {
	srv := &Service{}
	srv.rpcClient = RPCClientBad{}

	_, err := srv.HeaderByNumber(context.Background(), big.NewInt(100))
	assert.Equal(t, ethereum.NotFound, err)
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

type testEngineService struct{}

func (*testEngineService) NoArgsRets() {}

func (*testEngineService) GetBlockByHash(
	_ context.Context, _ common.Hash, _ bool,
) *pb.ExecutionBlock {
	fix := fixtures()
	item, ok := fix["ExecutionBlock"].(*pb.ExecutionBlock)
	if !ok {
		panic("not found")
	}
	return item
}

func (*testEngineService) GetBlockByNumber(
	_ context.Context, _ string, _ bool,
) *pb.ExecutionBlock {
	fix := fixtures()
	item, ok := fix["ExecutionBlock"].(*pb.ExecutionBlock)
	if !ok {
		panic("not found")
	}
	return item
}

func (*testEngineService) GetPayloadV1(
	_ context.Context, _ pb.PayloadIDBytes,
) *pb.ExecutionPayload {
	fix := fixtures()
	item, ok := fix["ExecutionPayload"].(*pb.ExecutionPayload)
	if !ok {
		panic("not found")
	}
	return item
}

func (*testEngineService) GetPayloadV2(
	_ context.Context, _ pb.PayloadIDBytes,
) *pb.ExecutionPayloadCapellaWithValue {
	fix := fixtures()
	item, ok := fix["ExecutionPayloadCapellaWithValue"].(*pb.ExecutionPayloadCapellaWithValue)
	if !ok {
		panic("not found")
	}
	return item
}

func (*testEngineService) ForkchoiceUpdatedV1(
	_ context.Context, _ *pb.ForkchoiceState, _ *pb.PayloadAttributes,
) *ForkchoiceUpdatedResponse {
	fix := fixtures()
	item, ok := fix["ForkchoiceUpdatedResponse"].(*ForkchoiceUpdatedResponse)
	if !ok {
		panic("not found")
	}
	item.Status.Status = pb.PayloadStatus_VALID
	return item
}

func (*testEngineService) ForkchoiceUpdatedV2(
	_ context.Context, _ *pb.ForkchoiceState, _ *pb.PayloadAttributes,
) *ForkchoiceUpdatedResponse {
	fix := fixtures()
	item, ok := fix["ForkchoiceUpdatedResponse"].(*ForkchoiceUpdatedResponse)
	if !ok {
		panic("not found")
	}
	item.Status.Status = pb.PayloadStatus_VALID
	return item
}

func (*testEngineService) NewPayloadV1(
	_ context.Context, _ *pb.ExecutionPayload,
) *pb.PayloadStatus {
	fix := fixtures()
	item, ok := fix["ValidPayloadStatus"].(*pb.PayloadStatus)
	if !ok {
		panic("not found")
	}
	return item
}

func (*testEngineService) NewPayloadV2(
	_ context.Context, _ *pb.ExecutionPayloadCapella,
) *pb.PayloadStatus {
	fix := fixtures()
	item, ok := fix["ValidPayloadStatus"].(*pb.PayloadStatus)
	if !ok {
		panic("not found")
	}
	return item
}

func forkchoiceUpdateSetup(t *testing.T, fcs *pb.ForkchoiceState, att *pb.PayloadAttributes, res *ForkchoiceUpdatedResponse) *Service {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		defer func() {
			require.NoError(t, r.Body.Close())
		}()
		enc, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		jsonRequestString := string(enc)

		forkChoiceStateReq, err := json.Marshal(fcs)
		require.NoError(t, err)
		payloadAttrsReq, err := json.Marshal(att)
		require.NoError(t, err)

		// We expect the JSON string RPC request contains the right arguments.
		require.Equal(t, true, strings.Contains(
			jsonRequestString, string(forkChoiceStateReq),
		))
		require.Equal(t, true, strings.Contains(
			jsonRequestString, string(payloadAttrsReq),
		))
		resp := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"result":  res,
		}
		err = json.NewEncoder(w).Encode(resp)
		require.NoError(t, err)
	}))

	rpcClient, err := rpc.DialHTTP(srv.URL)
	require.NoError(t, err)

	service := &Service{}
	service.rpcClient = rpcClient
	return service
}

func forkchoiceUpdateSetupV2(t *testing.T, fcs *pb.ForkchoiceState, att *pb.PayloadAttributesV2, res *ForkchoiceUpdatedResponse) *Service {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		defer func() {
			require.NoError(t, r.Body.Close())
		}()
		enc, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		jsonRequestString := string(enc)

		forkChoiceStateReq, err := json.Marshal(fcs)
		require.NoError(t, err)
		payloadAttrsReq, err := json.Marshal(att)
		require.NoError(t, err)

		// We expect the JSON string RPC request contains the right arguments.
		require.Equal(t, true, strings.Contains(
			jsonRequestString, string(forkChoiceStateReq),
		))
		require.Equal(t, true, strings.Contains(
			jsonRequestString, string(payloadAttrsReq),
		))
		resp := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"result":  res,
		}
		err = json.NewEncoder(w).Encode(resp)
		require.NoError(t, err)
	}))

	rpcClient, err := rpc.DialHTTP(srv.URL)
	require.NoError(t, err)

	service := &Service{}
	service.rpcClient = rpcClient
	return service
}

func newPayloadSetup(t *testing.T, status *pb.PayloadStatus, payload *pb.ExecutionPayload) *Service {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		defer func() {
			require.NoError(t, r.Body.Close())
		}()
		enc, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		jsonRequestString := string(enc)

		reqArg, err := json.Marshal(payload)
		require.NoError(t, err)

		// We expect the JSON string RPC request contains the right arguments.
		require.Equal(t, true, strings.Contains(
			jsonRequestString, string(reqArg),
		))
		resp := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"result":  status,
		}
		err = json.NewEncoder(w).Encode(resp)
		require.NoError(t, err)
	}))

	rpcClient, err := rpc.DialHTTP(srv.URL)
	require.NoError(t, err)

	service := &Service{}
	service.rpcClient = rpcClient
	return service
}

func newPayloadV2Setup(t *testing.T, status *pb.PayloadStatus, payload *pb.ExecutionPayloadCapella) *Service {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		defer func() {
			require.NoError(t, r.Body.Close())
		}()
		enc, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		jsonRequestString := string(enc)

		reqArg, err := json.Marshal(payload)
		require.NoError(t, err)

		// We expect the JSON string RPC request contains the right arguments.
		require.Equal(t, true, strings.Contains(
			jsonRequestString, string(reqArg),
		))
		resp := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"result":  status,
		}
		err = json.NewEncoder(w).Encode(resp)
		require.NoError(t, err)
	}))

	rpcClient, err := rpc.DialHTTP(srv.URL)
	require.NoError(t, err)

	service := &Service{}
	service.rpcClient = rpcClient
	return service
}

func newPayloadV3Setup(t *testing.T, status *pb.PayloadStatus, payload *pb.ExecutionPayloadDeneb) *Service {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		defer func() {
			require.NoError(t, r.Body.Close())
		}()
		enc, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		jsonRequestString := string(enc)

		reqArg, err := json.Marshal(payload)
		require.NoError(t, err)

		// We expect the JSON string RPC request contains the right arguments.
		require.Equal(t, true, strings.Contains(
			jsonRequestString, string(reqArg),
		))
		resp := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"result":  status,
		}
		err = json.NewEncoder(w).Encode(resp)
		require.NoError(t, err)
	}))

	rpcClient, err := rpc.DialHTTP(srv.URL)
	require.NoError(t, err)

	service := &Service{}
	service.rpcClient = rpcClient
	return service
}

func newPayloadV4Setup(t *testing.T, status *pb.PayloadStatus, payload *pb.ExecutionPayloadDeneb, requests *pb.ExecutionRequests) *Service {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		defer func() {
			require.NoError(t, r.Body.Close())
		}()
		enc, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		jsonRequestString := string(enc)
		require.Equal(t, true, strings.Contains(
			jsonRequestString, string("engine_newPayloadV4"),
		))

		reqPayload, err := json.Marshal(payload)
		require.NoError(t, err)

		// We expect the JSON string RPC request contains the right arguments.
		require.Equal(t, true, strings.Contains(
			jsonRequestString, string(reqPayload),
		))

		reqRequests, err := pb.EncodeExecutionRequests(requests)
		require.NoError(t, err)

		jsonRequests, err := json.Marshal(reqRequests)
		require.NoError(t, err)

		require.Equal(t, true, strings.Contains(
			jsonRequestString, string(jsonRequests),
		))

		resp := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"result":  status,
		}
		err = json.NewEncoder(w).Encode(resp)
		require.NoError(t, err)
	}))

	rpcClient, err := rpc.DialHTTP(srv.URL)
	require.NoError(t, err)

	service := &Service{}
	service.rpcClient = rpcClient
	return service
}

func TestReconstructBlindedBlockBatch(t *testing.T) {
	t.Run("empty response works", func(t *testing.T) {
		ctx := context.Background()
		cli, srv := newMockEngine(t)
		srv.registerDefault(func(*jsonrpcMessage, http.ResponseWriter, *http.Request) {

			t.Fatal("http request should not be made")
		})
		results, err := reconstructBlindedBlockBatch(ctx, cli, []interfaces.ReadOnlySignedBeaconBlock{})
		require.NoError(t, err)
		require.Equal(t, 0, len(results))
	})
	t.Run("expected error for nil response", func(t *testing.T) {
		ctx := context.Background()
		slot, err := slots.EpochStart(params.BeaconConfig().DenebForkEpoch)
		require.NoError(t, err)
		blk, _ := util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, slot, 0)
		cli, srv := newMockEngine(t)
		srv.registerDefault(func(msg *jsonrpcMessage, w http.ResponseWriter, req *http.Request) {
			executionPayloadBodies := []*pb.ExecutionPayloadBody{nil}
			mockWriteResult(t, w, msg, executionPayloadBodies)
		})

		blinded, err := blk.ToBlinded()
		require.NoError(t, err)
		service := &Service{}
		service.rpcClient = cli
		_, err = service.ReconstructFullBlock(ctx, blinded)
		require.ErrorIs(t, err, errNilPayloadBody)
	})
}

func Test_ExchangeCapabilities(t *testing.T) {
	t.Run("empty response works", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			defer func() {
				require.NoError(t, r.Body.Close())
			}()
			resp := map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      1,
				"result":  []string{},
			}
			err := json.NewEncoder(w).Encode(resp)
			require.NoError(t, err)
		}))
		ctx := context.Background()
		logHook := logTest.NewGlobal()

		rpcClient, err := rpc.DialHTTP(srv.URL)
		require.NoError(t, err)

		service := &Service{}
		service.rpcClient = rpcClient

		results, err := service.ExchangeCapabilities(ctx)
		require.NoError(t, err)
		require.Equal(t, 0, len(results))

		for _, item := range results {
			require.NotNil(t, item)
		}
		assert.LogsContain(t, logHook, "Please update client, detected the following unsupported engine methods:")
	})
	t.Run("list of items", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			defer func() {
				require.NoError(t, r.Body.Close())
			}()

			resp := map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      1,
				"result":  []string{"A", "B", "C"},
			}
			err := json.NewEncoder(w).Encode(resp)
			require.NoError(t, err)
		}))
		ctx := context.Background()

		rpcClient, err := rpc.DialHTTP(srv.URL)
		require.NoError(t, err)

		service := &Service{}
		service.rpcClient = rpcClient

		results, err := service.ExchangeCapabilities(ctx)
		require.NoError(t, err)
		require.Equal(t, 3, len(results))

		for _, item := range results {
			require.NotNil(t, item)
		}
	})
}

func TestReconstructBlobSidecars(t *testing.T) {
	client := &Service{capabilityCache: &capabilityCache{}}
	b := util.NewBeaconBlockDeneb()
	kzgCommitments := createRandomKzgCommitments(t, 6)

	b.Block.Body.BlobKzgCommitments = kzgCommitments
	r, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	sb, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)

	ctx := context.Background()
	t.Run("all seen", func(t *testing.T) {
		exists := []bool{true, true, true, true, true, true}
		verifiedBlobs, err := client.ReconstructBlobSidecars(ctx, sb, r, exists)
		require.NoError(t, err)
		require.Equal(t, 0, len(verifiedBlobs))
	})

	t.Run("get-blobs end point is not supported", func(t *testing.T) {
		exists := []bool{true, true, true, true, true, false}
		verifiedBlobs, err := client.ReconstructBlobSidecars(ctx, sb, r, exists)
		require.NoError(t, err)
		require.Equal(t, 0, len(verifiedBlobs))
	})

	client.capabilityCache = &capabilityCache{capabilities: map[string]interface{}{GetBlobsV1: nil}}

	t.Run("recovered 6 missing blobs", func(t *testing.T) {
		srv := createBlobServer(t, 6)
		defer srv.Close()

		rpcClient, client := setupRpcClient(t, srv.URL, client)
		defer rpcClient.Close()

		exists := [6]bool{}
		verifiedBlobs, err := client.ReconstructBlobSidecars(ctx, sb, r, exists[:])
		require.NoError(t, err)
		require.Equal(t, 6, len(verifiedBlobs))
	})

	t.Run("recovered 3 missing blobs", func(t *testing.T) {
		srv := createBlobServer(t, 3)
		defer srv.Close()

		rpcClient, client := setupRpcClient(t, srv.URL, client)
		defer rpcClient.Close()

		exists := []bool{true, false, true, false, true, false}
		verifiedBlobs, err := client.ReconstructBlobSidecars(ctx, sb, r, exists)
		require.NoError(t, err)
		require.Equal(t, 3, len(verifiedBlobs))
	})

	t.Run("kzg is longer than exist", func(t *testing.T) {
		srv := createBlobServer(t, 3)
		defer srv.Close()

		rpcClient, client := setupRpcClient(t, srv.URL, client)
		defer rpcClient.Close()

		exists := []bool{true, false, true, false, true}
		_, err := client.ReconstructBlobSidecars(ctx, sb, r, exists)
		require.ErrorContains(t, "length of KZG commitments (6) is greater than length of exists (5)", err)
	})
}

func createRandomKzgCommitments(t *testing.T, num int) [][]byte {
	kzgCommitments := make([][]byte, num)
	for i := range kzgCommitments {
		kzgCommitments[i] = make([]byte, 48)
		_, err := rand.Read(kzgCommitments[i])
		require.NoError(t, err)
	}
	return kzgCommitments
}

func createBlobServer(t *testing.T, numBlobs int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		defer func() {
			require.NoError(t, r.Body.Close())
		}()

		blobs := make([]pb.BlobAndProofJson, numBlobs)
		for i := range blobs {
			blobs[i] = pb.BlobAndProofJson{Blob: []byte(fmt.Sprintf("blob%d", i+1)), KzgProof: []byte(fmt.Sprintf("proof%d", i+1))}
		}

		respJSON := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"result":  blobs,
		}
		require.NoError(t, json.NewEncoder(w).Encode(respJSON))
	}))
}

func setupRpcClient(t *testing.T, url string, client *Service) (*rpc.Client, *Service) {
	rpcClient, err := rpc.DialHTTP(url)
	require.NoError(t, err)

	client.rpcClient = rpcClient
	client.capabilityCache = &capabilityCache{capabilities: map[string]interface{}{GetBlobsV1: nil}}
	client.blobVerifier = testNewBlobVerifier()

	return rpcClient, client
}

func testNewBlobVerifier() verification.NewBlobVerifier {
	return func(b blocks.ROBlob, reqs []verification.Requirement) verification.BlobVerifier {
		return &verification.MockBlobVerifier{
			CbVerifiedROBlob: func() (blocks.VerifiedROBlob, error) {
				return blocks.VerifiedROBlob{}, nil
			},
		}
	}
}
