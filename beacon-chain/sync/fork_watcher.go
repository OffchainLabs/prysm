package sync

import (
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/pkg/errors"
)

// Is a background routine that observes for new incoming forks. Depending on the epoch
// it will be in charge of subscribing/unsubscribing the relevant topics at the fork boundaries.
func (s *Service) forkWatcher() {
	// At startup, launch registration and peer discovery loops, and register rpc stream handlers.
	startEntry := params.GetNetworkScheduleEntry(s.cfg.clock.CurrentEpoch())
	s.registerSubscribers(startEntry)

	slotTicker := slots.NewSlotTicker(s.cfg.clock.GenesisTime(), params.BeaconConfig().SecondsPerSlot)
	for {
		select {
		// In the event of a node restart, we will still end up subscribing to the correct
		// topics during/after the fork epoch. This routine is to ensure correct
		// subscriptions for nodes running before a fork epoch.
		case <-slotTicker.C():
			current := s.cfg.clock.CurrentEpoch()
			if err := s.registerForUpcomingFork(current); err != nil {
				log.WithError(err).Error("Unable to check for fork in the next epoch")
				continue
			}
			if err := s.deregisterFromPastFork(current); err != nil {
				log.WithError(err).Error("Unable to check for fork in the previous epoch")
				continue
			}
		case <-s.ctx.Done():
			log.Debug("Context closed, exiting goroutine")
			slotTicker.Done()
			return
		}
	}
}

// registerForUpcomingFork registers appropriate gossip and RPC topic if there is a fork in the next epoch.
func (s *Service) registerForUpcomingFork(epoch primitives.Epoch) error {
	current := params.GetNetworkScheduleEntry(epoch)
	s.registerSubscribers(current)

	currentHandler, err := s.rpcHandlerByTopicFromFork(current.VersionEnum)
	if err != nil {
		return errors.Wrap(err, "RPC handler by topic from before fork epoch")
	}
	if !s.digestActionDone(current.ForkDigest, registerRpcOnce) {
		for topic, handler := range currentHandler {
			s.registerRPC(topic, handler)
		}
	}

	next := params.GetNetworkScheduleEntry(epoch + 1)
	if current.Epoch == next.Epoch {
		return nil // no fork in the next epoch
	}
	s.registerSubscribers(next)

	if s.digestActionDone(next.ForkDigest, registerRpcOnce) {
		return nil
	}

	nextHandler, err := s.rpcHandlerByTopicFromFork(next.VersionEnum)
	if err != nil {
		return errors.Wrap(err, "RPC handler by topic from fork epoch")
	}
	// Compute newly added topics.
	newHandlersByTopic := addedRPCHandlerByTopic(currentHandler, nextHandler)
	// Register the new RPC handlers.
	// We deregister the old topics later, at least one epoch after the fork.
	for topic, handler := range newHandlersByTopic {
		s.registerRPC(topic, handler)
	}

	return nil
}

// deregisterFromPastFork deregisters appropriate gossip and RPC topic if there is a fork in the current epoch.
func (s *Service) deregisterFromPastFork(currentEpoch primitives.Epoch) error {
	current := params.GetNetworkScheduleEntry(currentEpoch)

	// If we are still in our genesis fork version then exit early.
	if current.Epoch == params.BeaconConfig().GenesisEpoch {
		return nil
	}
	if currentEpoch < current.Epoch+1 {
		return nil // wait until we are 1 epoch into the fork
	}

	previous := params.GetNetworkScheduleEntry(current.Epoch - 1)
	// Remove stream handlers for all topics that are in the set of
	// currentTopics-previousTopics
	if !s.digestActionDone(previous.ForkDigest, unregisterRpcOnce) {
		previousTopics, err := s.rpcHandlerByTopicFromFork(previous.VersionEnum)
		if err != nil {
			return errors.Wrap(err, "RPC handler by topic from before fork epoch")
		}
		currentTopics, err := s.rpcHandlerByTopicFromFork(current.VersionEnum)
		if err != nil {
			return errors.Wrap(err, "RPC handler by topic from fork epoch")
		}
		topicsToRemove := removedRPCTopics(previousTopics, currentTopics)
		for topic := range topicsToRemove {
			fullTopic := topic + s.cfg.p2p.Encoding().ProtocolSuffix()
			s.cfg.p2p.Host().RemoveStreamHandler(protocol.ID(fullTopic))
			log.WithField("topic", fullTopic).Debug("Removed RPC handler")
		}
	}

	// Unsubscribe from all gossip topics with the previous fork digest.
	if s.digestActionDone(previous.ForkDigest, unregisterGossipOnce) {
		return nil
	}
	for _, t := range s.subHandler.allTopics() {
		retDigest, err := p2p.ExtractGossipDigest(t)
		if err != nil {
			log.WithError(err).Error("Could not retrieve digest")
			continue
		}
		if retDigest == previous.ForkDigest {
			s.unSubscribeFromTopic(t)
		}
	}

	return nil
}
