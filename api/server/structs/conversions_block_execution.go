package structs

import (
	"fmt"
	"strconv"

	"github.com/OffchainLabs/prysm/v6/api/server"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	consensusblocks "github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/container/slice"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v6/proto/engine/v1"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
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

func (e *ExecutionPayload) ToConsensus() (*enginev1.ExecutionPayload, error) {
	if e == nil {
		return nil, server.NewDecodeError(errNilValue, "ExecutionPayload")
	}
	payloadParentHash, err := bytesutil.DecodeHexWithLength(e.ParentHash, common.HashLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.ParentHash")
	}
	payloadFeeRecipient, err := bytesutil.DecodeHexWithLength(e.FeeRecipient, fieldparams.FeeRecipientLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.FeeRecipient")
	}
	payloadStateRoot, err := bytesutil.DecodeHexWithLength(e.StateRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.StateRoot")
	}
	payloadReceiptsRoot, err := bytesutil.DecodeHexWithLength(e.ReceiptsRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.ReceiptsRoot")
	}
	payloadLogsBloom, err := bytesutil.DecodeHexWithLength(e.LogsBloom, fieldparams.LogsBloomLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.LogsBloom")
	}
	payloadPrevRandao, err := bytesutil.DecodeHexWithLength(e.PrevRandao, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.PrevRandao")
	}
	payloadBlockNumber, err := strconv.ParseUint(e.BlockNumber, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.BlockNumber")
	}
	payloadGasLimit, err := strconv.ParseUint(e.GasLimit, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.GasLimit")
	}
	payloadGasUsed, err := strconv.ParseUint(e.GasUsed, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.GasUsed")
	}
	payloadTimestamp, err := strconv.ParseUint(e.Timestamp, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.Timestamp")
	}
	payloadExtraData, err := bytesutil.DecodeHexWithMaxLength(e.ExtraData, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.ExtraData")
	}
	payloadBaseFeePerGas, err := bytesutil.Uint256ToSSZBytes(e.BaseFeePerGas)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.BaseFeePerGas")
	}
	payloadBlockHash, err := bytesutil.DecodeHexWithLength(e.BlockHash, common.HashLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.BlockHash")
	}
	err = slice.VerifyMaxLength(e.Transactions, fieldparams.MaxTxsPerPayloadLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.Transactions")
	}
	payloadTxs := make([][]byte, len(e.Transactions))
	for i, tx := range e.Transactions {
		payloadTxs[i], err = bytesutil.DecodeHexWithMaxLength(tx, fieldparams.MaxBytesPerTxLength)
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("ExecutionPayload.Transactions[%d]", i))
		}
	}
	return &enginev1.ExecutionPayload{
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
		Transactions:  payloadTxs,
	}, nil
}

