package structs

import (
	"fmt"
	"strconv"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/api/server"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/container/slice"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
)

// ----------------------------------------------------------------------------
// Bellatrix
// ----------------------------------------------------------------------------

func ExecutionPayloadFromConsensus(payload *enginev1.ExecutionPayload) (*ExecutionPayload, error) {
	baseFeePerGas, err := sszBytesToUint256String(payload.BaseFeePerGas)
	if err != nil {
		return nil, err
	}
	transactions := make([]string, len(payload.Transactions))
	for i, tx := range payload.Transactions {
		transactions[i] = hexutil.Encode(tx)
	}

	return &ExecutionPayload{
		ParentHash:    hexutil.Encode(payload.ParentHash),
		FeeRecipient:  hexutil.Encode(payload.FeeRecipient),
		StateRoot:     hexutil.Encode(payload.StateRoot),
		ReceiptsRoot:  hexutil.Encode(payload.ReceiptsRoot),
		LogsBloom:     hexutil.Encode(payload.LogsBloom),
		PrevRandao:    hexutil.Encode(payload.PrevRandao),
		BlockNumber:   fmt.Sprintf("%d", payload.BlockNumber),
		GasLimit:      fmt.Sprintf("%d", payload.GasLimit),
		GasUsed:       fmt.Sprintf("%d", payload.GasUsed),
		Timestamp:     fmt.Sprintf("%d", payload.Timestamp),
		ExtraData:     hexutil.Encode(payload.ExtraData),
		BaseFeePerGas: baseFeePerGas,
		BlockHash:     hexutil.Encode(payload.BlockHash),
		Transactions:  transactions,
	}, nil
}

func ExecutionPayloadHeaderFromConsensus(payload *enginev1.ExecutionPayloadHeader) (*ExecutionPayloadHeader, error) {
	baseFeePerGas, err := sszBytesToUint256String(payload.BaseFeePerGas)
	if err != nil {
		return nil, err
	}

	return &ExecutionPayloadHeader{
		ParentHash:       hexutil.Encode(payload.ParentHash),
		FeeRecipient:     hexutil.Encode(payload.FeeRecipient),
		StateRoot:        hexutil.Encode(payload.StateRoot),
		ReceiptsRoot:     hexutil.Encode(payload.ReceiptsRoot),
		LogsBloom:        hexutil.Encode(payload.LogsBloom),
		PrevRandao:       hexutil.Encode(payload.PrevRandao),
		BlockNumber:      fmt.Sprintf("%d", payload.BlockNumber),
		GasLimit:         fmt.Sprintf("%d", payload.GasLimit),
		GasUsed:          fmt.Sprintf("%d", payload.GasUsed),
		Timestamp:        fmt.Sprintf("%d", payload.Timestamp),
		ExtraData:        hexutil.Encode(payload.ExtraData),
		BaseFeePerGas:    baseFeePerGas,
		BlockHash:        hexutil.Encode(payload.BlockHash),
		TransactionsRoot: hexutil.Encode(payload.TransactionsRoot),
	}, nil
}

// ----------------------------------------------------------------------------
// Capella
// ----------------------------------------------------------------------------

func ExecutionPayloadCapellaFromConsensus(payload *enginev1.ExecutionPayloadCapella) (*ExecutionPayloadCapella, error) {
	baseFeePerGas, err := sszBytesToUint256String(payload.BaseFeePerGas)
	if err != nil {
		return nil, err
	}
	transactions := make([]string, len(payload.Transactions))
	for i, tx := range payload.Transactions {
		transactions[i] = hexutil.Encode(tx)
	}

	return &ExecutionPayloadCapella{
		ParentHash:    hexutil.Encode(payload.ParentHash),
		FeeRecipient:  hexutil.Encode(payload.FeeRecipient),
		StateRoot:     hexutil.Encode(payload.StateRoot),
		ReceiptsRoot:  hexutil.Encode(payload.ReceiptsRoot),
		LogsBloom:     hexutil.Encode(payload.LogsBloom),
		PrevRandao:    hexutil.Encode(payload.PrevRandao),
		BlockNumber:   fmt.Sprintf("%d", payload.BlockNumber),
		GasLimit:      fmt.Sprintf("%d", payload.GasLimit),
		GasUsed:       fmt.Sprintf("%d", payload.GasUsed),
		Timestamp:     fmt.Sprintf("%d", payload.Timestamp),
		ExtraData:     hexutil.Encode(payload.ExtraData),
		BaseFeePerGas: baseFeePerGas,
		BlockHash:     hexutil.Encode(payload.BlockHash),
		Transactions:  transactions,
		Withdrawals:   WithdrawalsFromConsensus(payload.Withdrawals),
	}, nil
}

