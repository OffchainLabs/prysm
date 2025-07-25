package sync

import (
	"strings"
	"time"

	"github.com/OffchainLabs/prysm/v6/async"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v6/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var nilFinalizedStateError = errors.New("finalized state is nil")

// validatorsCustodyRequirements computes the custody requirements based on the
// finalized state and the tracked validators.
func (s *Service) validatorsCustodyRequirement() (uint64, error) {
	// Get the indices of the tracked validators.
	indices := s.trackedValidatorsCache.Indices()

	// Return early if no validators are tracked.
	if len(indices) == 0 {
		return 0, nil
	}

	// Retrieve the finalized state.
	finalizedState := s.cfg.stateGen.FinalizedState()
	if finalizedState == nil || finalizedState.IsNil() {
		return 0, nilFinalizedStateError
	}

	// Compute the validators custody requirements.
	result, err := peerdas.ValidatorsCustodyRequirement(finalizedState, indices)
	if err != nil {
		return 0, errors.Wrap(err, "validators custody requirements")
	}

	return result, nil
}

// custodyGroupCount computes the custody group count based on the custody requirement,
// the validators custody requirement, and whether the node is subscribed to all data subnets.
func (s *Service) custodyGroupCount() (uint64, error) {
	beaconConfig := params.BeaconConfig()

	if flags.Get().SubscribeAllDataSubnets {
		return beaconConfig.NumberOfCustodyGroups, nil
	}

	validatorsCustodyRequirement, err := s.validatorsCustodyRequirement()
	if err != nil {
		return 0, errors.Wrap(err, "validators custody requirement")
	}

	return max(beaconConfig.CustodyRequirement, validatorsCustodyRequirement), nil
}

func (s *Service) maintainCustodyGroupCount() {
	const (
		interval         = 1 * time.Minute
		minimumPeerCount = 1
	)

	async.RunEvery(s.ctx, interval, func() {
		custodyGroupCount, err := s.custodyGroupCount()
		if err != nil {
			log.WithError(err).Error("Could not compute custody group count")
			return
		}

		// Check that all subscribed data column sidecars topics have at least `minimumPeerCount` peers.
		topics := s.cfg.p2p.PubSub().GetTopics()
		enoughPeers := true
		for _, topic := range topics {
			if !strings.Contains(topic, p2p.GossipDataColumnSidecarMessage) {
				continue
			}

			if peers := s.cfg.p2p.PubSub().ListPeers(topic); len(peers) < minimumPeerCount {
				// If a topic has fewer than the minimum required peers, log a warning.
				log.WithFields(logrus.Fields{
					"topic":            topic,
					"peerCount":        len(peers),
					"minimumPeerCount": minimumPeerCount,
				}).Debug("Insufficient peers for data column sidecar topic to maintain custody count")
				enoughPeers = false
			}
		}

		if !enoughPeers {
			return
		}

		headROBlock, err := s.cfg.chain.HeadBlock(s.ctx)
		if err != nil {
			log.WithError(err).Error("Could not retrieve head block")
			return
		}
		headSlot := headROBlock.Block().Slot()

		earliestSlot, storedGroupCount, err := s.cfg.p2p.UpdateCustodyInfo(headSlot, custodyGroupCount)
		if err != nil {
			log.WithError(err).Error("Could not update custody info")
			return
		}

		if storedGroupCount >= custodyGroupCount {
			return
		}

		// Update the custody group count in the P2P service and the database.
		if _, _, err := s.cfg.p2p.UpdateCustodyInfo(earliestSlot, storedGroupCount); err != nil {
			log.WithError(err).Error("Could not update custody info")
			return
		}

		if err := s.cfg.beaconDB.SaveCustodyGroupCount(s.ctx, custodyGroupCount); err != nil {
			log.WithError(err).Error("Could not save custody group count")
			return
		}
	})
}