func (r *ExecutionPayload) PayloadProto() (proto.Message, error) {
	if r == nil {
		return nil, errors.Wrap(consensusblocks.ErrNilObject, "nil execution payload")
	}
	return r.ToConsensus()
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

func (e *ExecutionPayloadHeader) ToConsensus() (*enginev1.ExecutionPayloadHeader, error) {
	if e == nil {
		return nil, server.NewDecodeError(errNilValue, "ExecutionPayloadHeader")
	}
	payloadParentHash, err := bytesutil.DecodeHexWithLength(e.ParentHash, common.HashLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayloadHeader.ParentHash")
	}
	payloadFeeRecipient, err := bytesutil.DecodeHexWithLength(e.FeeRecipient, fieldparams.FeeRecipientLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayloadHeader.FeeRecipient")
	}
	payloadStateRoot, err := bytesutil.DecodeHexWithLength(e.StateRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayloadHeader.StateRoot")
	}
	payloadReceiptsRoot, err := bytesutil.DecodeHexWithLength(e.ReceiptsRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayloadHeader.ReceiptsRoot")
	}
	payloadLogsBloom, err := bytesutil.DecodeHexWithLength(e.LogsBloom, fieldparams.LogsBloomLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayloadHeader.LogsBloom")
	}
	payloadPrevRandao, err := bytesutil.DecodeHexWithLength(e.PrevRandao, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayloadHeader.PrevRandao")
	}
	payloadBlockNumber, err := strconv.ParseUint(e.BlockNumber, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayloadHeader.BlockNumber")
	}
	payloadGasLimit, err := strconv.ParseUint(e.GasLimit, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayloadHeader.GasLimit")
	}
	payloadGasUsed, err := strconv.ParseUint(e.GasUsed, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayloadHeader.GasUsed")
	}
	payloadTimestamp, err := strconv.ParseUint(e.Timestamp, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayloadHeader.Timestamp")
	}
	payloadExtraData, err := bytesutil.DecodeHexWithMaxLength(e.ExtraData, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayloadHeader.ExtraData")
	}
	payloadBaseFeePerGas, err := bytesutil.Uint256ToSSZBytes(e.BaseFeePerGas)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayloadHeader.BaseFeePerGas")
	}
	payloadBlockHash, err := bytesutil.DecodeHexWithLength(e.BlockHash, common.HashLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayloadHeader.BlockHash")
	}
	payloadTxsRoot, err := bytesutil.DecodeHexWithLength(e.TransactionsRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayloadHeader.TransactionsRoot")
	}

	return &enginev1.ExecutionPayloadHeader{
		ParentHash:       payloadParentHash,
		FeeRecipient:     payloadFeeRecipient,
		StateRoot:        payloadStateRoot,
		ReceiptsRoot:     payloadReceiptsRoot,
		LogsBloom:        payloadLogsBloom,
		PrevRandao:       payloadPrevRandao,
		BlockNumber:      payloadBlockNumber,
		GasLimit:         payloadGasLimit,
		GasUsed:          payloadGasUsed,
		Timestamp:        payloadTimestamp,
		ExtraData:        payloadExtraData,
		BaseFeePerGas:    payloadBaseFeePerGas,
		BlockHash:        payloadBlockHash,
		TransactionsRoot: payloadTxsRoot,
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

func (e *ExecutionPayloadCapella) ToConsensus() (*enginev1.ExecutionPayloadCapella, error) {
	if e == nil {
		return nil, server.NewDecodeError(errNilValue, "ExecutionPayload")
	}
	payloadParentHash, err := bytesutil.DecodeHexWithLength(e.ParentHash, common.HashLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.ParentHash")
	}
	payloadFeeRecipient, err := bytesutil.DecodeHexWithLength(e.FeeRecipient, fieldparams.FeeRecipientLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.FeeRecipient")
	}
	payloadStateRoot, err := bytesutil.DecodeHexWithLength(e.StateRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.StateRoot")
	}
	payloadReceiptsRoot, err := bytesutil.DecodeHexWithLength(e.ReceiptsRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.ReceiptsRoot")
	}
	payloadLogsBloom, err := bytesutil.DecodeHexWithLength(e.LogsBloom, fieldparams.LogsBloomLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.LogsBloom")
	}
	payloadPrevRandao, err := bytesutil.DecodeHexWithLength(e.PrevRandao, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.PrevRandao")
	}
	payloadBlockNumber, err := strconv.ParseUint(e.BlockNumber, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.BlockNumber")
	}
	payloadGasLimit, err := strconv.ParseUint(e.GasLimit, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.GasLimit")
	}
	payloadGasUsed, err := strconv.ParseUint(e.GasUsed, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.GasUsed")
	}
	payloadTimestamp, err := strconv.ParseUint(e.Timestamp, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.Timestamp")
	}
	payloadExtraData, err := bytesutil.DecodeHexWithMaxLength(e.ExtraData, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.ExtraData")
	}
	payloadBaseFeePerGas, err := bytesutil.Uint256ToSSZBytes(e.BaseFeePerGas)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.BaseFeePerGas")
	}
	payloadBlockHash, err := bytesutil.DecodeHexWithLength(e.BlockHash, common.HashLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.BlockHash")
	}
	err = slice.VerifyMaxLength(e.Transactions, fieldparams.MaxTxsPerPayloadLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.Transactions")
	}
	payloadTxs := make([][]byte, len(e.Transactions))
	for i, tx := range e.Transactions {
		payloadTxs[i], err = bytesutil.DecodeHexWithMaxLength(tx, fieldparams.MaxBytesPerTxLength)
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("ExecutionPayload.Transactions[%d]", i))
		}
	}
	err = slice.VerifyMaxLength(e.Withdrawals, fieldparams.MaxWithdrawalsPerPayload)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.Withdrawals")
	}
	withdrawals := make([]*enginev1.Withdrawal, len(e.Withdrawals))
	for i, w := range e.Withdrawals {
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
			return nil, server.NewDecodeError(err, fmt.Sprintf("ExecutionPayload.Withdrawals[%d].Amount", i))
		}
		withdrawals[i] = &enginev1.Withdrawal{
			Index:          withdrawalIndex,
			ValidatorIndex: primitives.ValidatorIndex(validatorIndex),
			Address:        address,
			Amount:         amount,
		}
	}
	return &enginev1.ExecutionPayloadCapella{
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
		Transactions:  payloadTxs,
		Withdrawals:   withdrawals,
	}, nil
}

