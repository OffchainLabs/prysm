package enginev1

import "github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"

type copier[T any] interface {
	Copy() T
}

func copySlice[T any, C copier[T]](original []C) []T {
	// Create a new slice with the same length as the original
	newSlice := make([]T, len(original))
	for i := 0; i < len(newSlice); i++ {
		newSlice[i] = original[i].Copy()
	}
	return newSlice
}

// Copy --
func (w *Withdrawal) Copy() *Withdrawal {
	if w == nil {
		return nil
	}

	return &Withdrawal{
		Index:          w.Index,
		ValidatorIndex: w.ValidatorIndex,
		Address:        bytesutil.SafeCopyBytes(w.Address),
		Amount:         w.Amount,
	}
}

// Copy --
func (d *DepositRequest) Copy() *DepositRequest {
	if d == nil {
		return nil
	}
	return &DepositRequest{
		Pubkey:                bytesutil.SafeCopyBytes(d.Pubkey),
		WithdrawalCredentials: bytesutil.SafeCopyBytes(d.WithdrawalCredentials),
		Amount:                d.Amount,
		Signature:             bytesutil.SafeCopyBytes(d.Signature),
		Index:                 d.Index,
	}
}

// Copy --
func (wr *WithdrawalRequest) Copy() *WithdrawalRequest {
	if wr == nil {
		return nil
	}
	return &WithdrawalRequest{
		SourceAddress:   bytesutil.SafeCopyBytes(wr.SourceAddress),
		ValidatorPubkey: bytesutil.SafeCopyBytes(wr.ValidatorPubkey),
		Amount:          wr.Amount,
	}
}

// Copy --
func (cr *ConsolidationRequest) Copy() *ConsolidationRequest {
	if cr == nil {
		return nil
	}
	return &ConsolidationRequest{
		SourceAddress: bytesutil.SafeCopyBytes(cr.SourceAddress),
		SourcePubkey:  bytesutil.SafeCopyBytes(cr.SourcePubkey),
		TargetPubkey:  bytesutil.SafeCopyBytes(cr.TargetPubkey),
	}
}

// Copy -- Deneb
func (payload *ExecutionPayloadDeneb) Copy() *ExecutionPayloadDeneb {
	if payload == nil {
		return nil
	}
	return &ExecutionPayloadDeneb{
		ParentHash:    bytesutil.SafeCopyBytes(payload.ParentHash),
		FeeRecipient:  bytesutil.SafeCopyBytes(payload.FeeRecipient),
		StateRoot:     bytesutil.SafeCopyBytes(payload.StateRoot),
		ReceiptsRoot:  bytesutil.SafeCopyBytes(payload.ReceiptsRoot),
		LogsBloom:     bytesutil.SafeCopyBytes(payload.LogsBloom),
		PrevRandao:    bytesutil.SafeCopyBytes(payload.PrevRandao),
		BlockNumber:   payload.BlockNumber,
		GasLimit:      payload.GasLimit,
		GasUsed:       payload.GasUsed,
		Timestamp:     payload.Timestamp,
		ExtraData:     bytesutil.SafeCopyBytes(payload.ExtraData),
		BaseFeePerGas: bytesutil.SafeCopyBytes(payload.BaseFeePerGas),
		BlockHash:     bytesutil.SafeCopyBytes(payload.BlockHash),
		Transactions:  bytesutil.SafeCopy2dBytes(payload.Transactions),
		Withdrawals:   copySlice(payload.Withdrawals),
		BlobGasUsed:   payload.BlobGasUsed,
		ExcessBlobGas: payload.ExcessBlobGas,
	}
}

// Copy -- Capella
func (payload *ExecutionPayloadCapella) Copy() *ExecutionPayloadCapella {
	if payload == nil {
		return nil
	}

	return &ExecutionPayloadCapella{
		ParentHash:    bytesutil.SafeCopyBytes(payload.ParentHash),
		FeeRecipient:  bytesutil.SafeCopyBytes(payload.FeeRecipient),
		StateRoot:     bytesutil.SafeCopyBytes(payload.StateRoot),
		ReceiptsRoot:  bytesutil.SafeCopyBytes(payload.ReceiptsRoot),
		LogsBloom:     bytesutil.SafeCopyBytes(payload.LogsBloom),
		PrevRandao:    bytesutil.SafeCopyBytes(payload.PrevRandao),
		BlockNumber:   payload.BlockNumber,
		GasLimit:      payload.GasLimit,
		GasUsed:       payload.GasUsed,
		Timestamp:     payload.Timestamp,
		ExtraData:     bytesutil.SafeCopyBytes(payload.ExtraData),
		BaseFeePerGas: bytesutil.SafeCopyBytes(payload.BaseFeePerGas),
		BlockHash:     bytesutil.SafeCopyBytes(payload.BlockHash),
		Transactions:  bytesutil.SafeCopy2dBytes(payload.Transactions),
		Withdrawals:   copySlice(payload.Withdrawals),
	}
}

