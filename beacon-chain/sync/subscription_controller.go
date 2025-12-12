package sync

import (
	"context"
	"fmt"
	"sync"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type topicFamilyKey struct {
	topicName  string
	forkDigest [4]byte
}

func topicFamilyKeyFrom(tf TopicFamily) topicFamilyKey {
	return topicFamilyKey{topicName: tf.Name(), forkDigest: tf.NetworkScheduleEntry().ForkDigest}
}

type SubscriptionController struct {
	ctx    context.Context
	cancel context.CancelFunc

	syncService *Service
	wg          sync.WaitGroup

	mu                  sync.RWMutex
	activeTopicFamilies map[topicFamilyKey]TopicFamily
}

func NewSubscriptionController(ctx context.Context, s *Service) *SubscriptionController {
	ctx, cancel := context.WithCancel(ctx)
	return &SubscriptionController{
		ctx:                 ctx,
		cancel:              cancel,
		syncService:         s,
		activeTopicFamilies: make(map[topicFamilyKey]TopicFamily),
	}
}

func (g *SubscriptionController) Start() {
	currentEpoch := g.syncService.cfg.clock.CurrentEpoch()
	if err := g.syncService.waitForInitialSync(g.ctx); err != nil {
		log.WithError(err).Debug("Context cancelled while waiting for initial sync, not starting SubscriptionController")
		return
	}

	g.updateActiveTopicFamilies(currentEpoch)
	g.wg.Go(func() { g.controlLoop() })

	log.Info("SubscriptionController started")
}

func (g *SubscriptionController) controlLoop() {
	slotTicker := slots.NewSlotTicker(g.syncService.cfg.clock.GenesisTime(), params.BeaconConfig().SecondsPerSlot)
	defer slotTicker.Done()

	for {
		select {
		case <-slotTicker.C():
			currentEpoch := g.syncService.cfg.clock.CurrentEpoch()
			g.updateActiveTopicFamilies(currentEpoch)

		case <-g.ctx.Done():
			return
		}
	}
}

func (g *SubscriptionController) updateActiveTopicFamilies(currentEpoch primitives.Epoch) {
	slot := g.syncService.cfg.clock.CurrentSlot()
	currentNSE := params.GetNetworkScheduleEntry(currentEpoch)

	families := TopicFamiliesForEpoch(currentEpoch, g.syncService, currentNSE)
	isForkBoundary, nextNSE := isNextEpochForkBoundary(currentNSE, currentEpoch)
	if isForkBoundary {
		families = append(families, TopicFamiliesForEpoch(nextNSE.Epoch, g.syncService, nextNSE)...)
	}
	g.mu.Lock()
	defer g.mu.Unlock()

	// register topic families for the current NSE -> this is idempotent
	for _, family := range families {
		key := topicFamilyKeyFrom(family)
		existing, seen := g.activeTopicFamilies[key]
		if !seen {
			g.activeTopicFamilies[key] = family
			existing = family
		}

		switch tf := existing.(type) {
		case DynamicShardedTopicFamily:
			tf.UnsubscribeForSlot(slot)
			tf.SubscribeForSlot(slot)
		case ShardedTopicFamily:
			if !seen {
				tf.Subscribe()
			}
		}
	}

	// remove topic families for the previous NSE -> this is idempotent
	if beyond, previous := isOneEpochBeyondForkBoundary(currentNSE, currentEpoch); beyond {
		for key, family := range g.activeTopicFamilies {
			if key.forkDigest == previous.ForkDigest {

				family.UnsubscribeAll()
				delete(g.activeTopicFamilies, key)

				log.WithFields(logrus.Fields{
					"topicName":  key.topicName,
					"forkDigest": fmt.Sprintf("%#x", key.forkDigest),
				}).Info("Removed topic family")
			}
		}
	}
}

func (g *SubscriptionController) Stop() {
	g.cancel()
	g.wg.Wait()

	g.mu.Lock()
	defer g.mu.Unlock()

	for _, family := range g.activeTopicFamilies {
		family.UnsubscribeAll()
	}
}

func (g *SubscriptionController) GetCurrentActiveTopics() []string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	slot := g.syncService.cfg.clock.CurrentSlot()
	var topics []string
	for _, f := range g.activeTopicFamilies {
		tfm, ok := f.(DynamicShardedTopicFamily)
		if !ok {
			continue
		}
		topics = append(topics, tfm.TopicsToSubscribeForSlot(slot)...)
	}
	return topics
}

func (g *SubscriptionController) ExtractTopics(_ context.Context, node *enode.Node) ([]string, error) {
	if node == nil {
		return nil, errors.New("enode is nil")
	}

	g.mu.RLock()
	defer g.mu.RUnlock()

	families := make([]DynamicShardedTopicFamily, 0, len(g.activeTopicFamilies))
	for _, f := range g.activeTopicFamilies {
		if tfm, ok := f.(DynamicShardedTopicFamily); ok {
			families = append(families, tfm)
		}
	}

	// Collect topics from dynamic families only, de-duplicated.
	topicSet := make(map[string]struct{})
	for _, df := range families {
		topics, err := df.ExtractTopicsForNode(node)
		if err != nil {
			log.WithError(err).WithFields(logrus.Fields{
				"topicFamily": fmt.Sprintf("%T", df),
			}).Debug("Failed to get topics for node from family")
			continue
		}
		for _, t := range topics {
			topicSet[t] = struct{}{}
		}
	}

	out := make([]string, 0, len(topicSet))
	for t := range topicSet {
		out = append(out, t)
	}
	return out, nil
}

func isNextEpochForkBoundary(currentNSE params.NetworkScheduleEntry, currentEpoch primitives.Epoch) (bool, params.NetworkScheduleEntry) {
	nextNSE := params.GetNetworkScheduleEntry(currentEpoch + 1)
	return currentNSE.Epoch != nextNSE.Epoch, nextNSE
}

func isOneEpochBeyondForkBoundary(currentNSE params.NetworkScheduleEntry, currentEpoch primitives.Epoch) (bool, params.NetworkScheduleEntry) {
	previousNSE := params.PreviousNetworkScheduleEntry(currentNSE.Epoch)

	if currentNSE.Epoch == params.BeaconConfig().GenesisEpoch {
		return false, previousNSE
	}
	if currentEpoch < currentNSE.Epoch+1 {
		return false, previousNSE
	}
	return true, previousNSE
}