func ExecutionPayloadHeaderCapellaFromConsensus(payload *enginev1.ExecutionPayloadHeaderCapella) (*ExecutionPayloadHeaderCapella, error) {
	baseFeePerGas, err := sszBytesToUint256String(payload.BaseFeePerGas)
	if err != nil {
		return nil, err
	}

	return &ExecutionPayloadHeaderCapella{
		ParentHash:       hexutil.Encode(payload.ParentHash),
		FeeRecipient:     hexutil.Encode(payload.FeeRecipient),
		StateRoot:        hexutil.Encode(payload.StateRoot),
		ReceiptsRoot:     hexutil.Encode(payload.ReceiptsRoot),
		LogsBloom:        hexutil.Encode(payload.LogsBloom),
		PrevRandao:       hexutil.Encode(payload.PrevRandao),
		BlockNumber:      fmt.Sprintf("%d", payload.BlockNumber),
		GasLimit:         fmt.Sprintf("%d", payload.GasLimit),
		GasUsed:          fmt.Sprintf("%d", payload.GasUsed),
		Timestamp:        fmt.Sprintf("%d", payload.Timestamp),
		ExtraData:        hexutil.Encode(payload.ExtraData),
		BaseFeePerGas:    baseFeePerGas,
		BlockHash:        hexutil.Encode(payload.BlockHash),
		TransactionsRoot: hexutil.Encode(payload.TransactionsRoot),
		WithdrawalsRoot:  hexutil.Encode(payload.WithdrawalsRoot),
	}, nil
}

// ----------------------------------------------------------------------------
// Deneb
// ----------------------------------------------------------------------------

func ExecutionPayloadDenebFromConsensus(payload *enginev1.ExecutionPayloadDeneb) (*ExecutionPayloadDeneb, error) {
	baseFeePerGas, err := sszBytesToUint256String(payload.BaseFeePerGas)
	if err != nil {
		return nil, err
	}
	transactions := make([]string, len(payload.Transactions))
	for i, tx := range payload.Transactions {
		transactions[i] = hexutil.Encode(tx)
	}

	return &ExecutionPayloadDeneb{
		ParentHash:    hexutil.Encode(payload.ParentHash),
		FeeRecipient:  hexutil.Encode(payload.FeeRecipient),
		StateRoot:     hexutil.Encode(payload.StateRoot),
		ReceiptsRoot:  hexutil.Encode(payload.ReceiptsRoot),
		LogsBloom:     hexutil.Encode(payload.LogsBloom),
		PrevRandao:    hexutil.Encode(payload.PrevRandao),
		BlockNumber:   fmt.Sprintf("%d", payload.BlockNumber),
		GasLimit:      fmt.Sprintf("%d", payload.GasLimit),
		GasUsed:       fmt.Sprintf("%d", payload.GasUsed),
		Timestamp:     fmt.Sprintf("%d", payload.Timestamp),
		ExtraData:     hexutil.Encode(payload.ExtraData),
		BaseFeePerGas: baseFeePerGas,
		BlockHash:     hexutil.Encode(payload.BlockHash),
		Transactions:  transactions,
		Withdrawals:   WithdrawalsFromConsensus(payload.Withdrawals),
		BlobGasUsed:   fmt.Sprintf("%d", payload.BlobGasUsed),
		ExcessBlobGas: fmt.Sprintf("%d", payload.ExcessBlobGas),
	}, nil
}

