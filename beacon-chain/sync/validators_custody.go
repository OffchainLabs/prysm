package sync

import (
	"strings"
	"time"

	"github.com/OffchainLabs/prysm/v6/async"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
)

const validatorCustodyPeriod = 1 * time.Minute

func (s *Service) maintainValidatorsCustody() {
	async.RunEvery(s.ctx, validatorCustodyPeriod, func() {
		s.setTargetValidatorsCustodyRequirement()
		s.updateToAdvertiseCustodyGroupCount()
	})
}

// setTargetValidatorsCustodyRequirement sets the target validators custody requirement according to the head state.
func (s *Service) setTargetValidatorsCustodyRequirement() {
	// Get the indices of the tracked validators.
	indices := s.trackedValidatorsCache.Indices()

	// Write lock custody group count.
	s.cfg.custodyInfo.Mut.Lock()
	defer s.cfg.custodyInfo.Mut.Unlock()

	// Set the validators custody requirement if there are no tracked validators.
	if len(indices) == 0 {
		s.cfg.custodyInfo.TargetGroupCount.SetValidatorsCustodyRequirement(0)
		return
	}

	// Retrieve the head state.
	finalizedState := s.cfg.stateGen.FinalizedState()
	if finalizedState == nil || finalizedState.IsNil() {
		log.Error("Finalized state is nil")
		return
	}

	// Get the validators custody requirement.
	validatorsCustodyRequirement, err := peerdas.ValidatorsCustodyRequirement(finalizedState, indices)
	if err != nil {
		log.WithError(err).Error("Failed to get validators custody requirement")
		return
	}

	// Set the validators custody requirement.
	s.cfg.custodyInfo.TargetGroupCount.SetValidatorsCustodyRequirement(validatorsCustodyRequirement)
}

// updateToAdvertiseCustodyGroupCount updates the custody group count to advertise.
func (s *Service) updateToAdvertiseCustodyGroupCount() {
	// TODO: Add something here where correct conditions (all good subscriptions, enough peers?) are met AND valid before FULU
	// Retrieve the registered topics, and store them in a map for quick lookup.
	registeredTopicsSlice := s.subHandler.allTopics()
	registeredTopics := make(map[string]bool, len(registeredTopicsSlice))

	for _, topic := range registeredTopicsSlice {
		topicMessage := extractGossipMessage(topic)
		registeredTopics[topicMessage] = true
	}

	// Get the node ID.
	// nodeID := s.cfg.p2p.NodeID()

	s.cfg.custodyInfo.Mut.Lock()
	defer s.cfg.custodyInfo.Mut.Unlock()

	// Get the custody group count.
	targetCustodyGroupCount := s.cfg.custodyInfo.TargetGroupCount.Get()

	// Get the peerDAS info.
	// info, _, err := peerdas.Info(nodeID, targetCustodyGroupCount)
	// if err != nil {
	// 	log.WithError(err).Error("Failed to get peerDAS info")
	// 	return
	// }

	// for column := range info.CustodyColumns {
	// 	topicMessage := fmt.Sprintf(p2p.GossipDataColumnSidecarMessage+"_%d", column)
	// 	if !registeredTopics[topicMessage] {
	// 		// At least one data column subnet we should be subscribed to is not.
	// 		return
	// 	}
	// }

	// All data column subnets we should be subscribed to are.
	s.cfg.custodyInfo.ToAdvertiseGroupCount.Set(targetCustodyGroupCount)
}

// extractGossipMessage extracts the gossip data column sidecar message from a topic.
func extractGossipMessage(s string) string {
	parts := strings.SplitN(s, "/", 5)

	if len(parts) < 4 {
		return ""
	}

	return parts[3]
}
