package structs

import (
	"fmt"
	"testing"

	enginev1 "github.com/OffchainLabs/prysm/v6/proto/engine/v1"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func fillByteSlice(sliceLength int, value byte) []byte {
	bytes := make([]byte, sliceLength)

	for index := range bytes {
		bytes[index] = value
	}

	return bytes
}

// TestExecutionPayloadFromConsensus_HappyPath checks the
// ExecutionPayloadFromConsensus function under normal conditions.
func TestExecutionPayloadFromConsensus_HappyPath(t *testing.T) {
	consensusPayload := &enginev1.ExecutionPayload{
		ParentHash:    fillByteSlice(common.HashLength, 0xaa),
		FeeRecipient:  fillByteSlice(20, 0xbb),
		StateRoot:     fillByteSlice(32, 0xcc),
		ReceiptsRoot:  fillByteSlice(32, 0xdd),
		LogsBloom:     fillByteSlice(256, 0xee),
		PrevRandao:    fillByteSlice(32, 0xff),
		BlockNumber:   12345,
		GasLimit:      15000000,
		GasUsed:       8000000,
		Timestamp:     1680000000,
		ExtraData:     fillByteSlice(8, 0x11),
		BaseFeePerGas: fillByteSlice(32, 0x01),
		BlockHash:     fillByteSlice(common.HashLength, 0x22),
		Transactions: [][]byte{
			fillByteSlice(10, 0x33),
			fillByteSlice(10, 0x44),
		},
	}

	result, err := ExecutionPayloadFromConsensus(consensusPayload)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, hexutil.Encode(consensusPayload.ParentHash), result.ParentHash)
	require.Equal(t, hexutil.Encode(consensusPayload.FeeRecipient), result.FeeRecipient)
	require.Equal(t, hexutil.Encode(consensusPayload.StateRoot), result.StateRoot)
	require.Equal(t, hexutil.Encode(consensusPayload.ReceiptsRoot), result.ReceiptsRoot)
	require.Equal(t, fmt.Sprintf("%d", consensusPayload.BlockNumber), result.BlockNumber)
}

// TestExecutionPayload_ToConsensus_HappyPath checks the
// (*ExecutionPayload).ToConsensus function under normal conditions.
func TestExecutionPayload_ToConsensus_HappyPath(t *testing.T) {
	payload := &ExecutionPayload{
		ParentHash:    hexutil.Encode(fillByteSlice(common.HashLength, 0xaa)),
		FeeRecipient:  hexutil.Encode(fillByteSlice(20, 0xbb)),
		StateRoot:     hexutil.Encode(fillByteSlice(32, 0xcc)),
		ReceiptsRoot:  hexutil.Encode(fillByteSlice(32, 0xdd)),
		LogsBloom:     hexutil.Encode(fillByteSlice(256, 0xee)),
		PrevRandao:    hexutil.Encode(fillByteSlice(32, 0xff)),
		BlockNumber:   "12345",
		GasLimit:      "15000000",
		GasUsed:       "8000000",
		Timestamp:     "1680000000",
		ExtraData:     "0x11111111",
		BaseFeePerGas: "1234",
		BlockHash:     hexutil.Encode(fillByteSlice(common.HashLength, 0x22)),
		Transactions: []string{
			hexutil.Encode(fillByteSlice(10, 0x33)),
			hexutil.Encode(fillByteSlice(10, 0x44)),
		},
	}

	result, err := payload.ToConsensus()
	require.NoError(t, err)
	require.DeepEqual(t, result.ParentHash, fillByteSlice(common.HashLength, 0xaa))
	require.DeepEqual(t, result.FeeRecipient, fillByteSlice(20, 0xbb))
	require.DeepEqual(t, result.StateRoot, fillByteSlice(32, 0xcc))
}

