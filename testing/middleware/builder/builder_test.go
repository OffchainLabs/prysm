package builder

import (
	"bytes"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OffchainLabs/prysm/v7/api"
	builderClient "github.com/OffchainLabs/prysm/v7/api/client/builder"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	v1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/ethereum/go-ethereum/beacon/engine"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	gethRPC "github.com/ethereum/go-ethereum/rpc"
	"github.com/sirupsen/logrus"
)

type testEngineService struct {
	payload *engine.ExecutableData
}

func (s *testEngineService) GetPayloadV1(v1.PayloadIDBytes) (*engine.ExecutableData, error) {
	return s.payload, nil
}

func TestRegisterValidatorsAcceptsSSZ(t *testing.T) {
	pubkey := bytes.Repeat([]byte{0x42}, 48)
	registration := &eth.SignedValidatorRegistrationV1{
		Message: &eth.ValidatorRegistrationV1{
			FeeRecipient: bytes.Repeat([]byte{0x24}, 20),
			GasLimit:     30_000_000,
			Timestamp:    1,
			Pubkey:       pubkey,
		},
		Signature: bytes.Repeat([]byte{0x11}, 96),
	}
	b := &Builder{validatorMap: make(map[string]*eth.ValidatorRegistrationV1)}
	server := httptest.NewServer(http.HandlerFunc(b.registerValidators))
	defer server.Close()
	client, err := builderClient.NewClient(server.URL, builderClient.WithSSZ())
	require.NoError(t, err)

	require.NoError(t, client.RegisterValidator(t.Context(), []*eth.SignedValidatorRegistrationV1{registration}))

	require.DeepEqual(t, registration.Message, b.validatorMap[hexutil.Encode(pubkey)])
}

func TestGetHeaderReturnsSSZWithVersionHeader(t *testing.T) {
	rpcServer := gethRPC.NewServer()
	payload := &engine.ExecutableData{
		ParentHash:    common.Hash{1},
		FeeRecipient:  common.Address{2},
		StateRoot:     common.Hash{3},
		ReceiptsRoot:  common.Hash{4},
		LogsBloom:     bytes.Repeat([]byte{5}, 256),
		Random:        common.Hash{6},
		Number:        1,
		GasLimit:      30_000_000,
		GasUsed:       1,
		Timestamp:     1,
		ExtraData:     []byte{7},
		BaseFeePerGas: big.NewInt(1),
		BlockHash:     common.Hash{8},
		Transactions:  [][]byte{},
	}
	require.NoError(t, rpcServer.RegisterName("engine", &testEngineService{payload: payload}))
	t.Cleanup(rpcServer.Stop)
	b := &Builder{
		cfg:        &config{logger: logrus.New()},
		currId:     &v1.PayloadIDBytes{1},
		execClient: gethRPC.DialInProc(rpcServer),
	}
	mux := http.NewServeMux()
	mux.HandleFunc(headerPath, b.handleHeaderRequest)
	server := httptest.NewServer(mux)
	defer server.Close()
	client, err := builderClient.NewClient(server.URL, builderClient.WithSSZ())
	require.NoError(t, err)

	_, err = client.GetHeader(t.Context(), 1, [32]byte{1}, [48]byte{2})
	require.NoError(t, err)
}

func TestHandleBlindedBlockAcceptsSSZ(t *testing.T) {
	b, payload := denebBuilderWithPayload(t)
	blindedBlock := util.NewBlindedBeaconBlockDeneb()
	body, err := blindedBlock.MarshalSSZ()
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/eth/v1/builder/blinded_blocks", bytes.NewReader(body))
	req.Header.Set("Content-Type", api.OctetStreamMediaType)
	req.Header.Set("Accept", api.OctetStreamMediaType)
	recorder := httptest.NewRecorder()

	b.handleBlindedBlock(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, api.OctetStreamMediaType, recorder.Header().Get("Content-Type"))
	require.Equal(t, version.String(version.Deneb), recorder.Header().Get(api.VersionHeader))
	response := &v1.ExecutionPayloadDenebAndBlobsBundle{}
	require.NoError(t, response.UnmarshalSSZ(recorder.Body.Bytes()))
	require.DeepEqual(t, payload.BlockHash, response.Payload.BlockHash)
}

func TestHandleBlindedBlockRejectsMalformedSSZ(t *testing.T) {
	b, _ := denebBuilderWithPayload(t)
	req := httptest.NewRequest(http.MethodPost, "/eth/v1/builder/blinded_blocks", bytes.NewReader([]byte{1, 2, 3}))
	req.Header.Set("Content-Type", api.OctetStreamMediaType)
	recorder := httptest.NewRecorder()

	b.handleBlindedBlock(recorder, req)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
}

func denebBuilderWithPayload(t *testing.T) (*Builder, *v1.ExecutionPayloadDeneb) {
	t.Helper()
	payload := &v1.ExecutionPayloadDeneb{
		ParentHash:    bytes.Repeat([]byte{1}, 32),
		FeeRecipient:  bytes.Repeat([]byte{2}, 20),
		StateRoot:     bytes.Repeat([]byte{3}, 32),
		ReceiptsRoot:  bytes.Repeat([]byte{4}, 32),
		LogsBloom:     bytes.Repeat([]byte{5}, 256),
		PrevRandao:    bytes.Repeat([]byte{6}, 32),
		ExtraData:     []byte{},
		BaseFeePerGas: bytes.Repeat([]byte{7}, 32),
		BlockHash:     bytes.Repeat([]byte{8}, 32),
		Transactions:  [][]byte{},
		Withdrawals:   []*v1.Withdrawal{},
	}
	wrapped, err := blocks.WrappedExecutionPayloadDeneb(payload)
	require.NoError(t, err)
	return &Builder{
		cfg:         &config{logger: logrus.New()},
		currVersion: version.Deneb,
		currPayload: wrapped,
		blobBundle:  &v1.BlobsBundle{},
	}, payload
}
