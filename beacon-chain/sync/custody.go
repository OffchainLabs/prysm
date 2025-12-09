package sync

import (
	"context"
	"strings"
	"time"

	"github.com/OffchainLabs/prysm/v7/async"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var nilFinalizedStateError = errors.New("finalized state is nil")

func (s *Service) maintainCustodyInfo() {
	const interval = 1 * time.Minute

	async.RunEvery(s.ctx, interval, func() {
		if err := s.updateCustodyInfoIfNeeded(); err != nil {
			log.WithError(err).Error("Failed to update custody info")
		}
	})
}

func (s *Service) updateCustodyInfoIfNeeded() error {
	const minimumPeerCount = 1

	// Get our actual custody group count.
	actualCustodyGrounpCount, err := s.cfg.p2p.CustodyGroupCount(s.ctx)
	if err != nil {
		return errors.Wrap(err, "p2p custody group count")
	}

	// Get our target custody group count.
	targetCustodyGroupCount, err := s.custodyGroupCount(s.ctx)
	if err != nil {
		return errors.Wrap(err, "custody group count")
	}

	// If the actual custody group count is already equal to the target, skip the update.
	if actualCustodyGrounpCount >= targetCustodyGroupCount {
		return nil
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
		return nil
	}

	headROBlock, err := s.cfg.chain.HeadBlock(s.ctx)
	if err != nil {
		return errors.Wrap(err, "head block")
	}
	headSlot := headROBlock.Block().Slot()

	storedEarliestSlot, storedGroupCount, err := s.cfg.p2p.UpdateCustodyInfo(headSlot, targetCustodyGroupCount)
	if err != nil {
		return errors.Wrap(err, "p2p update custody info")
	}

	if _, _, err := s.cfg.beaconDB.UpdateCustodyInfo(s.ctx, storedEarliestSlot, storedGroupCount); err != nil {
		return errors.Wrap(err, "beacon db update custody info")
	}

	return nil
}

// custodyGroupCount computes the custody group count based on the custody requirement,
// the validators custody requirement, and whether the node is subscribed to all data subnets.
func (s *Service) custodyGroupCount(context.Context) (uint64, error) {
	cfg := params.BeaconConfig()

	if flags.Get().Supernode {
		return cfg.NumberOfCustodyGroups, nil
	}

	// Calculate validator custody requirements
	validatorsCustodyRequirement, err := s.validatorsCustodyRequirement()
	if err != nil {
		return 0, errors.Wrap(err, "validators custody requirement")
	}

	effectiveCustodyRequirement := max(cfg.CustodyRequirement, validatorsCustodyRequirement)

	// If we're not in semi-supernode mode, just use the effective requirement.
	if !flags.Get().SemiSupernode {
		return effectiveCustodyRequirement, nil
	}

	// Semi-supernode mode custodies the minimum custody groups required for reconstruction.
	// This is future-proof and works correctly even if custody groups != columns.
	semiSupernodeTarget, err := peerdas.MinimumCustodyGroupCountToReconstruct()
	if err != nil {
		return 0, errors.Wrap(err, "minimum custody group count")
	}
	return max(effectiveCustodyRequirement, semiSupernodeTarget), nil
}

// validatorsCustodyRequirements computes the custody requirements based on the
// finalized state and the tracked validators.
func (s *Service) validatorsCustodyRequirement() (uint64, error) {
	if s.trackedValidatorsCache == nil {
		return 0, nil
	}
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
