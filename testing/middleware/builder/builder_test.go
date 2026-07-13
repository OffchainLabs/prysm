package builder

import (
	"bytes"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"

	builderClient "github.com/OffchainLabs/prysm/v7/api/client/builder"
	v1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
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
