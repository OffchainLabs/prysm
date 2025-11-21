package sync

import (
	"context"
	"fmt"
	"sync"

	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type topicFamilyKey struct {
	topicName  string
	forkDigest [4]byte
}

func topicFamilyKeyFrom(tf GossipsubTopicFamily) topicFamilyKey {
	return topicFamilyKey{topicName: tf.Name(), forkDigest: tf.NetworkScheduleEntry().ForkDigest}
}

type GossipsubController struct {
	ctx    context.Context
	cancel context.CancelFunc

	syncService *Service
	wg          sync.WaitGroup

	mu                  sync.RWMutex
	activeTopicFamilies map[topicFamilyKey]GossipsubTopicFamily
}

func NewGossipsubController(ctx context.Context, s *Service) *GossipsubController {
	ctx, cancel := context.WithCancel(ctx)
	return &GossipsubController{
		ctx:                 ctx,
		cancel:              cancel,
		syncService:         s,
		activeTopicFamilies: make(map[topicFamilyKey]GossipsubTopicFamily),
	}
}

func (g *GossipsubController) Start() {
	currentEpoch := g.syncService.cfg.clock.CurrentEpoch()
	if err := g.syncService.waitForInitialSync(g.ctx); err != nil {
		log.WithError(err).Debug("Context cancelled while waiting for initial sync, not starting GossipsubController")
		return
	}

	g.updateActiveTopicFamilies(currentEpoch)
	g.wg.Go(func() { g.controlLoop() })

	log.Info("GossipsubController started")
}

func (g *GossipsubController) controlLoop() {
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

func (g *GossipsubController) updateActiveTopicFamilies(currentEpoch primitives.Epoch) {
	slot := g.syncService.cfg.clock.CurrentSlot()
	currentNSE := params.GetNetworkScheduleEntry(currentEpoch)

	families := TopicFamiliesForEpoch(currentEpoch, g.syncService, currentNSE)
	isForkBoundary, nextNSE := isNextEpochForkBoundary(currentEpoch)
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
			log.WithFields(logrus.Fields{
				"topicName":  key.topicName,
				"forkDigest": fmt.Sprintf("%#x", key.forkDigest),
				"epoch":      currentEpoch,
			}).Info("Registered topic family")
		}

		switch tf := existing.(type) {
		case GossipsubTopicFamilyWithDynamicSubnets:
			tf.UnsubscribeForSlot(slot)
			tf.SubscribeForSlot(slot)
		case GossipsubTopicFamilyWithoutDynamicSubnets:
			if !seen {
				tf.Subscribe()
			}
		}
	}

	// remove topic families for the previous NSE -> this is idempotent
	if beyond, previous := isOneEpochBeyondForkBoundary(currentEpoch); beyond {
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

func (g *GossipsubController) Stop() {
	g.cancel()
	g.wg.Wait()

	g.mu.Lock()
	defer g.mu.Unlock()

	for _, family := range g.activeTopicFamilies {
		family.UnsubscribeAll()
	}
}

func (g *GossipsubController) GetCurrentActiveTopics() []string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	slot := g.syncService.cfg.clock.CurrentSlot()
	var topics []string
	for _, f := range g.activeTopicFamilies {
		tfm, ok := f.(GossipsubTopicFamilyWithDynamicSubnets)
		if !ok {
			continue
		}
		topics = append(topics, tfm.TopicsToSubscribeForSlot(slot)...)
	}
	return topics
}

func (g *GossipsubController) ExtractTopics(_ context.Context, node *enode.Node) ([]string, error) {
	if node == nil {
		return nil, errors.New("enode is nil")
	}

	g.mu.RLock()
	defer g.mu.RUnlock()

	families := make([]GossipsubTopicFamilyWithDynamicSubnets, 0, len(g.activeTopicFamilies))
	for _, f := range g.activeTopicFamilies {
		if tfm, ok := f.(GossipsubTopicFamilyWithDynamicSubnets); ok {
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

func isNextEpochForkBoundary(currentEpoch primitives.Epoch) (bool, params.NetworkScheduleEntry) {
	current := params.GetNetworkScheduleEntry(currentEpoch)
	next := params.GetNetworkScheduleEntry(currentEpoch + 1)
	if current.Epoch == next.Epoch {
		return false, next // no fork in the next epoch
	}
	return true, next // there is a fork in the next epoch
}
func isOneEpochBeyondForkBoundary(currentEpoch primitives.Epoch) (bool, params.NetworkScheduleEntry) {
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