func (p *ExecutionPayloadCapella) PayloadProto() (proto.Message, error) {
	if p == nil {
		return nil, errors.Wrap(consensusblocks.ErrNilObject, "nil capella execution payload")
	}
	return p.ToConsensus()
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

func (e *ExecutionPayloadHeaderCapella) ToConsensus() (*enginev1.ExecutionPayloadHeaderCapella, error) {
	if e == nil {
		return nil, server.NewDecodeError(errNilValue, "ExecutionPayloadHeader")
	}
	payloadParentHash, err := bytesutil.DecodeHexWithLength(e.ParentHash, common.HashLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayloadHeader.ParentHash")
	}
	payloadFeeRecipient, err := bytesutil.DecodeHexWithLength(e.FeeRecipient, fieldparams.FeeRecipientLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayloadHeader.FeeRecipient")
	}
	payloadStateRoot, err := bytesutil.DecodeHexWithLength(e.StateRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayloadHeader.StateRoot")
	}
	payloadReceiptsRoot, err := bytesutil.DecodeHexWithLength(e.ReceiptsRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayloadHeader.ReceiptsRoot")
	}
	payloadLogsBloom, err := bytesutil.DecodeHexWithLength(e.LogsBloom, fieldparams.LogsBloomLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayloadHeader.LogsBloom")
	}
	payloadPrevRandao, err := bytesutil.DecodeHexWithLength(e.PrevRandao, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayloadHeader.PrevRandao")
	}
	payloadBlockNumber, err := strconv.ParseUint(e.BlockNumber, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayloadHeader.BlockNumber")
	}
	payloadGasLimit, err := strconv.ParseUint(e.GasLimit, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayloadHeader.GasLimit")
	}
	payloadGasUsed, err := strconv.ParseUint(e.GasUsed, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayloadHeader.GasUsed")
	}
	payloadTimestamp, err := strconv.ParseUint(e.Timestamp, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayloadHeader.Timestamp")
	}
	payloadExtraData, err := bytesutil.DecodeHexWithMaxLength(e.ExtraData, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayloadHeader.ExtraData")
	}
	payloadBaseFeePerGas, err := bytesutil.Uint256ToSSZBytes(e.BaseFeePerGas)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayloadHeader.BaseFeePerGas")
	}
	payloadBlockHash, err := bytesutil.DecodeHexWithMaxLength(e.BlockHash, common.HashLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayloadHeader.BlockHash")
	}
	payloadTxsRoot, err := bytesutil.DecodeHexWithMaxLength(e.TransactionsRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayloadHeader.TransactionsRoot")
	}
	payloadWithdrawalsRoot, err := bytesutil.DecodeHexWithMaxLength(e.WithdrawalsRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayloadHeader.WithdrawalsRoot")
	}
	return &enginev1.ExecutionPayloadHeaderCapella{
		ParentHash:       payloadParentHash,
		FeeRecipient:     payloadFeeRecipient,
		StateRoot:        payloadStateRoot,
		ReceiptsRoot:     payloadReceiptsRoot,
		LogsBloom:        payloadLogsBloom,
		PrevRandao:       payloadPrevRandao,
		BlockNumber:      payloadBlockNumber,
		GasLimit:         payloadGasLimit,
		GasUsed:          payloadGasUsed,
		Timestamp:        payloadTimestamp,
		ExtraData:        payloadExtraData,
		BaseFeePerGas:    payloadBaseFeePerGas,
		BlockHash:        payloadBlockHash,
		TransactionsRoot: payloadTxsRoot,
		WithdrawalsRoot:  payloadWithdrawalsRoot,
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

func (e *ExecutionPayloadDeneb) ToConsensus() (*enginev1.ExecutionPayloadDeneb, error) {
	if e == nil {
		return nil, server.NewDecodeError(errNilValue, "ExecutionPayload")
	}
	payloadParentHash, err := bytesutil.DecodeHexWithLength(e.ParentHash, common.HashLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.ParentHash")
	}
	payloadFeeRecipient, err := bytesutil.DecodeHexWithLength(e.FeeRecipient, fieldparams.FeeRecipientLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.FeeRecipient")
	}
	payloadStateRoot, err := bytesutil.DecodeHexWithLength(e.StateRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.StateRoot")
	}
	payloadReceiptsRoot, err := bytesutil.DecodeHexWithLength(e.ReceiptsRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.ReceiptsRoot")
	}
	payloadLogsBloom, err := bytesutil.DecodeHexWithLength(e.LogsBloom, fieldparams.LogsBloomLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.LogsBloom")
	}
	payloadPrevRandao, err := bytesutil.DecodeHexWithLength(e.PrevRandao, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.PrevRandao")
	}
	payloadBlockNumber, err := strconv.ParseUint(e.BlockNumber, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.BlockNumber")
	}
	payloadGasLimit, err := strconv.ParseUint(e.GasLimit, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.GasLimit")
	}
	payloadGasUsed, err := strconv.ParseUint(e.GasUsed, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.GasUsed")
	}
	payloadTimestamp, err := strconv.ParseUint(e.Timestamp, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayloadHeader.Timestamp")
	}
	payloadExtraData, err := bytesutil.DecodeHexWithMaxLength(e.ExtraData, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.ExtraData")
	}
	payloadBaseFeePerGas, err := bytesutil.Uint256ToSSZBytes(e.BaseFeePerGas)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.BaseFeePerGas")
	}
	payloadBlockHash, err := bytesutil.DecodeHexWithLength(e.BlockHash, common.HashLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.BlockHash")
	}
	err = slice.VerifyMaxLength(e.Transactions, fieldparams.MaxTxsPerPayloadLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.Transactions")
	}
	txs := make([][]byte, len(e.Transactions))
	for i, tx := range e.Transactions {
		txs[i], err = bytesutil.DecodeHexWithMaxLength(tx, fieldparams.MaxBytesPerTxLength)
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("ExecutionPayload.Transactions[%d]", i))
		}
	}
	err = slice.VerifyMaxLength(e.Withdrawals, fieldparams.MaxWithdrawalsPerPayload)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.Withdrawals")
	}
	withdrawals := make([]*enginev1.Withdrawal, len(e.Withdrawals))
	for i, w := range e.Withdrawals {
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
			return nil, server.NewDecodeError(err, fmt.Sprintf("ExecutionPayload.Withdrawals[%d].Amount", i))
		}
		withdrawals[i] = &enginev1.Withdrawal{
			Index:          withdrawalIndex,
			ValidatorIndex: primitives.ValidatorIndex(validatorIndex),
			Address:        address,
			Amount:         amount,
		}
	}

	payloadBlobGasUsed, err := strconv.ParseUint(e.BlobGasUsed, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.BlobGasUsed")
	}
	payloadExcessBlobGas, err := strconv.ParseUint(e.ExcessBlobGas, 10, 64)
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

