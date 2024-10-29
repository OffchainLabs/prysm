package enginev1_test

import (
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

var depositRequestsSSZHex = "0x706b0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000077630000000000000000000000000000000000000000000000000000000000007b00000000000000736967000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000c801000000000000706b00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000776300000000000000000000000000000000000000000000000000000000000090010000000000007369670000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000002000000000000000"

func TestUnmarshalItems_OK(t *testing.T) {
	drb, err := hexutil.Decode(depositRequestsSSZHex)
	require.NoError(t, err)
	exampleRequest := &enginev1.DepositRequest{}
	depositRequests, err := enginev1.UnmarshalItems(drb, exampleRequest.SizeSSZ(), func() *enginev1.DepositRequest { return &enginev1.DepositRequest{} })
	require.NoError(t, err)

	exampleRequest1 := &enginev1.DepositRequest{
		Pubkey:                bytesutil.PadTo([]byte("pk"), 48),
		WithdrawalCredentials: bytesutil.PadTo([]byte("wc"), 32),
		Amount:                123,
		Signature:             bytesutil.PadTo([]byte("sig"), 96),
		Index:                 456,
	}
	exampleRequest2 := &enginev1.DepositRequest{
		Pubkey:                bytesutil.PadTo([]byte("pk"), 48),
		WithdrawalCredentials: bytesutil.PadTo([]byte("wc"), 32),
		Amount:                400,
		Signature:             bytesutil.PadTo([]byte("sig"), 96),
		Index:                 32,
	}
	require.DeepEqual(t, depositRequests, []*enginev1.DepositRequest{exampleRequest1, exampleRequest2})
}

func TestMarshalItems_OK(t *testing.T) {
	exampleRequest1 := &enginev1.DepositRequest{
		Pubkey:                bytesutil.PadTo([]byte("pk"), 48),
		WithdrawalCredentials: bytesutil.PadTo([]byte("wc"), 32),
		Amount:                123,
		Signature:             bytesutil.PadTo([]byte("sig"), 96),
		Index:                 456,
	}
	exampleRequest2 := &enginev1.DepositRequest{
		Pubkey:                bytesutil.PadTo([]byte("pk"), 48),
		WithdrawalCredentials: bytesutil.PadTo([]byte("wc"), 32),
		Amount:                400,
		Signature:             bytesutil.PadTo([]byte("sig"), 96),
		Index:                 32,
	}
	drbs, err := enginev1.MarshalItems([]*enginev1.DepositRequest{exampleRequest1, exampleRequest2})
	require.NoError(t, err)
	require.DeepEqual(t, depositRequestsSSZHex, hexutil.Encode(drbs))
}
