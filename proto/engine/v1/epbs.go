package enginev1

func (s *SignedExecutionPayloadEnvelope) Blind() *SignedBlindPayloadEnvelope {
	if s.Message == nil {
		return nil
	}
	if s.Message.Payload == nil {
		return nil
	}
	return &SignedBlindPayloadEnvelope{
		Message: &BlindPayloadEnvelope{
			ParentHash:         s.Message.Payload.ParentHash,
			FeeRecipient:       s.Message.Payload.FeeRecipient,
			StateRoot:          s.Message.Payload.StateRoot,
			ReceiptsRoot:       s.Message.Payload.ReceiptsRoot,
			LogsBloom:          s.Message.Payload.LogsBloom,
			PrevRandao:         s.Message.Payload.PrevRandao,
			BlockNumber:        s.Message.Payload.BlockNumber,
			GasLimit:           s.Message.Payload.GasLimit,
			GasUsed:            s.Message.Payload.GasUsed,
			Timestamp:          s.Message.Payload.Timestamp,
			ExtraData:          s.Message.Payload.ExtraData,
			BaseFeePerGas:      s.Message.Payload.BaseFeePerGas,
			BlockHash:          s.Message.Payload.BlockHash,
			BlobGasUsed:        s.Message.Payload.BlobGasUsed,
			ExcessBlobGas:      s.Message.Payload.ExcessBlobGas,
			BuilderIndex:       s.Message.BuilderIndex,
			BeaconBlockRoot:    s.Message.BeaconBlockRoot,
			Slot:               s.Message.Slot,
			BlobKzgCommitments: s.Message.BlobKzgCommitments,
			BeaconStateRoot:    s.Message.BeaconStateRoot,
		},
		Signature: s.Signature,
	}
}
