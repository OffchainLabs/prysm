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
	const gracePeriodSeconds = 300 // 300-second grace period for CGC increases

	// Get our actual custody group count.
	actualCustodyGrounpCount, err := s.cfg.p2p.CustodyGroupCount(s.ctx)
	if err != nil {
		return errors.Wrap(err, "p2p custody group count")
	}

	// Update the P2P custody group count metric
	custodyGroupCountP2P.Set(float64(actualCustodyGrounpCount))

	// Get our target custody group count.
	targetCustodyGroupCount, err := s.custodyGroupCount(s.ctx)
	if err != nil {
		return errors.Wrap(err, "custody group count")
	}

	// Handle pending CGC changes with proper grace period
	s.pendingCGCLock.Lock()
	now := time.Now()

	switch {
	case s.pendingCGC > 0 && now.After(s.pendingCGCDeadline):
		// Grace period expired - check if pending change is still valid
		targetToApply := s.pendingCGC
		s.pendingCGC = 0 // Clear the pending change
		s.pendingCGCDeadline = time.Time{}
		s.pendingCGCLock.Unlock()

		// Only apply the pending change if current target still justifies it
		// This prevents applying stale increases when validators have been removed
		// or configuration has changed during the grace period
		if targetToApply <= targetCustodyGroupCount {
			// Pending value is still valid (at or below current target)
			// the network still wants at least that many groups
			// Use the current target to allow for any increases that happened during grace period
			if targetCustodyGroupCount > actualCustodyGrounpCount {
				log.WithFields(logrus.Fields{
					"previousCGC": actualCustodyGrounpCount,
					"newCGC":      targetCustodyGroupCount,
					"pendingCGC":  targetToApply,
				}).Info("Applying custody group count increase after grace period")
			}
		} else {
			// Pending value is higher than current target - drop it as stale
			log.WithFields(logrus.Fields{
				"currentCGC":    actualCustodyGrounpCount,
				"targetCGC":     targetCustodyGroupCount,
				"stalePendingCGC": targetToApply,
			}).Info("Dropping stale pending CGC increase as target has decreased")

			// Still check if current target needs an increase (with new grace period)
			if targetCustodyGroupCount > actualCustodyGrounpCount {
				// Re-schedule with current target and new grace period
				s.pendingCGCLock.Lock()
				s.pendingCGC = targetCustodyGroupCount
				s.pendingCGCDeadline = now.Add(time.Duration(gracePeriodSeconds) * time.Second)
				s.pendingCGCLock.Unlock()

				log.WithFields(logrus.Fields{
					"currentCGC":  actualCustodyGrounpCount,
					"targetCGC":   targetCustodyGroupCount,
					"gracePeriod": gracePeriodSeconds,
				}).Info("Re-scheduling CGC increase with updated target")

				return nil
			}
		}

	case s.pendingCGC > 0 && !now.After(s.pendingCGCDeadline):
		// Pending change exists but grace period not expired - do nothing
		pending := s.pendingCGC
		timeRemaining := s.pendingCGCDeadline.Sub(now).Seconds()
		s.pendingCGCLock.Unlock()

		log.WithFields(logrus.Fields{
			"pendingCGC":    pending,
			"timeRemaining": timeRemaining,
		}).Debug("Grace period still active, skipping CGC update")

		return nil

	default:
		// No pending change: check if we need to schedule one
		if targetCustodyGroupCount > actualCustodyGrounpCount {
			// Schedule the increase with grace period
			s.pendingCGC = targetCustodyGroupCount
			s.pendingCGCDeadline = now.Add(time.Duration(gracePeriodSeconds) * time.Second)
			s.pendingCGCLock.Unlock()

			log.WithFields(logrus.Fields{
				"currentCGC":    actualCustodyGrounpCount,
				"targetCGC":     targetCustodyGroupCount,
				"gracePeriod":   gracePeriodSeconds,
				"effectiveTime": s.pendingCGCDeadline.Format(time.RFC3339),
			}).Info("Scheduling custody group count increase with grace period")

			return nil
		}
		// No change needed
		s.pendingCGCLock.Unlock()
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

	// Update the p2p earliest available slot metric
	earliestAvailableSlotP2P.Set(float64(storedEarliestSlot))

	dbEarliestSlot, dbStoredGroupCount, err := s.cfg.beaconDB.UpdateCustodyInfo(s.ctx, storedEarliestSlot, storedGroupCount)
	if err != nil {
		return errors.Wrap(err, "beacon db update custody info")
	}

	// Update the DB earliest available slot metric
	earliestAvailableSlotDB.Set(float64(dbEarliestSlot))

	// Update both custody group count metrics with their respective values
	custodyGroupCountP2P.Set(float64(storedGroupCount))
	custodyGroupCountDB.Set(float64(dbStoredGroupCount))

	return nil
}

// custodyGroupCount computes the custody group count based on the custody requirement,
// the validators custody requirement, and whether the node is subscribed to all data subnets.
func (s *Service) custodyGroupCount(context.Context) (uint64, error) {
	cfg := params.BeaconConfig()

	if flags.Get().SubscribeAllDataSubnets {
		return cfg.NumberOfCustodyGroups, nil
	}

	validatorsCustodyRequirement, err := s.validatorsCustodyRequirement()
	if err != nil {
		return 0, errors.Wrap(err, "validators custody requirement")
	}

	return max(cfg.CustodyRequirement, validatorsCustodyRequirement), nil
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
