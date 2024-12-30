package sync

import (
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/network/forks"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
)

// Is a background routine that observes for new incoming forks. Depending on the epoch
// it will be in charge of subscribing/unsubscribing the relevant topics at the fork boundaries.
func (s *Service) forkWatcher() {
	slotTicker := slots.NewSlotTicker(s.cfg.clock.GenesisTime(), params.BeaconConfig().SecondsPerSlot)
	for {
		select {
		// In the event of a node restart, we will still end up subscribing to the correct
		// topics during/after the fork epoch. This routine is to ensure correct
		// subscriptions for nodes running before a fork epoch.
		case currSlot := <-slotTicker.C():
			currEpoch := slots.ToEpoch(currSlot)
			if err := s.registerForUpcomingFork(currEpoch); err != nil {
				log.WithError(err).Error("Unable to check for fork in the next epoch")
				continue
			}
			if err := s.deregisterFromPastFork(currEpoch); err != nil {
				log.WithError(err).Error("Unable to check for fork in the previous epoch")
				continue
			}
			// Broadcast BLS changes at the Capella fork boundary
			s.broadcastBLSChanges(currSlot)

		case <-s.ctx.Done():
			log.Debug("Context closed, exiting goroutine")
			slotTicker.Done()
			return
		}
	}
}

// registerForUpcomingFork registers appropriate gossip and RPC topic if there is a fork in the next epoch.
func (s *Service) registerForUpcomingFork(currentEpoch primitives.Epoch) error {
	// Get the genesis validators root.
	genesisValidatorsRoot := s.cfg.clock.GenesisValidatorsRoot()

	// Check if there is a fork in the next epoch.
	isForkNextEpoch, err := forks.IsForkNextEpoch(s.cfg.clock.GenesisTime(), genesisValidatorsRoot[:])
	if err != nil {
		return errors.Wrap(err, "Could not retrieve next fork epoch")
	}

	// Exit early if there is no fork in the next epoch.
	if !isForkNextEpoch {
		return nil
	}

	beforeForkEpoch := currentEpoch
	forkEpoch := beforeForkEpoch + 1

	// Get the fork afterForkDigest for the next epoch.
	afterForkDigest, err := forks.ForkDigestFromEpoch(forkEpoch, genesisValidatorsRoot[:])
	if err != nil {
		return errors.Wrap(err, "could not retrieve fork digest")
	}

	// Exit early if the topics for the next epoch are already registered.
	// It likely to be the case for all slots of the epoch that are not the first one.
	if s.subHandler.digestExists(afterForkDigest) {
		return nil
	}

	// Register the subscribers (gossipsub) for the next epoch.
	s.registerSubscribers(forkEpoch, afterForkDigest)

	// Get the handlers for the current and next fork.
	beforeForkHandlerByTopic, err := s.rpcHandlerByTopicFromEpoch(beforeForkEpoch)
	if err != nil {
		return errors.Wrap(err, "RPC handler by topic from before fork epoch")
	}

	forkHandlerByTopic, err := s.rpcHandlerByTopicFromEpoch(forkEpoch)
	if err != nil {
		return errors.Wrap(err, "RPC handler by topic from fork epoch")
	}

	// Compute newly added topics.
	newRPCHandlerByTopic := addedRPCHandlerByTopic(beforeForkHandlerByTopic, forkHandlerByTopic)

	// Register the new RPC handlers.
	for topic, handler := range newRPCHandlerByTopic {
		s.registerRPC(topic, handler)
	}

	return nil
}

// deregisterFromPastFork deregisters appropriate gossip and RPC topic if there is a fork in the current epoch.
func (s *Service) deregisterFromPastFork(currentEpoch primitives.Epoch) error {
	// Extract the genesis validators root.
	genesisValidatorsRoot := s.cfg.clock.GenesisValidatorsRoot()

	// Get the fork.
	currentFork, err := forks.Fork(currentEpoch)
	if err != nil {
		return errors.Wrap(err, "genesis validators root")
	}

	// If we are still in our genesis fork version then exit early.
	if currentFork.Epoch == params.BeaconConfig().GenesisEpoch {
		return nil
	}

	// Get the epoch after the fork epoch.
	afterForkEpoch := currentFork.Epoch + 1

	// Start de-registering if the current epoch is after the fork epoch.
	if currentEpoch != afterForkEpoch {
		return nil
	}

	// Look at the previous fork's digest.
	beforeForkEpoch := currentFork.Epoch - 1

	beforeForkDigest, err := forks.ForkDigestFromEpoch(beforeForkEpoch, genesisValidatorsRoot[:])
	if err != nil {
		return errors.Wrap(err, "fork digest from epoch")
	}

	// Exit early if there are no topics with that particular digest.
	if !s.subHandler.digestExists(beforeForkDigest) {
		return nil
	}

	// Compute the RPC handlers that are no longer needed.
	beforeForkHandlerByTopic, err := s.rpcHandlerByTopicFromEpoch(beforeForkEpoch)
	if err != nil {
		return errors.Wrap(err, "RPC handler by topic from before fork epoch")
	}

	forkHandlerByTopic, err := s.rpcHandlerByTopicFromEpoch(currentFork.Epoch)
	if err != nil {
		return errors.Wrap(err, "RPC handler by topic from fork epoch")
	}

	topicsToRemove := removedRPCTopics(beforeForkHandlerByTopic, forkHandlerByTopic)
	for topic := range topicsToRemove {
		fullTopic := topic + s.cfg.p2p.Encoding().ProtocolSuffix()
		s.cfg.p2p.Host().RemoveStreamHandler(protocol.ID(fullTopic))
	}

	// Run through all our current active topics and see
	// if there are any subscriptions to be removed.
	for _, t := range s.subHandler.allTopics() {
		retDigest, err := p2p.ExtractGossipDigest(t)
		if err != nil {
			log.WithError(err).Error("Could not retrieve digest")
			continue
		}
		if retDigest == beforeForkDigest {
			s.unSubscribeFromTopic(t)
		}
	}

	return nil
}