// TestExecutionPayloadHeaderFromConsensus_HappyPath checks the
// ExecutionPayloadHeaderFromConsensus function under normal conditions.
func TestExecutionPayloadHeaderFromConsensus_HappyPath(t *testing.T) {
	consensusHeader := &enginev1.ExecutionPayloadHeader{
		ParentHash:       fillByteSlice(common.HashLength, 0xaa),
		FeeRecipient:     fillByteSlice(20, 0xbb),
		StateRoot:        fillByteSlice(32, 0xcc),
		ReceiptsRoot:     fillByteSlice(32, 0xdd),
		LogsBloom:        fillByteSlice(256, 0xee),
		PrevRandao:       fillByteSlice(32, 0xff),
		BlockNumber:      9999,
		GasLimit:         5000000,
		GasUsed:          2500000,
		Timestamp:        1111111111,
		ExtraData:        fillByteSlice(4, 0x12),
		BaseFeePerGas:    fillByteSlice(32, 0x34),
		BlockHash:        fillByteSlice(common.HashLength, 0x56),
		TransactionsRoot: fillByteSlice(32, 0x78),
	}

	result, err := ExecutionPayloadHeaderFromConsensus(consensusHeader)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, hexutil.Encode(consensusHeader.ParentHash), result.ParentHash)
	require.Equal(t, fmt.Sprintf("%d", consensusHeader.BlockNumber), result.BlockNumber)
}

// TestExecutionPayloadHeader_ToConsensus_HappyPath checks the
// (*ExecutionPayloadHeader).ToConsensus function under normal conditions.
func TestExecutionPayloadHeader_ToConsensus_HappyPath(t *testing.T) {
	header := &ExecutionPayloadHeader{
		ParentHash:       hexutil.Encode(fillByteSlice(common.HashLength, 0xaa)),
		FeeRecipient:     hexutil.Encode(fillByteSlice(20, 0xbb)),
		StateRoot:        hexutil.Encode(fillByteSlice(32, 0xcc)),
		ReceiptsRoot:     hexutil.Encode(fillByteSlice(32, 0xdd)),
		LogsBloom:        hexutil.Encode(fillByteSlice(256, 0xee)),
		PrevRandao:       hexutil.Encode(fillByteSlice(32, 0xff)),
		BlockNumber:      "9999",
		GasLimit:         "5000000",
		GasUsed:          "2500000",
		Timestamp:        "1111111111",
		ExtraData:        "0x1234abcd",
		BaseFeePerGas:    "1234",
		BlockHash:        hexutil.Encode(fillByteSlice(common.HashLength, 0x56)),
		TransactionsRoot: hexutil.Encode(fillByteSlice(32, 0x78)),
	}

	result, err := header.ToConsensus()
	require.NoError(t, err)
	require.DeepEqual(t, hexutil.Encode(result.ParentHash), header.ParentHash)
	require.DeepEqual(t, hexutil.Encode(result.FeeRecipient), header.FeeRecipient)
	require.DeepEqual(t, hexutil.Encode(result.StateRoot), header.StateRoot)
}

// TestExecutionPayloadCapellaFromConsensus_HappyPath checks the
// ExecutionPayloadCapellaFromConsensus function under normal conditions.
func TestExecutionPayloadCapellaFromConsensus_HappyPath(t *testing.T) {
	capellaPayload := &enginev1.ExecutionPayloadCapella{
		ParentHash:    fillByteSlice(common.HashLength, 0xaa),
		FeeRecipient:  fillByteSlice(20, 0xbb),
		StateRoot:     fillByteSlice(32, 0xcc),
		ReceiptsRoot:  fillByteSlice(32, 0xdd),
		LogsBloom:     fillByteSlice(256, 0xee),
		PrevRandao:    fillByteSlice(32, 0xff),
		BlockNumber:   123,
		GasLimit:      9876543,
		GasUsed:       1234567,
		Timestamp:     5555555,
		ExtraData:     fillByteSlice(6, 0x11),
		BaseFeePerGas: fillByteSlice(32, 0x22),
		BlockHash:     fillByteSlice(common.HashLength, 0x33),
		Transactions: [][]byte{
			fillByteSlice(5, 0x44),
		},
		Withdrawals: []*enginev1.Withdrawal{
			{
				Index:          1,
				ValidatorIndex: 2,
				Address:        fillByteSlice(20, 0xaa),
				Amount:         100,
			},
		},
	}

	result, err := ExecutionPayloadCapellaFromConsensus(capellaPayload)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, hexutil.Encode(capellaPayload.ParentHash), result.ParentHash)
	require.Equal(t, len(capellaPayload.Transactions), len(result.Transactions))
	require.Equal(t, len(capellaPayload.Withdrawals), len(result.Withdrawals))
}

