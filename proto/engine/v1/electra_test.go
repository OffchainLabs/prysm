package enginev1_test

import (
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

var depositRequestsSSZHex = "0x706b0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000077630000000000000000000000000000000000000000000000000000000000007b00000000000000736967000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000c801000000000000706b00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000776300000000000000000000000000000000000000000000000000000000000090010000000000007369670000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000002000000000000000"

func TestGetDecodedExecutionRequests(t *testing.T) {
	t.Run("All requests decode successfully", func(t *testing.T) {
		depositRequestBytes, err := hexutil.Decode("0x610000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000" +
			"620000000000000000000000000000000000000000000000000000000000000000" +
			"4059730700000063000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000" +
			"00000000000000000000000000000000000000000000000000000000000000000000000000000000")
		require.NoError(t, err)
		withdrawalRequestBytes, err := hexutil.Decode("0x6400000000000000000000000000000000000000" +
			"6500000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000040597307000000")
		require.NoError(t, err)
		consolidationRequestBytes, err := hexutil.Decode("0x6600000000000000000000000000000000000000" +
			"670000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000" +
			"680000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000")
		require.NoError(t, err)
		ebe := &enginev1.ExecutionBundleElectra{
			ExecutionRequests: [][]byte{append([]byte{uint8(enginev1.DepositRequestType)}, depositRequestBytes...),
				append([]byte{uint8(enginev1.WithdrawalRequestType)}, withdrawalRequestBytes...),
				append([]byte{uint8(enginev1.ConsolidationRequestType)}, consolidationRequestBytes...)},
		}
		requests, err := ebe.GetDecodedExecutionRequests()
		require.NoError(t, err)
		require.Equal(t, len(requests.Deposits), 1)
		require.Equal(t, len(requests.Withdrawals), 1)
		require.Equal(t, len(requests.Consolidations), 1)
	})
	t.Run("Excluded requests still decode successfully when one request is missing", func(t *testing.T) {
		depositRequestBytes, err := hexutil.Decode("0x610000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000" +
			"620000000000000000000000000000000000000000000000000000000000000000" +
			"4059730700000063000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000" +
			"00000000000000000000000000000000000000000000000000000000000000000000000000000000")
		require.NoError(t, err)
		consolidationRequestBytes, err := hexutil.Decode("0x6600000000000000000000000000000000000000" +
			"670000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000" +
			"680000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000")
		require.NoError(t, err)
		ebe := &enginev1.ExecutionBundleElectra{
			ExecutionRequests: [][]byte{append([]byte{uint8(enginev1.DepositRequestType)}, depositRequestBytes...), append([]byte{uint8(enginev1.ConsolidationRequestType)}, consolidationRequestBytes...)},
		}
		requests, err := ebe.GetDecodedExecutionRequests()
		require.NoError(t, err)
		require.Equal(t, len(requests.Deposits), 1)
		require.Equal(t, len(requests.Withdrawals), 0)
		require.Equal(t, len(requests.Consolidations), 1)
	})
	t.Run("Decode execution requests should fail if ordering is not sorted", func(t *testing.T) {
		depositRequestBytes, err := hexutil.Decode("0x610000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000" +
			"620000000000000000000000000000000000000000000000000000000000000000" +
			"4059730700000063000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000" +
			"00000000000000000000000000000000000000000000000000000000000000000000000000000000")
		require.NoError(t, err)
		consolidationRequestBytes, err := hexutil.Decode("0x6600000000000000000000000000000000000000" +
			"670000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000" +
			"680000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000")
		require.NoError(t, err)
		ebe := &enginev1.ExecutionBundleElectra{
			ExecutionRequests: [][]byte{append([]byte{uint8(enginev1.ConsolidationRequestType)}, consolidationRequestBytes...), append([]byte{uint8(enginev1.DepositRequestType)}, depositRequestBytes...)},
		}
		_, err = ebe.GetDecodedExecutionRequests()
		require.ErrorContains(t, "invalid execution request type order", err)
	})
	t.Run("Requests should error if the request type is shorter than 1 byte", func(t *testing.T) {
		consolidationRequestBytes, err := hexutil.Decode("0x6600000000000000000000000000000000000000" +
			"670000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000" +
			"680000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000")
		require.NoError(t, err)
		ebe := &enginev1.ExecutionBundleElectra{
			ExecutionRequests: [][]byte{append([]byte{}, []byte{}...), append([]byte{uint8(enginev1.ConsolidationRequestType)}, consolidationRequestBytes...)},
		}
		_, err = ebe.GetDecodedExecutionRequests()
		require.ErrorContains(t, "invalid execution request, length less than 1", err)
	})
	t.Run("a duplicate request should fail", func(t *testing.T) {
		withdrawalRequestBytes, err := hexutil.Decode("0x6400000000000000000000000000000000000000" +
			"6500000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000040597307000000")
		require.NoError(t, err)
		withdrawalRequestBytes2, err := hexutil.Decode("0x6400000000000000000000000000000000000000" +
			"6500000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000040597307000000")
		require.NoError(t, err)
		ebe := &enginev1.ExecutionBundleElectra{
			ExecutionRequests: [][]byte{append([]byte{uint8(enginev1.WithdrawalRequestType)}, withdrawalRequestBytes...), append([]byte{uint8(enginev1.WithdrawalRequestType)}, withdrawalRequestBytes2...)},
		}
		_, err = ebe.GetDecodedExecutionRequests()
		require.ErrorContains(t, "requests should be in sorted order and unique", err)
	})
	t.Run("a duplicate withdrawals ( non 0 request type )request should fail", func(t *testing.T) {
		depositRequestBytes, err := hexutil.Decode("0x610000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000" +
			"620000000000000000000000000000000000000000000000000000000000000000" +
			"4059730700000063000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000" +
			"00000000000000000000000000000000000000000000000000000000000000000000000000000000")
		require.NoError(t, err)
		depositRequestBytes2, err := hexutil.Decode("0x610000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000" +
			"620000000000000000000000000000000000000000000000000000000000000000" +
			"4059730700000063000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000" +
			"00000000000000000000000000000000000000000000000000000000000000000000000000000000")
		require.NoError(t, err)
		ebe := &enginev1.ExecutionBundleElectra{
			ExecutionRequests: [][]byte{append([]byte{uint8(enginev1.DepositRequestType)}, depositRequestBytes...), append([]byte{uint8(enginev1.DepositRequestType)}, depositRequestBytes2...)},
		}
		_, err = ebe.GetDecodedExecutionRequests()
		require.ErrorContains(t, "requests should be in sorted order and unique", err)
	})
	t.Run("If a request type is provided, but the request list is shorter than the ssz of 1 request we error", func(t *testing.T) {
		consolidationRequestBytes, err := hexutil.Decode("0x6600000000000000000000000000000000000000" +
			"670000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000" +
			"680000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000")
		require.NoError(t, err)
		ebe := &enginev1.ExecutionBundleElectra{
			ExecutionRequests: [][]byte{append([]byte{uint8(enginev1.DepositRequestType)}, []byte{}...), append([]byte{uint8(enginev1.ConsolidationRequestType)}, consolidationRequestBytes...)},
		}
		_, err = ebe.GetDecodedExecutionRequests()
		require.ErrorContains(t, "invalid deposit requests length", err)
	})
	t.Run("If deposit requests are over the max allowed per payload then we should error", func(t *testing.T) {
		requests := make([]*enginev1.DepositRequest, params.BeaconConfig().MaxDepositRequestsPerPayload+1)
		for i := range requests {
			requests[i] = &enginev1.DepositRequest{
				Pubkey:                bytesutil.PadTo([]byte("pk"), 48),
				WithdrawalCredentials: bytesutil.PadTo([]byte("wc"), 32),
				Amount:                123,
				Signature:             bytesutil.PadTo([]byte("sig"), 96),
				Index:                 456,
			}
		}
		by, err := enginev1.MarshalItems(requests)
		require.NoError(t, err)
		ebe := &enginev1.ExecutionBundleElectra{
			ExecutionRequests: [][]byte{
				append([]byte{uint8(enginev1.DepositRequestType)}, by...),
			},
		}
		_, err = ebe.GetDecodedExecutionRequests()
		require.ErrorContains(t, "invalid deposit requests length, requests should not be more than the max per payload", err)
	})
	t.Run("If withdrawal requests are over the max allowed per payload then we should error", func(t *testing.T) {
		requests := make([]*enginev1.WithdrawalRequest, params.BeaconConfig().MaxWithdrawalRequestsPerPayload+1)
		for i := range requests {
			requests[i] = &enginev1.WithdrawalRequest{
				SourceAddress:   bytesutil.PadTo([]byte("sa"), 20),
				ValidatorPubkey: bytesutil.PadTo([]byte("pk"), 48),
				Amount:          55555,
			}
		}
		by, err := enginev1.MarshalItems(requests)
		require.NoError(t, err)
		ebe := &enginev1.ExecutionBundleElectra{
			ExecutionRequests: [][]byte{
				append([]byte{uint8(enginev1.WithdrawalRequestType)}, by...),
			},
		}
		_, err = ebe.GetDecodedExecutionRequests()
		require.ErrorContains(t, "invalid withdrawal requests length, requests should not be more than the max per payload", err)
	})
	t.Run("If consolidation requests are over the max allowed per payload then we should error", func(t *testing.T) {
		requests := make([]*enginev1.ConsolidationRequest, params.BeaconConfig().MaxConsolidationsRequestsPerPayload+1)
		for i := range requests {
			requests[i] = &enginev1.ConsolidationRequest{
				SourceAddress: bytesutil.PadTo([]byte("sa"), 20),
				SourcePubkey:  bytesutil.PadTo([]byte("pk"), 48),
				TargetPubkey:  bytesutil.PadTo([]byte("pk"), 48),
			}
		}
		by, err := enginev1.MarshalItems(requests)
		require.NoError(t, err)
		ebe := &enginev1.ExecutionBundleElectra{
			ExecutionRequests: [][]byte{
				append([]byte{uint8(enginev1.ConsolidationRequestType)}, by...),
			},
		}
		_, err = ebe.GetDecodedExecutionRequests()
		require.ErrorContains(t, "invalid consolidation requests length, requests should not be more than the max per payload", err)
	})
}

func TestEncodeExecutionRequests(t *testing.T) {
	t.Run("Empty execution requests should return an empty response and not nil", func(t *testing.T) {
		ebe := &enginev1.ExecutionRequests{}
		b, err := enginev1.EncodeExecutionRequests(ebe)
		require.NoError(t, err)
		require.NotNil(t, b)
		require.Equal(t, len(b), 0)
	})
}

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
