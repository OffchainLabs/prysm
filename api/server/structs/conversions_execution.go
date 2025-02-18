package structs

import (
	"fmt"
	"strconv"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/api/server"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
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