// TestExecutionPayloadCapella_ToConsensus_HappyPath checks the
// (*ExecutionPayloadCapella).ToConsensus function under normal conditions.
func TestExecutionPayloadCapella_ToConsensus_HappyPath(t *testing.T) {
	capella := &ExecutionPayloadCapella{
		ParentHash:    hexutil.Encode(fillByteSlice(common.HashLength, 0xaa)),
		FeeRecipient:  hexutil.Encode(fillByteSlice(20, 0xbb)),
		StateRoot:     hexutil.Encode(fillByteSlice(32, 0xcc)),
		ReceiptsRoot:  hexutil.Encode(fillByteSlice(32, 0xdd)),
		LogsBloom:     hexutil.Encode(fillByteSlice(256, 0xee)),
		PrevRandao:    hexutil.Encode(fillByteSlice(32, 0xff)),
		BlockNumber:   "123",
		GasLimit:      "9876543",
		GasUsed:       "1234567",
		Timestamp:     "5555555",
		ExtraData:     hexutil.Encode(fillByteSlice(6, 0x11)),
		BaseFeePerGas: "1234",
		BlockHash:     hexutil.Encode(fillByteSlice(common.HashLength, 0x33)),
		Transactions: []string{
			hexutil.Encode(fillByteSlice(5, 0x44)),
		},
		Withdrawals: []*Withdrawal{
			{
				WithdrawalIndex:  "1",
				ValidatorIndex:   "2",
				ExecutionAddress: hexutil.Encode(fillByteSlice(20, 0xaa)),
				Amount:           "100",
			},
		},
	}

	result, err := capella.ToConsensus()
	require.NoError(t, err)
	require.DeepEqual(t, hexutil.Encode(result.ParentHash), capella.ParentHash)
	require.DeepEqual(t, hexutil.Encode(result.FeeRecipient), capella.FeeRecipient)
	require.DeepEqual(t, hexutil.Encode(result.StateRoot), capella.StateRoot)
}

// TestExecutionPayloadDenebFromConsensus_HappyPath checks the
// ExecutionPayloadDenebFromConsensus function under normal conditions.
func TestExecutionPayloadDenebFromConsensus_HappyPath(t *testing.T) {
	denebPayload := &enginev1.ExecutionPayloadDeneb{
		ParentHash:    fillByteSlice(common.HashLength, 0xaa),
		FeeRecipient:  fillByteSlice(20, 0xbb),
		StateRoot:     fillByteSlice(32, 0xcc),
		ReceiptsRoot:  fillByteSlice(32, 0xdd),
		LogsBloom:     fillByteSlice(256, 0xee),
		PrevRandao:    fillByteSlice(32, 0xff),
		BlockNumber:   999,
		GasLimit:      2222222,
		GasUsed:       1111111,
		Timestamp:     666666,
		ExtraData:     fillByteSlice(6, 0x11),
		BaseFeePerGas: fillByteSlice(32, 0x22),
		BlockHash:     fillByteSlice(common.HashLength, 0x33),
		Transactions: [][]byte{
			fillByteSlice(5, 0x44),
		},
		Withdrawals: []*enginev1.Withdrawal{
			{
				Index:          1,
				ValidatorIndex: 2,
				Address:        fillByteSlice(20, 0xaa),
				Amount:         100,
			},
		},
		BlobGasUsed:   1234,
		ExcessBlobGas: 5678,
	}

	result, err := ExecutionPayloadDenebFromConsensus(denebPayload)
	require.NoError(t, err)
	require.Equal(t, hexutil.Encode(denebPayload.ParentHash), result.ParentHash)
	require.Equal(t, len(denebPayload.Transactions), len(result.Transactions))
	require.Equal(t, len(denebPayload.Withdrawals), len(result.Withdrawals))
	require.Equal(t, "1234", result.BlobGasUsed)
	require.Equal(t, fmt.Sprintf("%d", denebPayload.BlockNumber), result.BlockNumber)
}

