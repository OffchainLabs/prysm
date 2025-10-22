package sync

import (
	"context"
	"fmt"
	"sync"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// topicFamilyKey uniquely identifies a topic family by its topic format and fork digest.
type topicFamilyKey struct {
	topicFormat string  // e.g., p2p.AttestationSubnetTopicFormat
	digest      [4]byte // Fork digest
}

// topicFamilyKeyFrom derives a stable key for a TopicFamily based on
// its topic format and fork digest.
func topicFamilyKeyFrom(tf TopicFamily) topicFamilyKey {
	return topicFamilyKey{topicFormat: tf.TopicFormat(), digest: tf.ForkDigest()}
}

// GossipTopicManager manages gossip topic families for the sync service.
// It maintains a mapping of active topic families and provides topic extraction
// functionality for the peer crawler. It also contains the control loop
// that manages topic subscriptions across epochs and forks.
type GossipTopicManager struct {
	ctx         context.Context
	cancel      context.CancelFunc
	syncService *Service

	wg sync.WaitGroup

	mu                  sync.RWMutex
	activeTopicFamilies map[topicFamilyKey]TopicFamily
}

// NewGossipTopicManager creates a new GossipTopicManager instance.
func NewGossipTopicManager(ctx context.Context, s *Service) *GossipTopicManager {
	ctx, cancel := context.WithCancel(ctx)
	return &GossipTopicManager{
		ctx:                 ctx,
		cancel:              cancel,
		syncService:         s,
		activeTopicFamilies: make(map[topicFamilyKey]TopicFamily),
	}
}

// Start begins the gossip topic manager's control loop.
func (g *GossipTopicManager) Start() {
	// Wait for chain start before beginning operations
	g.syncService.waitForChainStart()

	currentEpoch := g.syncService.cfg.clock.CurrentEpoch()
	// Register initial subscribers based on current epoch
	startEntry := params.GetNetworkScheduleEntry(currentEpoch)
	g.syncService.registerSubscribers(startEntry)

	// Initialize topic families we'll be tracking
	g.updateTopicFamilies(currentEpoch)

	// Start the control loop
	g.wg.Add(1)
	go g.controlLoop()

	log.Info("GossipTopicManager started")
}

// Stop gracefully stops the gossip topic manager.
func (g *GossipTopicManager) Stop() {
	g.cancel()
	g.wg.Wait()
	log.Info("GossipTopicManager stopped")
}

// controlLoop is the main control loop that manages topic subscriptions and updates.
func (g *GossipTopicManager) controlLoop() {
	slotTicker := slots.NewSlotTicker(g.syncService.cfg.clock.GenesisTime(), params.BeaconConfig().SecondsPerSlot)
	defer slotTicker.Done()
	defer g.wg.Done()

	for {
		select {
		case <-slotTicker.C():
			current := g.syncService.cfg.clock.CurrentEpoch()

			// Ensure registrations for current and possibly next epoch if the next epoch is a fork boundary
			if err := g.ensureRegistrationsForEpoch(current); err != nil {
				log.WithError(err).Error("Unable to ensure registrations for current and possibly next epoch")
			}

			// Ensure deregistrations for old epochs and forks
			if err := g.ensureDeregistrationForEpoch(current); err != nil {
				log.WithError(err).Error("Unable to ensure deregistrations for old epochs")
			}

			// Update topic families for the current epoch
			g.updateTopicFamilies(current)

		case <-g.ctx.Done():
			log.Debug("GossipTopicManager control loop exiting")
			return
		}
	}
}

// ExtractTopics is the callback function used by the crawler to determine which topics
// a discovered node supports based on its advertised subnets.
func (g *GossipTopicManager) ExtractTopics(ctx context.Context, node *enode.Node) ([]string, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var topics []string

	for _, tf := range g.activeTopicFamilies {
		ts, err := tf.GetTopicsForNode(node)
		if err != nil {
			log.WithError(err).WithField("topicFormat", tf.TopicFormat()).
				Trace("Failed to compute topics for node")
			continue
		}
		topics = append(topics, ts...)
	}

	return topics, nil
}

func (g *GossipTopicManager) updateTopicFamilies(currentEpoch primitives.Epoch) {
	currentNSE := params.GetNetworkScheduleEntry(currentEpoch)

	families := TopicFamiliesForEpoch(currentEpoch, g.syncService, currentNSE.ForkDigest)
	isForkBoundary, nextNSE := g.isNextEpochForkBoundary(currentEpoch)
	if isForkBoundary {
		families = append(families, TopicFamiliesForEpoch(nextNSE.Epoch, g.syncService, nextNSE.ForkDigest)...)
	}
	g.mu.Lock()
	defer g.mu.Unlock()

	// register topic familieis for the current NSE -> this is idempotent
	for _, family := range families {
		key := topicFamilyKeyFrom(family)
		if _, ok := g.activeTopicFamilies[key]; ok {
			continue
		}
		g.activeTopicFamilies[key] = family
		log.WithFields(logrus.Fields{
			"topicFormat": family.TopicFormat(),
			"digest":      fmt.Sprintf("%#x", family.ForkDigest()),
			"epoch":       currentEpoch,
		}).Info("Registered topic family")
	}

	// remove topic families for the previous NSE -> this is idempotent
	if beyond, previous := g.isOneEpochBeyonForkBoundary(currentEpoch); beyond {
		for key := range g.activeTopicFamilies {
			if key.digest == previous.ForkDigest {
				delete(g.activeTopicFamilies, key)
				log.WithFields(logrus.Fields{
					"topicFormat": key.topicFormat,
					"digest":      fmt.Sprintf("%#x", key.digest),
				}).Info("Removed topic family")
			}
		}
	}
}

// ensureRegistrationsForEpoch ensures that gossip topic and RPC stream handler
// registrations are in place for the current and subsequent epoch.
// Moved from fork_watcher.go
func (g *GossipTopicManager) ensureRegistrationsForEpoch(epoch primitives.Epoch) error {
	current := params.GetNetworkScheduleEntry(epoch)
	g.syncService.registerSubscribers(current)

	currentHandler, err := g.syncService.rpcHandlerByTopicFromFork(current.VersionEnum)
	if err != nil {
		return errors.Wrap(err, "RPC handler by topic from before fork epoch")
	}
	if !g.syncService.digestActionDone(current.ForkDigest, registerRpcOnce) {
		for topic, handler := range currentHandler {
			g.syncService.registerRPC(topic, handler)
		}
	}

	isForkBoundary, next := g.isNextEpochForkBoundary(epoch)
	if !isForkBoundary {
		return nil // no fork boundary in the next epoch
	}
	g.syncService.registerSubscribers(next)

	if g.syncService.digestActionDone(next.ForkDigest, registerRpcOnce) {
		return nil
	}

	nextHandler, err := g.syncService.rpcHandlerByTopicFromFork(next.VersionEnum)
	if err != nil {
		return errors.Wrap(err, "RPC handler by topic from fork epoch")
	}
	// Compute newly added topics.
	newHandlersByTopic := addedRPCHandlerByTopic(currentHandler, nextHandler)
	// Register the new RPC handlers.
	// We deregister the old topics later, at least one epoch after the fork.
	for topic, handler := range newHandlersByTopic {
		g.syncService.registerRPC(topic, handler)
	}

	return nil
}

// ensureDeregistrationForEpoch deregisters appropriate gossip and RPC topic if there is a fork in the current epoch.
// Moved from fork_watcher.go
func (g *GossipTopicManager) ensureDeregistrationForEpoch(currentEpoch primitives.Epoch) error {
	current := params.GetNetworkScheduleEntry(currentEpoch)

	beyond, previous := g.isOneEpochBeyonForkBoundary(currentEpoch)
	if !beyond {
		return nil // no fork boundary in the previous epoch
	}

	// Remove stream handlers for all topics that are in the set of
	// currentTopics-previousTopics
	if !g.syncService.digestActionDone(previous.ForkDigest, unregisterRpcOnce) {
		previousTopics, err := g.syncService.rpcHandlerByTopicFromFork(previous.VersionEnum)
		if err != nil {
			return errors.Wrap(err, "RPC handler by topic from before fork epoch")
		}
		currentTopics, err := g.syncService.rpcHandlerByTopicFromFork(current.VersionEnum)
		if err != nil {
			return errors.Wrap(err, "RPC handler by topic from fork epoch")
		}
		topicsToRemove := removedRPCTopics(previousTopics, currentTopics)
		for topic := range topicsToRemove {
			fullTopic := topic + g.syncService.cfg.p2p.Encoding().ProtocolSuffix()
			g.syncService.cfg.p2p.Host().RemoveStreamHandler(protocol.ID(fullTopic))
			log.WithField("topic", fullTopic).Debug("Removed RPC handler")
		}
	}

	// Unsubscribe from all gossip topics with the previous fork digest.
	if g.syncService.digestActionDone(previous.ForkDigest, unregisterGossipOnce) {
		return nil
	}
	for _, t := range g.syncService.subHandler.allTopics() {
		retDigest, err := p2p.ExtractGossipDigest(t)
		if err != nil {
			log.WithError(err).Error("Could not retrieve digest")
			continue
		}
		if retDigest == previous.ForkDigest {
			g.syncService.unSubscribeFromTopic(t)
		}
	}

	return nil
}

func (g *GossipTopicManager) isNextEpochForkBoundary(currentEpoch primitives.Epoch) (bool, params.NetworkScheduleEntry) {
	current := params.GetNetworkScheduleEntry(currentEpoch)
	next := params.GetNetworkScheduleEntry(currentEpoch + 1)
	if current.Epoch == next.Epoch {
		return false, next // no fork in the next epoch
	}
	return true, next // there is a fork in the next epoch
}

func (g *GossipTopicManager) isOneEpochBeyonForkBoundary(currentEpoch primitives.Epoch) (bool, params.NetworkScheduleEntry) {
	current := params.GetNetworkScheduleEntry(currentEpoch)
	previous := params.GetNetworkScheduleEntry(current.Epoch - 1)

	if current.Epoch == params.BeaconConfig().GenesisEpoch {
		return false, previous
	}
	if currentEpoch < current.Epoch+1 {
		return false, previous
	}
	return true, previous
}