func (e *ExecutionPayloadHeaderDeneb) ToConsensus() (*enginev1.ExecutionPayloadHeaderDeneb, error) {
	if e == nil {
		return nil, server.NewDecodeError(errNilValue, "ExecutionPayloadHeader")
	}
	payloadParentHash, err := bytesutil.DecodeHexWithLength(e.ParentHash, common.HashLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayloadHeader.ParentHash")
	}
	payloadFeeRecipient, err := bytesutil.DecodeHexWithLength(e.FeeRecipient, fieldparams.FeeRecipientLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayloadHeader.FeeRecipient")
	}
	payloadStateRoot, err := bytesutil.DecodeHexWithLength(e.StateRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayloadHeader.StateRoot")
	}
	payloadReceiptsRoot, err := bytesutil.DecodeHexWithLength(e.ReceiptsRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayloadHeader.ReceiptsRoot")
	}
	payloadLogsBloom, err := bytesutil.DecodeHexWithLength(e.LogsBloom, fieldparams.LogsBloomLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayloadHeader.LogsBloom")
	}
	payloadPrevRandao, err := bytesutil.DecodeHexWithLength(e.PrevRandao, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayloadHeader.PrevRandao")
	}
	payloadBlockNumber, err := strconv.ParseUint(e.BlockNumber, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayloadHeader.BlockNumber")
	}
	payloadGasLimit, err := strconv.ParseUint(e.GasLimit, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayloadHeader.GasLimit")
	}
	payloadGasUsed, err := strconv.ParseUint(e.GasUsed, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayloadHeader.GasUsed")
	}
	payloadTimestamp, err := strconv.ParseUint(e.Timestamp, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayloadHeader.Timestamp")
	}
	payloadExtraData, err := bytesutil.DecodeHexWithMaxLength(e.ExtraData, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayloadHeader.ExtraData")
	}
	payloadBaseFeePerGas, err := bytesutil.Uint256ToSSZBytes(e.BaseFeePerGas)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayloadHeader.BaseFeePerGas")
	}
	payloadBlockHash, err := bytesutil.DecodeHexWithLength(e.BlockHash, common.HashLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayloadHeader.BlockHash")
	}
	payloadTxsRoot, err := bytesutil.DecodeHexWithLength(e.TransactionsRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayloadHeader.TransactionsRoot")
	}
	payloadWithdrawalsRoot, err := bytesutil.DecodeHexWithLength(e.WithdrawalsRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayloadHeader.WithdrawalsRoot")
	}
	payloadBlobGasUsed, err := strconv.ParseUint(e.BlobGasUsed, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.BlobGasUsed")
	}
	payloadExcessBlobGas, err := strconv.ParseUint(e.ExcessBlobGas, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayload.ExcessBlobGas")
	}
	return &enginev1.ExecutionPayloadHeaderDeneb{
		ParentHash:       payloadParentHash,
		FeeRecipient:     payloadFeeRecipient,
		StateRoot:        payloadStateRoot,
		ReceiptsRoot:     payloadReceiptsRoot,
		LogsBloom:        payloadLogsBloom,
		PrevRandao:       payloadPrevRandao,
		BlockNumber:      payloadBlockNumber,
		GasLimit:         payloadGasLimit,
		GasUsed:          payloadGasUsed,
		Timestamp:        payloadTimestamp,
		ExtraData:        payloadExtraData,
		BaseFeePerGas:    payloadBaseFeePerGas,
		BlockHash:        payloadBlockHash,
		TransactionsRoot: payloadTxsRoot,
		WithdrawalsRoot:  payloadWithdrawalsRoot,
		BlobGasUsed:      payloadBlobGasUsed,
		ExcessBlobGas:    payloadExcessBlobGas,
	}, nil
}

