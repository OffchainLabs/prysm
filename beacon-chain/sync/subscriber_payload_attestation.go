package sync

import (
	"context"

	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"google.golang.org/protobuf/proto"
)

func (s *Service) payloadAttestationSubscriber(ctx context.Context, msg proto.Message) error {
	a, ok := msg.(*eth.PayloadAttestationMessage)
	if !ok {
		return errWrongMessage
	}

	if err := s.payloadAttestationCache.Add(a.Data.Slot, a.ValidatorIndex); err != nil {
		return err
	}

	return s.cfg.chain.ReceivePayloadAttestationMessage(ctx, a)
}
