package eth

import (
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
)

// Copy creates a deep copy of ExecutionPayloadBid.
func (header *ExecutionPayloadBid) Copy() *ExecutionPayloadBid {
	if header == nil {
		return nil
	}
	return &ExecutionPayloadBid{
		ParentBlockHash:       bytesutil.SafeCopyBytes(header.ParentBlockHash),
		ParentBlockRoot:       bytesutil.SafeCopyBytes(header.ParentBlockRoot),
		BlockHash:             bytesutil.SafeCopyBytes(header.BlockHash),
		PrevRandao:            bytesutil.SafeCopyBytes(header.PrevRandao),
		FeeRecipient:          bytesutil.SafeCopyBytes(header.FeeRecipient),
		GasLimit:              header.GasLimit,
		BuilderIndex:          header.BuilderIndex,
		Slot:                  header.Slot,
		Value:                 header.Value,
		ExecutionPayment:      header.ExecutionPayment,
		BlobKzgCommitments:    bytesutil.SafeCopy2dBytes(header.BlobKzgCommitments),
		ExecutionRequestsRoot: bytesutil.SafeCopyBytes(header.ExecutionRequestsRoot),
	}
}

// Copy creates a deep copy of BuilderPendingWithdrawal.
func (withdrawal *BuilderPendingWithdrawal) Copy() *BuilderPendingWithdrawal {
	if withdrawal == nil {
		return nil
	}
	return &BuilderPendingWithdrawal{
		FeeRecipient: bytesutil.SafeCopyBytes(withdrawal.FeeRecipient),
		Amount:       withdrawal.Amount,
		BuilderIndex: withdrawal.BuilderIndex,
	}
}

// Copy creates a deep copy of BuilderPendingPayment.
func (payment *BuilderPendingPayment) Copy() *BuilderPendingPayment {
	if payment == nil {
		return nil
	}
	return &BuilderPendingPayment{
		Weight:     payment.Weight,
		Withdrawal: payment.Withdrawal.Copy(),
	}
}

// WireBlindedFromFull derives the blinded form; payload_root=HTR(payload) so HTR(blinded)==HTR(full).
func WireBlindedFromFull(full *ExecutionPayloadEnvelope) (*WireBlindedExecutionPayloadEnvelope, error) {
	if full == nil {
		return nil, nil
	}
	payloadRoot, err := full.Payload.HashTreeRoot()
	if err != nil {
		return nil, err
	}
	return &WireBlindedExecutionPayloadEnvelope{
		PayloadRoot:           payloadRoot[:],
		ExecutionRequests:     full.ExecutionRequests,
		BuilderIndex:          full.BuilderIndex,
		BeaconBlockRoot:       bytesutil.SafeCopyBytes(full.BeaconBlockRoot),
		ParentBeaconBlockRoot: bytesutil.SafeCopyBytes(full.ParentBeaconBlockRoot),
	}, nil
}

// SignedWireBlindedFromFull lifts a signed envelope to blinded form, preserving the signature (HTR equivalence).
func SignedWireBlindedFromFull(full *SignedExecutionPayloadEnvelope) (*SignedWireBlindedExecutionPayloadEnvelope, error) {
	if full == nil {
		return nil, nil
	}
	msg, err := WireBlindedFromFull(full.Message)
	if err != nil {
		return nil, err
	}
	return &SignedWireBlindedExecutionPayloadEnvelope{
		Message:   msg,
		Signature: bytesutil.SafeCopyBytes(full.Signature),
	}, nil
}