// ----------------------------------------------------------------------------
// Electra
// ----------------------------------------------------------------------------

var (
	ExecutionPayloadElectraFromConsensus       = ExecutionPayloadDenebFromConsensus
	ExecutionPayloadHeaderElectraFromConsensus = ExecutionPayloadHeaderDenebFromConsensus
)

func WithdrawalRequestsFromConsensus(ws []*enginev1.WithdrawalRequest) []*WithdrawalRequest {
	result := make([]*WithdrawalRequest, len(ws))
	for i, w := range ws {
		result[i] = WithdrawalRequestFromConsensus(w)
	}
	return result
}

func WithdrawalRequestFromConsensus(w *enginev1.WithdrawalRequest) *WithdrawalRequest {
	return &WithdrawalRequest{
		SourceAddress:   hexutil.Encode(w.SourceAddress),
		ValidatorPubkey: hexutil.Encode(w.ValidatorPubkey),
		Amount:          fmt.Sprintf("%d", w.Amount),
	}
}

func (w *WithdrawalRequest) ToConsensus() (*enginev1.WithdrawalRequest, error) {
	if w == nil {
		return nil, server.NewDecodeError(errNilValue, "WithdrawalRequest")
	}
	src, err := bytesutil.DecodeHexWithLength(w.SourceAddress, common.AddressLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "SourceAddress")
	}
	pubkey, err := bytesutil.DecodeHexWithLength(w.ValidatorPubkey, fieldparams.BLSPubkeyLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ValidatorPubkey")
	}
	amount, err := strconv.ParseUint(w.Amount, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Amount")
	}
	return &enginev1.WithdrawalRequest{
		SourceAddress:   src,
		ValidatorPubkey: pubkey,
		Amount:          amount,
	}, nil
}

