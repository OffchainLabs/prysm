package sync

import (
	"context"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/feed"
	statefeed "github.com/OffchainLabs/prysm/v6/beacon-chain/core/feed/state"
	lightclientTypes "github.com/OffchainLabs/prysm/v6/consensus-types/light-client"
	"google.golang.org/protobuf/proto"
)

func (s *Service) lightClientOptimisticUpdateSubscriber(_ context.Context, msg proto.Message) error {
	update, err := lightclientTypes.NewWrappedOptimisticUpdate(msg)
	if err != nil {
		return err
	}

	log.Debug("Saving newly received light client optimistic update. Attested slot %d, Signature slot %d", update.AttestedHeader().Beacon().Slot, update.SignatureSlot())
	s.lcStore.SetLastOptimisticUpdate(update)

	s.cfg.stateNotifier.StateFeed().Send(&feed.Event{
		Type: statefeed.LightClientOptimisticUpdate,
		Data: update,
	})

	return nil
}

func (s *Service) lightClientFinalityUpdateSubscriber(_ context.Context, msg proto.Message) error {
	update, err := lightclientTypes.NewWrappedFinalityUpdate(msg)
	if err != nil {
		return err
	}

	log.Debug("Saving newly received light client finality update. Attested slot %d, Signature slot %d", update.AttestedHeader().Beacon().Slot, update.SignatureSlot())
	s.lcStore.SetLastFinalityUpdate(update)

	s.cfg.stateNotifier.StateFeed().Send(&feed.Event{
		Type: statefeed.LightClientFinalityUpdate,
		Data: update,
	})

	return nil
}