// TestExecutionPayloadDeneb_ToConsensus_HappyPath checks the
// (*ExecutionPayloadDeneb).ToConsensus function under normal conditions.
func TestExecutionPayloadDeneb_ToConsensus_HappyPath(t *testing.T) {
	deneb := &ExecutionPayloadDeneb{
		ParentHash:    hexutil.Encode(fillByteSlice(common.HashLength, 0xaa)),
		FeeRecipient:  hexutil.Encode(fillByteSlice(20, 0xbb)),
		StateRoot:     hexutil.Encode(fillByteSlice(32, 0xcc)),
		ReceiptsRoot:  hexutil.Encode(fillByteSlice(32, 0xdd)),
		LogsBloom:     hexutil.Encode(fillByteSlice(256, 0xee)),
		PrevRandao:    hexutil.Encode(fillByteSlice(32, 0xff)),
		BlockNumber:   "999",
		GasLimit:      "2222222",
		GasUsed:       "1111111",
		Timestamp:     "666666",
		ExtraData:     hexutil.Encode(fillByteSlice(6, 0x11)),
		BaseFeePerGas: "1234",
		BlockHash:     hexutil.Encode(fillByteSlice(common.HashLength, 0x33)),
		Transactions: []string{
			hexutil.Encode(fillByteSlice(5, 0x44)),
		},
		Withdrawals: []*Withdrawal{
			{
				WithdrawalIndex:  "1",
				ValidatorIndex:   "2",
				ExecutionAddress: hexutil.Encode(fillByteSlice(20, 0xaa)),
				Amount:           "100",
			},
		},
		BlobGasUsed:   "1234",
		ExcessBlobGas: "5678",
	}

	result, err := deneb.ToConsensus()
	require.NoError(t, err)
	require.DeepEqual(t, hexutil.Encode(result.ParentHash), deneb.ParentHash)
	require.DeepEqual(t, hexutil.Encode(result.FeeRecipient), deneb.FeeRecipient)
	require.Equal(t, result.BlockNumber, uint64(999))
}

func TestExecutionPayloadHeaderCapellaFromConsensus_HappyPath(t *testing.T) {
	capellaHeader := &enginev1.ExecutionPayloadHeaderCapella{
		ParentHash:       fillByteSlice(common.HashLength, 0xaa),
		FeeRecipient:     fillByteSlice(20, 0xbb),
		StateRoot:        fillByteSlice(32, 0xcc),
		ReceiptsRoot:     fillByteSlice(32, 0xdd),
		LogsBloom:        fillByteSlice(256, 0xee),
		PrevRandao:       fillByteSlice(32, 0xff),
		BlockNumber:      555,
		GasLimit:         1111111,
		GasUsed:          222222,
		Timestamp:        3333333333,
		ExtraData:        fillByteSlice(4, 0x12),
		BaseFeePerGas:    fillByteSlice(32, 0x34),
		BlockHash:        fillByteSlice(common.HashLength, 0x56),
		TransactionsRoot: fillByteSlice(32, 0x78),
		WithdrawalsRoot:  fillByteSlice(32, 0x99),
	}

	result, err := ExecutionPayloadHeaderCapellaFromConsensus(capellaHeader)
	require.NoError(t, err)
	require.Equal(t, hexutil.Encode(capellaHeader.ParentHash), result.ParentHash)
	require.DeepEqual(t, hexutil.Encode(capellaHeader.WithdrawalsRoot), result.WithdrawalsRoot)
}

