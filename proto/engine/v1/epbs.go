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
			BlockHash:          s.Message.Payload.BlockHash,
			ExecutionRequests:  s.Message.ExecutionRequests,
			BuilderIndex:       s.Message.BuilderIndex,
			BeaconBlockRoot:    s.Message.BeaconBlockRoot,
			BlobKzgCommitments: s.Message.BlobKzgCommitments,
			StateRoot:          s.Message.StateRoot,
		},
		Signature: s.Signature,
	}
}
