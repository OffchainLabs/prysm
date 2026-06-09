package sync

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed"
	opfeed "github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/operation"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"google.golang.org/protobuf/proto"
)

func (s *Service) signedProposerPreferencesSubscriber(_ context.Context, msg proto.Message) error {
	signedPreferences, ok := msg.(*ethpb.SignedProposerPreferences)
	if !ok {
		return errWrongMessage
	}
	if signedPreferences == nil || signedPreferences.Message == nil {
		return errNilMessage
	}
	s.cfg.operationNotifier.OperationFeed().Send(&feed.Event{
		Type: opfeed.ProposerPreferencesReceived,
		Data: &opfeed.ProposerPreferencesReceivedData{
			Data: signedPreferences,
		},
	})
	return nil
}