func TestExecutionPayloadHeaderCapella_ToConsensus_HappyPath(t *testing.T) {
	header := &ExecutionPayloadHeaderCapella{
		ParentHash:       hexutil.Encode(fillByteSlice(common.HashLength, 0xaa)),
		FeeRecipient:     hexutil.Encode(fillByteSlice(20, 0xbb)),
		StateRoot:        hexutil.Encode(fillByteSlice(32, 0xcc)),
		ReceiptsRoot:     hexutil.Encode(fillByteSlice(32, 0xdd)),
		LogsBloom:        hexutil.Encode(fillByteSlice(256, 0xee)),
		PrevRandao:       hexutil.Encode(fillByteSlice(32, 0xff)),
		BlockNumber:      "555",
		GasLimit:         "1111111",
		GasUsed:          "222222",
		Timestamp:        "3333333333",
		ExtraData:        "0x1234abcd",
		BaseFeePerGas:    "1234",
		BlockHash:        hexutil.Encode(fillByteSlice(common.HashLength, 0x56)),
		TransactionsRoot: hexutil.Encode(fillByteSlice(32, 0x78)),
		WithdrawalsRoot:  hexutil.Encode(fillByteSlice(32, 0x99)),
	}

	result, err := header.ToConsensus()
	require.NoError(t, err)
	require.DeepEqual(t, hexutil.Encode(result.ParentHash), header.ParentHash)
	require.DeepEqual(t, hexutil.Encode(result.FeeRecipient), header.FeeRecipient)
	require.DeepEqual(t, hexutil.Encode(result.StateRoot), header.StateRoot)
	require.DeepEqual(t, hexutil.Encode(result.ReceiptsRoot), header.ReceiptsRoot)
	require.DeepEqual(t, hexutil.Encode(result.WithdrawalsRoot), header.WithdrawalsRoot)
}

func TestExecutionPayloadHeaderDenebFromConsensus_HappyPath(t *testing.T) {
	denebHeader := &enginev1.ExecutionPayloadHeaderDeneb{
		ParentHash:       fillByteSlice(common.HashLength, 0xaa),
		FeeRecipient:     fillByteSlice(20, 0xbb),
		StateRoot:        fillByteSlice(32, 0xcc),
		ReceiptsRoot:     fillByteSlice(32, 0xdd),
		LogsBloom:        fillByteSlice(256, 0xee),
		PrevRandao:       fillByteSlice(32, 0xff),
		BlockNumber:      999,
		GasLimit:         5000000,
		GasUsed:          2500000,
		Timestamp:        4444444444,
		ExtraData:        fillByteSlice(4, 0x12),
		BaseFeePerGas:    fillByteSlice(32, 0x34),
		BlockHash:        fillByteSlice(common.HashLength, 0x56),
		TransactionsRoot: fillByteSlice(32, 0x78),
		WithdrawalsRoot:  fillByteSlice(32, 0x99),
		BlobGasUsed:      1234,
		ExcessBlobGas:    5678,
	}

	result, err := ExecutionPayloadHeaderDenebFromConsensus(denebHeader)
	require.NoError(t, err)
	require.Equal(t, hexutil.Encode(denebHeader.ParentHash), result.ParentHash)
	require.DeepEqual(t, hexutil.Encode(denebHeader.FeeRecipient), result.FeeRecipient)
	require.DeepEqual(t, hexutil.Encode(denebHeader.StateRoot), result.StateRoot)
	require.DeepEqual(t, fmt.Sprintf("%d", denebHeader.BlobGasUsed), result.BlobGasUsed)
}

