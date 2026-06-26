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
		Weight:        payment.Weight,
		Withdrawal:    payment.Withdrawal.Copy(),
		ProposerIndex: payment.ProposerIndex,
	}
}

// WireBlinded derives the spec-wire blinded envelope from a full one: payload_root is
// HashTreeRoot(payload), so HashTreeRoot(blinded) == HashTreeRoot(full) and a validator signature
// over either form is valid against the other.
func (e *ExecutionPayloadEnvelope) WireBlinded() (*WireBlindedExecutionPayloadEnvelope, error) {
	if e == nil {
		return nil, nil
	}
	payloadRoot, err := e.Payload.HashTreeRoot()
	if err != nil {
		return nil, err
	}
	return &WireBlindedExecutionPayloadEnvelope{
		PayloadRoot:           payloadRoot[:],
		ExecutionRequests:     e.ExecutionRequests,
		BuilderIndex:          e.BuilderIndex,
		BeaconBlockRoot:       bytesutil.SafeCopyBytes(e.BeaconBlockRoot),
		ParentBeaconBlockRoot: bytesutil.SafeCopyBytes(e.ParentBeaconBlockRoot),
	}, nil
}

// WireBlinded lifts a signed envelope to its blinded form, preserving the signature.
func (e *SignedExecutionPayloadEnvelope) WireBlinded() (*SignedWireBlindedExecutionPayloadEnvelope, error) {
	if e == nil {
		return nil, nil
	}
	msg, err := e.Message.WireBlinded()
	if err != nil {
		return nil, err
	}
	return &SignedWireBlindedExecutionPayloadEnvelope{
		Message:   msg,
		Signature: bytesutil.SafeCopyBytes(e.Signature),
	}, nil
}
