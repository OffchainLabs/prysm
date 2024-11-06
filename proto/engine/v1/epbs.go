package enginev1

func (s *SignedExecutionPayloadEnvelope) Blind() *SignedBlindPayloadEnvelope {
	if s.Message == nil {
		return nil
	}
	if s.Message.Payload == nil {
		return nil
	}
	payloadRoot, err := s.Message.Payload.HashTreeRoot()
	if err != nil {
		return nil
	}
	return &SignedBlindPayloadEnvelope{
		Message: &BlindPayloadEnvelope{
			PayloadRoot:        payloadRoot[:],
			ExecutionRequests:  s.Message.ExecutionRequests,
			BuilderIndex:       s.Message.BuilderIndex,
			BeaconBlockRoot:    s.Message.BeaconBlockRoot,
			BlobKzgCommitments: s.Message.BlobKzgCommitments,
			PayloadWithheld:    s.Message.PayloadWithheld,
			StateRoot:          s.Message.StateRoot,
		},
		Signature: s.Signature,
	}
}
