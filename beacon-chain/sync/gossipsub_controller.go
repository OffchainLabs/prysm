package sync

import (
	"context"
	"fmt"
	"sync"

	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/sirupsen/logrus"
)

type topicFamilyKey struct {
	topicName  string
	forkDigest [4]byte
}

func topicFamilyKeyFrom(tf GossipsubTopicFamily) topicFamilyKey {
	return topicFamilyKey{topicName: fmt.Sprintf("%T", tf), forkDigest: tf.NetworkScheduleEntry().ForkDigest}
}

type GossipsubController struct {
	ctx    context.Context
	cancel context.CancelFunc

	syncService *Service

	wg sync.WaitGroup

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
		if _, ok := g.activeTopicFamilies[key]; ok {
			continue
		}
		g.activeTopicFamilies[key] = family

		family.Subscribe()

		log.WithFields(logrus.Fields{
			"topicName":  key.topicName,
			"forkDigest": fmt.Sprintf("%#x", key.forkDigest),
			"epoch":      currentEpoch,
		}).Info("Registered topic family")
	}

	// remove topic families for the previous NSE -> this is idempotent
	if beyond, previous := isOneEpochBeyondForkBoundary(currentEpoch); beyond {
		for key, family := range g.activeTopicFamilies {
			if key.forkDigest == previous.ForkDigest {

				family.Unsubscribe()

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
