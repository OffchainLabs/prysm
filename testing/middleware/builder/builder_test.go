package builder

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	v1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/sirupsen/logrus"
)

func testBuilder() *Builder {
	return &Builder{
		cfg:          &config{logger: logrus.New()},
		validatorMap: map[string]*eth.ValidatorRegistrationV1{},
		valLock:      sync.RWMutex{},
	}
}

func TestRegisterValidatorsSSZ(t *testing.T) {
	p := testBuilder()
	regs := []*eth.SignedValidatorRegistrationV1{
		{
			Message: &eth.ValidatorRegistrationV1{
				FeeRecipient: bytes.Repeat([]byte{0xaa}, 20),
				GasLimit:     30000000,
				Timestamp:    1,
				Pubkey:       bytes.Repeat([]byte{0xbb}, 48),
			},
			Signature: bytes.Repeat([]byte{0xcc}, 96),
		},
		{
			Message: &eth.ValidatorRegistrationV1{
				FeeRecipient: bytes.Repeat([]byte{0xdd}, 20),
				GasLimit:     30000000,
				Timestamp:    2,
				Pubkey:       bytes.Repeat([]byte{0xee}, 48),
			},
			Signature: bytes.Repeat([]byte{0xff}, 96),
		},
	}
	var body []byte
	for _, r := range regs {
		enc, err := r.MarshalSSZ()
		require.NoError(t, err)
		body = append(body, enc...)
	}

	req := httptest.NewRequest(http.MethodPost, "/eth/v1/builder/validators", bytes.NewReader(body))
	req.Header.Set("Content-Type", api.OctetStreamMediaType)
	w := httptest.NewRecorder()
	p.registerValidators(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, 2, len(p.validatorMap))
	got, ok := p.validatorMap[hexutil.Encode(regs[0].Message.Pubkey)]
	require.Equal(t, true, ok)
	require.Equal(t, regs[0].Message.GasLimit, got.GasLimit)
}

func TestRegisterValidatorsSSZInvalidLength(t *testing.T) {
	p := testBuilder()
	req := httptest.NewRequest(http.MethodPost, "/eth/v1/builder/validators", bytes.NewReader([]byte{1, 2, 3}))
	req.Header.Set("Content-Type", api.OctetStreamMediaType)
	w := httptest.NewRecorder()
	p.registerValidators(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestWriteHeaderResponseSSZ(t *testing.T) {
	p := testBuilder()
	bid := &eth.SignedBuilderBidDeneb{
		Message: &eth.BuilderBidDeneb{
			Header: &v1.ExecutionPayloadHeaderDeneb{
				ParentHash:       bytes.Repeat([]byte{0x01}, 32),
				FeeRecipient:     bytes.Repeat([]byte{0x02}, 20),
				StateRoot:        bytes.Repeat([]byte{0x03}, 32),
				ReceiptsRoot:     bytes.Repeat([]byte{0x04}, 32),
				LogsBloom:        bytes.Repeat([]byte{0x05}, 256),
				PrevRandao:       bytes.Repeat([]byte{0x06}, 32),
				BaseFeePerGas:    bytes.Repeat([]byte{0x07}, 32),
				BlockHash:        bytes.Repeat([]byte{0x08}, 32),
				TransactionsRoot: bytes.Repeat([]byte{0x09}, 32),
				WithdrawalsRoot:  bytes.Repeat([]byte{0x0a}, 32),
			},
			Value:  bytes.Repeat([]byte{0x01}, 32),
			Pubkey: bytes.Repeat([]byte{0x02}, 48),
		},
		Signature: bytes.Repeat([]byte{0x03}, 96),
	}

	req := httptest.NewRequest(http.MethodGet, "/eth/v1/builder/header/1/0xab/0xcd", nil)
	req.Header.Set("Accept", api.OctetStreamMediaType)
	w := httptest.NewRecorder()
	require.NoError(t, p.writeHeaderResponse(w, req, version.Deneb, nil, bid))

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, version.String(version.Deneb), w.Header().Get(api.VersionHeader))
	require.Equal(t, api.OctetStreamMediaType, w.Header().Get("Content-Type"))
	got := &eth.SignedBuilderBidDeneb{}
	require.NoError(t, got.UnmarshalSSZ(w.Body.Bytes()))
	require.DeepEqual(t, bid.Message.Value, got.Message.Value)
}

func TestHandleBlindedBlockSSZ(t *testing.T) {
	p := testBuilder()
	payload := &v1.ExecutionPayloadDeneb{
		ParentHash:    bytes.Repeat([]byte{0x01}, 32),
		FeeRecipient:  bytes.Repeat([]byte{0x02}, 20),
		StateRoot:     bytes.Repeat([]byte{0x03}, 32),
		ReceiptsRoot:  bytes.Repeat([]byte{0x04}, 32),
		LogsBloom:     bytes.Repeat([]byte{0x05}, 256),
		PrevRandao:    bytes.Repeat([]byte{0x06}, 32),
		BaseFeePerGas: bytes.Repeat([]byte{0x07}, 32),
		BlockHash:     bytes.Repeat([]byte{0x08}, 32),
	}
	wrapped, err := blocks.WrappedExecutionPayloadDeneb(payload)
	require.NoError(t, err)
	p.currVersion = version.Deneb
	p.currPayload = wrapped
	p.blobBundle = &v1.BlobsBundle{}

	blk := util.NewBlindedBeaconBlockDeneb()
	body, err := blk.MarshalSSZ()
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/eth/v1/builder/blinded_blocks", bytes.NewReader(body))
	req.Header.Set("Content-Type", api.OctetStreamMediaType)
	req.Header.Set("Accept", api.OctetStreamMediaType)
	req.Header.Set(api.VersionHeader, version.String(version.Deneb))
	w := httptest.NewRecorder()
	p.handleBlindedBlock(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, version.String(version.Deneb), w.Header().Get(api.VersionHeader))
	got := &v1.ExecutionPayloadDenebAndBlobsBundle{}
	require.NoError(t, got.UnmarshalSSZ(w.Body.Bytes()))
	require.DeepEqual(t, payload.BlockHash, got.Payload.BlockHash)
}