func TestExecutionPayloadHeaderDeneb_ToConsensus_HappyPath(t *testing.T) {
	header := &ExecutionPayloadHeaderDeneb{
		ParentHash:       hexutil.Encode(fillByteSlice(common.HashLength, 0xaa)),
		FeeRecipient:     hexutil.Encode(fillByteSlice(20, 0xbb)),
		StateRoot:        hexutil.Encode(fillByteSlice(32, 0xcc)),
		ReceiptsRoot:     hexutil.Encode(fillByteSlice(32, 0xdd)),
		LogsBloom:        hexutil.Encode(fillByteSlice(256, 0xee)),
		PrevRandao:       hexutil.Encode(fillByteSlice(32, 0xff)),
		BlockNumber:      "999",
		GasLimit:         "5000000",
		GasUsed:          "2500000",
		Timestamp:        "4444444444",
		ExtraData:        "0x1234abcd",
		BaseFeePerGas:    "1234",
		BlockHash:        hexutil.Encode(fillByteSlice(common.HashLength, 0x56)),
		TransactionsRoot: hexutil.Encode(fillByteSlice(32, 0x78)),
		WithdrawalsRoot:  hexutil.Encode(fillByteSlice(32, 0x99)),
		BlobGasUsed:      "1234",
		ExcessBlobGas:    "5678",
	}

	result, err := header.ToConsensus()
	require.NoError(t, err)
	require.DeepEqual(t, hexutil.Encode(result.ParentHash), header.ParentHash)
	require.DeepEqual(t, result.BlobGasUsed, uint64(1234))
	require.DeepEqual(t, result.ExcessBlobGas, uint64(5678))
	require.DeepEqual(t, result.BlockNumber, uint64(999))
}

func TestWithdrawalRequestsFromConsensus_HappyPath(t *testing.T) {
	consensusRequests := []*enginev1.WithdrawalRequest{
		{
			SourceAddress:   fillByteSlice(20, 0xbb),
			ValidatorPubkey: fillByteSlice(48, 0xbb),
			Amount:          12345,
		},
		{
			SourceAddress:   fillByteSlice(20, 0xcc),
			ValidatorPubkey: fillByteSlice(48, 0xcc),
			Amount:          54321,
		},
	}

	result := WithdrawalRequestsFromConsensus(consensusRequests)
	require.DeepEqual(t, len(result), len(consensusRequests))
	require.DeepEqual(t, result[0].Amount, fmt.Sprintf("%d", consensusRequests[0].Amount))
}

func TestWithdrawalRequestFromConsensus_HappyPath(t *testing.T) {
	req := &enginev1.WithdrawalRequest{
		SourceAddress:   fillByteSlice(20, 0xbb),
		ValidatorPubkey: fillByteSlice(48, 0xbb),
		Amount:          42,
	}
	result := WithdrawalRequestFromConsensus(req)
	require.NotNil(t, result)
	require.DeepEqual(t, result.SourceAddress, hexutil.Encode(fillByteSlice(20, 0xbb)))
}

func TestWithdrawalRequest_ToConsensus_HappyPath(t *testing.T) {
	withdrawalReq := &WithdrawalRequest{
		SourceAddress:   hexutil.Encode(fillByteSlice(20, 111)),
		ValidatorPubkey: hexutil.Encode(fillByteSlice(48, 123)),
		Amount:          "12345",
	}
	result, err := withdrawalReq.ToConsensus()
	require.NoError(t, err)
	require.DeepEqual(t, result.Amount, uint64(12345))
}

func TestConsolidationRequestsFromConsensus_HappyPath(t *testing.T) {
	consensusRequests := []*enginev1.ConsolidationRequest{
		{
			SourceAddress: fillByteSlice(20, 111),
			SourcePubkey:  fillByteSlice(48, 112),
			TargetPubkey:  fillByteSlice(48, 113),
		},
	}
	result := ConsolidationRequestsFromConsensus(consensusRequests)
	require.DeepEqual(t, len(result), len(consensusRequests))
	require.DeepEqual(t, result[0].SourceAddress, "0x6f6f6f6f6f6f6f6f6f6f6f6f6f6f6f6f6f6f6f6f")
}

func TestDepositRequestsFromConsensus_HappyPath(t *testing.T) {
	ds := []*enginev1.DepositRequest{
		{
			Pubkey:                fillByteSlice(48, 0xbb),
			WithdrawalCredentials: fillByteSlice(32, 0xdd),
			Amount:                98765,
			Signature:             fillByteSlice(96, 0xff),
			Index:                 111,
		},
	}
	result := DepositRequestsFromConsensus(ds)
	require.DeepEqual(t, len(result), len(ds))
	require.DeepEqual(t, result[0].Amount, "98765")
}