// Copy -- Bellatrix
func (payload *ExecutionPayload) Copy() *ExecutionPayload {
	if payload == nil {
		return nil
	}

	return &ExecutionPayload{
		ParentHash:    bytesutil.SafeCopyBytes(payload.ParentHash),
		FeeRecipient:  bytesutil.SafeCopyBytes(payload.FeeRecipient),
		StateRoot:     bytesutil.SafeCopyBytes(payload.StateRoot),
		ReceiptsRoot:  bytesutil.SafeCopyBytes(payload.ReceiptsRoot),
		LogsBloom:     bytesutil.SafeCopyBytes(payload.LogsBloom),
		PrevRandao:    bytesutil.SafeCopyBytes(payload.PrevRandao),
		BlockNumber:   payload.BlockNumber,
		GasLimit:      payload.GasLimit,
		GasUsed:       payload.GasUsed,
		Timestamp:     payload.Timestamp,
		ExtraData:     bytesutil.SafeCopyBytes(payload.ExtraData),
		BaseFeePerGas: bytesutil.SafeCopyBytes(payload.BaseFeePerGas),
		BlockHash:     bytesutil.SafeCopyBytes(payload.BlockHash),
		Transactions:  bytesutil.SafeCopy2dBytes(payload.Transactions),
	}
}

// Copy -- Deneb
func (payload *ExecutionPayloadHeaderDeneb) Copy() *ExecutionPayloadHeaderDeneb {
	if payload == nil {
		return nil
	}
	return &ExecutionPayloadHeaderDeneb{
		ParentHash:       bytesutil.SafeCopyBytes(payload.ParentHash),
		FeeRecipient:     bytesutil.SafeCopyBytes(payload.FeeRecipient),
		StateRoot:        bytesutil.SafeCopyBytes(payload.StateRoot),
		ReceiptsRoot:     bytesutil.SafeCopyBytes(payload.ReceiptsRoot),
		LogsBloom:        bytesutil.SafeCopyBytes(payload.LogsBloom),
		PrevRandao:       bytesutil.SafeCopyBytes(payload.PrevRandao),
		BlockNumber:      payload.BlockNumber,
		GasLimit:         payload.GasLimit,
		GasUsed:          payload.GasUsed,
		Timestamp:        payload.Timestamp,
		ExtraData:        bytesutil.SafeCopyBytes(payload.ExtraData),
		BaseFeePerGas:    bytesutil.SafeCopyBytes(payload.BaseFeePerGas),
		BlockHash:        bytesutil.SafeCopyBytes(payload.BlockHash),
		TransactionsRoot: bytesutil.SafeCopyBytes(payload.TransactionsRoot),
		WithdrawalsRoot:  bytesutil.SafeCopyBytes(payload.WithdrawalsRoot),
		BlobGasUsed:      payload.BlobGasUsed,
		ExcessBlobGas:    payload.ExcessBlobGas,
	}
}

// Copy -- Capella
func (payload *ExecutionPayloadHeaderCapella) Copy() *ExecutionPayloadHeaderCapella {
	if payload == nil {
		return nil
	}
	return &ExecutionPayloadHeaderCapella{
		ParentHash:       bytesutil.SafeCopyBytes(payload.ParentHash),
		FeeRecipient:     bytesutil.SafeCopyBytes(payload.FeeRecipient),
		StateRoot:        bytesutil.SafeCopyBytes(payload.StateRoot),
		ReceiptsRoot:     bytesutil.SafeCopyBytes(payload.ReceiptsRoot),
		LogsBloom:        bytesutil.SafeCopyBytes(payload.LogsBloom),
		PrevRandao:       bytesutil.SafeCopyBytes(payload.PrevRandao),
		BlockNumber:      payload.BlockNumber,
		GasLimit:         payload.GasLimit,
		GasUsed:          payload.GasUsed,
		Timestamp:        payload.Timestamp,
		ExtraData:        bytesutil.SafeCopyBytes(payload.ExtraData),
		BaseFeePerGas:    bytesutil.SafeCopyBytes(payload.BaseFeePerGas),
		BlockHash:        bytesutil.SafeCopyBytes(payload.BlockHash),
		TransactionsRoot: bytesutil.SafeCopyBytes(payload.TransactionsRoot),
		WithdrawalsRoot:  bytesutil.SafeCopyBytes(payload.WithdrawalsRoot),
	}
}

// Copy -- Bellatrix
func (payload *ExecutionPayloadHeader) Copy() *ExecutionPayloadHeader {
	if payload == nil {
		return nil
	}
	return &ExecutionPayloadHeader{
		ParentHash:       bytesutil.SafeCopyBytes(payload.ParentHash),
		FeeRecipient:     bytesutil.SafeCopyBytes(payload.FeeRecipient),
		StateRoot:        bytesutil.SafeCopyBytes(payload.StateRoot),
		ReceiptsRoot:     bytesutil.SafeCopyBytes(payload.ReceiptsRoot),
		LogsBloom:        bytesutil.SafeCopyBytes(payload.LogsBloom),
		PrevRandao:       bytesutil.SafeCopyBytes(payload.PrevRandao),
		BlockNumber:      payload.BlockNumber,
		GasLimit:         payload.GasLimit,
		GasUsed:          payload.GasUsed,
		Timestamp:        payload.Timestamp,
		ExtraData:        bytesutil.SafeCopyBytes(payload.ExtraData),
		BaseFeePerGas:    bytesutil.SafeCopyBytes(payload.BaseFeePerGas),
		BlockHash:        bytesutil.SafeCopyBytes(payload.BlockHash),
		TransactionsRoot: bytesutil.SafeCopyBytes(payload.TransactionsRoot),
	}
}