func (p *ExecutionPayloadDeneb) ToConsensus() (*enginev1.ExecutionPayloadDeneb, error) {
	if p == nil {
		return nil, errors.New("nil execution payload deneb")
	}
	payloadParentHash, err := bytesutil.DecodeHexWithLength(p.ParentHash, common.HashLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.ParentHash")
	}
	payloadFeeRecipient, err := bytesutil.DecodeHexWithLength(p.FeeRecipient, fieldparams.FeeRecipientLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.FeeRecipient")
	}
	payloadStateRoot, err := bytesutil.DecodeHexWithLength(p.StateRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.StateRoot")
	}
	payloadReceiptsRoot, err := bytesutil.DecodeHexWithLength(p.ReceiptsRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.ReceiptsRoot")
	}
	payloadLogsBloom, err := bytesutil.DecodeHexWithLength(p.LogsBloom, fieldparams.LogsBloomLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.LogsBloom")
	}
	payloadPrevRandao, err := bytesutil.DecodeHexWithLength(p.PrevRandao, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.PrevRandao")
	}
	payloadBlockNumber, err := strconv.ParseUint(p.BlockNumber, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.BlockNumber")
	}
	payloadGasLimit, err := strconv.ParseUint(p.GasLimit, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.GasLimit")
	}
	payloadGasUsed, err := strconv.ParseUint(p.GasUsed, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.GasUsed")
	}
	payloadTimestamp, err := strconv.ParseUint(p.Timestamp, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayloadHeader.Timestamp")
	}
	payloadExtraData, err := bytesutil.DecodeHexWithMaxLength(p.ExtraData, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.ExtraData")
	}
	payloadBaseFeePerGas, err := bytesutil.Uint256ToSSZBytes(p.BaseFeePerGas)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.BaseFeePerGas")
	}
	payloadBlockHash, err := bytesutil.DecodeHexWithLength(p.BlockHash, common.HashLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.BlockHash")
	}
	err = slice.VerifyMaxLength(p.Transactions, fieldparams.MaxTxsPerPayloadLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.Transactions")
	}
	txs := make([][]byte, len(p.Transactions))
	for i, tx := range p.Transactions {
		txs[i], err = bytesutil.DecodeHexWithMaxLength(tx, fieldparams.MaxBytesPerTxLength)
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("Body.ExecutionPayload.Transactions[%d]", i))
		}
	}
	err = slice.VerifyMaxLength(p.Withdrawals, fieldparams.MaxWithdrawalsPerPayload)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.Withdrawals")
	}
	withdrawals := make([]*enginev1.Withdrawal, len(p.Withdrawals))
	for i, w := range p.Withdrawals {
		withdrawalIndex, err := strconv.ParseUint(w.WithdrawalIndex, 10, 64)
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("ExecutionPayload.Withdrawals[%d].WithdrawalIndex", i))
		}
		validatorIndex, err := strconv.ParseUint(w.ValidatorIndex, 10, 64)
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("ExecutionPayload.Withdrawals[%d].ValidatorIndex", i))
		}
		address, err := bytesutil.DecodeHexWithLength(w.ExecutionAddress, common.AddressLength)
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("ExecutionPayload.Withdrawals[%d].ExecutionAddress", i))
		}
		amount, err := strconv.ParseUint(w.Amount, 10, 64)
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("Body.ExecutionPayload.Withdrawals[%d].Amount", i))
		}
		withdrawals[i] = &enginev1.Withdrawal{
			Index:          withdrawalIndex,
			ValidatorIndex: primitives.ValidatorIndex(validatorIndex),
			Address:        address,
			Amount:         amount,
		}
	}

	payloadBlobGasUsed, err := strconv.ParseUint(p.BlobGasUsed, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.BlobGasUsed")
	}
	payloadExcessBlobGas, err := strconv.ParseUint(p.ExcessBlobGas, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.ExcessBlobGas")
	}
	return &enginev1.ExecutionPayloadDeneb{
		ParentHash:    payloadParentHash,
		FeeRecipient:  payloadFeeRecipient,
		StateRoot:     payloadStateRoot,
		ReceiptsRoot:  payloadReceiptsRoot,
		LogsBloom:     payloadLogsBloom,
		PrevRandao:    payloadPrevRandao,
		BlockNumber:   payloadBlockNumber,
		GasLimit:      payloadGasLimit,
		GasUsed:       payloadGasUsed,
		Timestamp:     payloadTimestamp,
		ExtraData:     payloadExtraData,
		BaseFeePerGas: payloadBaseFeePerGas,
		BlockHash:     payloadBlockHash,
		Transactions:  txs,
		Withdrawals:   withdrawals,
		BlobGasUsed:   payloadBlobGasUsed,
		ExcessBlobGas: payloadExcessBlobGas,
	}, nil
}

func ExecutionPayloadHeaderDenebFromConsensus(payload *enginev1.ExecutionPayloadHeaderDeneb) (*ExecutionPayloadHeaderDeneb, error) {
	baseFeePerGas, err := sszBytesToUint256String(payload.BaseFeePerGas)
	if err != nil {
		return nil, err
	}

	return &ExecutionPayloadHeaderDeneb{
		ParentHash:       hexutil.Encode(payload.ParentHash),
		FeeRecipient:     hexutil.Encode(payload.FeeRecipient),
		StateRoot:        hexutil.Encode(payload.StateRoot),
		ReceiptsRoot:     hexutil.Encode(payload.ReceiptsRoot),
		LogsBloom:        hexutil.Encode(payload.LogsBloom),
		PrevRandao:       hexutil.Encode(payload.PrevRandao),
		BlockNumber:      fmt.Sprintf("%d", payload.BlockNumber),
		GasLimit:         fmt.Sprintf("%d", payload.GasLimit),
		GasUsed:          fmt.Sprintf("%d", payload.GasUsed),
		Timestamp:        fmt.Sprintf("%d", payload.Timestamp),
		ExtraData:        hexutil.Encode(payload.ExtraData),
		BaseFeePerGas:    baseFeePerGas,
		BlockHash:        hexutil.Encode(payload.BlockHash),
		TransactionsRoot: hexutil.Encode(payload.TransactionsRoot),
		WithdrawalsRoot:  hexutil.Encode(payload.WithdrawalsRoot),
		BlobGasUsed:      fmt.Sprintf("%d", payload.BlobGasUsed),
		ExcessBlobGas:    fmt.Sprintf("%d", payload.ExcessBlobGas),
	}, nil
}

// ----------------------------------------------------------------------------
// Electra
// ----------------------------------------------------------------------------

var (
	ExecutionPayloadElectraFromConsensus       = ExecutionPayloadDenebFromConsensus
	ExecutionPayloadHeaderElectraFromConsensus = ExecutionPayloadHeaderDenebFromConsensus
)

// ----------------------------------------------------------------------------
// Fulu
// ----------------------------------------------------------------------------

var (
	ExecutionPayloadFuluFromConsensus       = ExecutionPayloadDenebFromConsensus
	ExecutionPayloadHeaderFuluFromConsensus = ExecutionPayloadHeaderDenebFromConsensus
	BeaconBlockFuluFromConsensus            = BeaconBlockElectraFromConsensus
)