func TestDepositRequest_ToConsensus_HappyPath(t *testing.T) {
	req := &DepositRequest{
		Pubkey:                hexutil.Encode(fillByteSlice(48, 0xbb)),
		WithdrawalCredentials: hexutil.Encode(fillByteSlice(32, 0xaa)),
		Amount:                "123",
		Signature:             hexutil.Encode(fillByteSlice(96, 0xdd)),
		Index:                 "456",
	}

	result, err := req.ToConsensus()
	require.NoError(t, err)
	require.DeepEqual(t, result.Amount, uint64(123))
	require.DeepEqual(t, result.Signature, fillByteSlice(96, 0xdd))
}

func TestExecutionRequestsFromConsensus_HappyPath(t *testing.T) {
	er := &enginev1.ExecutionRequests{
		Deposits: []*enginev1.DepositRequest{
			{
				Pubkey:                fillByteSlice(48, 0xba),
				WithdrawalCredentials: fillByteSlice(32, 0xaa),
				Amount:                33,
				Signature:             fillByteSlice(96, 0xff),
				Index:                 44,
			},
		},
		Withdrawals: []*enginev1.WithdrawalRequest{
			{
				SourceAddress:   fillByteSlice(20, 0xaa),
				ValidatorPubkey: fillByteSlice(48, 0xba),
				Amount:          555,
			},
		},
		Consolidations: []*enginev1.ConsolidationRequest{
			{
				SourceAddress: fillByteSlice(20, 0xdd),
				SourcePubkey:  fillByteSlice(48, 0xdd),
				TargetPubkey:  fillByteSlice(48, 0xcc),
			},
		},
	}

	result := ExecutionRequestsFromConsensus(er)
	require.NotNil(t, result)
	require.Equal(t, 1, len(result.Deposits))
	require.Equal(t, "33", result.Deposits[0].Amount)
	require.Equal(t, 1, len(result.Withdrawals))
	require.Equal(t, "555", result.Withdrawals[0].Amount)
	require.Equal(t, 1, len(result.Consolidations))
	require.Equal(t, "0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", result.Consolidations[0].TargetPubkey)
}

func TestExecutionRequests_ToConsensus_HappyPath(t *testing.T) {
	execReq := &ExecutionRequests{
		Deposits: []*DepositRequest{
			{
				Pubkey:                hexutil.Encode(fillByteSlice(48, 0xbb)),
				WithdrawalCredentials: hexutil.Encode(fillByteSlice(32, 0xaa)),
				Amount:                "33",
				Signature:             hexutil.Encode(fillByteSlice(96, 0xff)),
				Index:                 "44",
			},
		},
		Withdrawals: []*WithdrawalRequest{
			{
				SourceAddress:   hexutil.Encode(fillByteSlice(20, 0xdd)),
				ValidatorPubkey: hexutil.Encode(fillByteSlice(48, 0xbb)),
				Amount:          "555",
			},
		},
		Consolidations: []*ConsolidationRequest{
			{
				SourceAddress: hexutil.Encode(fillByteSlice(20, 0xcc)),
				SourcePubkey:  hexutil.Encode(fillByteSlice(48, 0xbb)),
				TargetPubkey:  hexutil.Encode(fillByteSlice(48, 0xcc)),
			},
		},
	}

	result, err := execReq.ToConsensus()
	require.NoError(t, err)

	require.Equal(t, 1, len(result.Deposits))
	require.Equal(t, uint64(33), result.Deposits[0].Amount)
	require.Equal(t, 1, len(result.Withdrawals))
	require.Equal(t, uint64(555), result.Withdrawals[0].Amount)
	require.Equal(t, 1, len(result.Consolidations))
	require.DeepEqual(t, fillByteSlice(48, 0xcc), result.Consolidations[0].TargetPubkey)
}