func ConsolidationRequestsFromConsensus(cs []*enginev1.ConsolidationRequest) []*ConsolidationRequest {
	result := make([]*ConsolidationRequest, len(cs))
	for i, c := range cs {
		result[i] = ConsolidationRequestFromConsensus(c)
	}
	return result
}

func ConsolidationRequestFromConsensus(c *enginev1.ConsolidationRequest) *ConsolidationRequest {
	return &ConsolidationRequest{
		SourceAddress: hexutil.Encode(c.SourceAddress),
		SourcePubkey:  hexutil.Encode(c.SourcePubkey),
		TargetPubkey:  hexutil.Encode(c.TargetPubkey),
	}
}

func (c *ConsolidationRequest) ToConsensus() (*enginev1.ConsolidationRequest, error) {
	if c == nil {
		return nil, server.NewDecodeError(errNilValue, "ConsolidationRequest")
	}
	srcAddress, err := bytesutil.DecodeHexWithLength(c.SourceAddress, common.AddressLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "SourceAddress")
	}
	srcPubkey, err := bytesutil.DecodeHexWithLength(c.SourcePubkey, fieldparams.BLSPubkeyLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "SourcePubkey")
	}
	targetPubkey, err := bytesutil.DecodeHexWithLength(c.TargetPubkey, fieldparams.BLSPubkeyLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "TargetPubkey")
	}
	return &enginev1.ConsolidationRequest{
		SourceAddress: srcAddress,
		SourcePubkey:  srcPubkey,
		TargetPubkey:  targetPubkey,
	}, nil
}

func DepositRequestsFromConsensus(ds []*enginev1.DepositRequest) []*DepositRequest {
	result := make([]*DepositRequest, len(ds))
	for i, d := range ds {
		result[i] = DepositRequestFromConsensus(d)
	}
	return result
}

