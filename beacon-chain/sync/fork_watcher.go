package sync

import (
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/pkg/errors"
)

// p2pHandlerControlLoop runs in a continuous loop to ensure that:
// - We are subscribed to the correct gossipsub topics (for the current and upcoming epoch).
// - We have registered the correct RPC stream handlers (for the current and upcoming epoch).
// - We have cleaned up gossipsub topics and RPC stream handlers that are no longer needed.
func (s *Service) p2pRPCHandlerControlLoop() {
	slotTicker := slots.NewSlotTicker(s.cfg.clock.GenesisTime(), params.BeaconConfig().SecondsPerSlot)
	for {
		select {
		case <-slotTicker.C():
			current := s.cfg.clock.CurrentEpoch()
			if err := s.ensureRPCRegistrationsForEpoch(current); err != nil {
				log.WithError(err).Error("Unable to check for fork in the next epoch")
				continue
			}
			if err := s.ensureRPCDeregistrationForEpoch(current); err != nil {
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

// ensureRegistrationsForEpoch ensures that gossip topic and RPC stream handler
// registrations are in place for the current and subsequent epoch.
func (s *Service) ensureRPCRegistrationsForEpoch(epoch primitives.Epoch) error {
	current := params.GetNetworkScheduleEntry(epoch)

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

// ensureDeregistrationForEpoch deregisters appropriate gossip and RPC topic if there is a fork in the current epoch.
func (s *Service) ensureRPCDeregistrationForEpoch(currentEpoch primitives.Epoch) error {
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

	return nil
}