func DepositRequestFromConsensus(d *enginev1.DepositRequest) *DepositRequest {
	return &DepositRequest{
		Pubkey:                hexutil.Encode(d.Pubkey),
		WithdrawalCredentials: hexutil.Encode(d.WithdrawalCredentials),
		Amount:                fmt.Sprintf("%d", d.Amount),
		Signature:             hexutil.Encode(d.Signature),
		Index:                 fmt.Sprintf("%d", d.Index),
	}
}

func (d *DepositRequest) ToConsensus() (*enginev1.DepositRequest, error) {
	if d == nil {
		return nil, server.NewDecodeError(errNilValue, "DepositRequest")
	}
	pubkey, err := bytesutil.DecodeHexWithLength(d.Pubkey, fieldparams.BLSPubkeyLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Pubkey")
	}
	withdrawalCredentials, err := bytesutil.DecodeHexWithLength(d.WithdrawalCredentials, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "WithdrawalCredentials")
	}
	amount, err := strconv.ParseUint(d.Amount, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Amount")
	}
	sig, err := bytesutil.DecodeHexWithLength(d.Signature, fieldparams.BLSSignatureLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Signature")
	}
	index, err := strconv.ParseUint(d.Index, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Index")
	}
	return &enginev1.DepositRequest{
		Pubkey:                pubkey,
		WithdrawalCredentials: withdrawalCredentials,
		Amount:                amount,
		Signature:             sig,
		Index:                 index,
	}, nil
}

func ExecutionRequestsFromConsensus(er *enginev1.ExecutionRequests) *ExecutionRequests {
	return &ExecutionRequests{
		Deposits:       DepositRequestsFromConsensus(er.Deposits),
		Withdrawals:    WithdrawalRequestsFromConsensus(er.Withdrawals),
		Consolidations: ConsolidationRequestsFromConsensus(er.Consolidations),
	}
}

func (e *ExecutionRequests) ToConsensus() (*enginev1.ExecutionRequests, error) {
	if e == nil {
		return nil, server.NewDecodeError(errNilValue, "ExecutionRequests")
	}
	var err error
	if err = slice.VerifyMaxLength(e.Deposits, params.BeaconConfig().MaxDepositRequestsPerPayload); err != nil {
		return nil, err
	}
	depositRequests := make([]*enginev1.DepositRequest, len(e.Deposits))
	for i, d := range e.Deposits {
		depositRequests[i], err = d.ToConsensus()
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("ExecutionRequests.Deposits[%d]", i))
		}
	}

	if err = slice.VerifyMaxLength(e.Withdrawals, params.BeaconConfig().MaxWithdrawalRequestsPerPayload); err != nil {
		return nil, err
	}
	withdrawalRequests := make([]*enginev1.WithdrawalRequest, len(e.Withdrawals))
	for i, w := range e.Withdrawals {
		withdrawalRequests[i], err = w.ToConsensus()
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("ExecutionRequests.Withdrawals[%d]", i))
		}
	}

	if err = slice.VerifyMaxLength(e.Consolidations, params.BeaconConfig().MaxConsolidationsRequestsPerPayload); err != nil {
		return nil, err
	}
	consolidationRequests := make([]*enginev1.ConsolidationRequest, len(e.Consolidations))
	for i, c := range e.Consolidations {
		consolidationRequests[i], err = c.ToConsensus()
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("ExecutionRequests.Consolidations[%d]", i))
		}
	}
	return &enginev1.ExecutionRequests{
		Deposits:       depositRequests,
		Withdrawals:    withdrawalRequests,
		Consolidations: consolidationRequests,
	}, nil
}

// ----------------------------------------------------------------------------
// Fulu
// ----------------------------------------------------------------------------

var (
	ExecutionPayloadFuluFromConsensus       = ExecutionPayloadDenebFromConsensus
	ExecutionPayloadHeaderFuluFromConsensus = ExecutionPayloadHeaderDenebFromConsensus
	BeaconBlockFuluFromConsensus            = BeaconBlockElectraFromConsensus
)
