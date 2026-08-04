package blockchain

import (
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/sirupsen/logrus"
)

// The caller MUST hold the forkchoice write lock.
func (s *Service) checkBuilderPayloadFailure(blk interfaces.ReadOnlyBeaconBlock) {
	cb := s.cfg.BuilderCircuitBreaker
	if cb == nil || blk.Version() < version.Gloas {
		return
	}
	epoch := slots.ToEpoch(blk.Slot())
	blacklisted := s.recordBuilderPayloadFailure(cb, blk, epoch)

	// Pruning and the gauges below run on every block: driving them from the failure path alone
	// leaves them reporting the last failure epoch long after the bans have expired.
	cb.Prune(epoch)
	count := cb.BlacklistedCount(epoch)
	builderBlacklistedCount.Set(float64(count))
	if !cb.SelfBuildOnly(epoch) {
		builderSelfBuildOnly.Set(0)
		return
	}
	builderSelfBuildOnly.Set(1)
	if blacklisted {
		log.WithField("blacklistedBuilders", count).
			Warn("Builder circuit breaker tripped, falling back to self-building")
	}
}

// recordBuilderPayloadFailure charges the parent's builder for a payload it never revealed and
// reports whether that blacklisted it.
func (s *Service) recordBuilderPayloadFailure(
	cb *cache.BuilderCircuitBreaker,
	blk interfaces.ReadOnlyBeaconBlock,
	epoch primitives.Epoch,
) bool {
	fc := s.cfg.ForkChoiceStore
	parentRoot := blk.ParentRoot()

	if fc.HasFullNode(parentRoot) {
		return false
	}
	parentSlot, err := fc.Slot(parentRoot)
	if err != nil {
		return false
	}
	if parentSlot+1 != blk.Slot() {
		return false
	}
	builderIndex, err := fc.BuilderIndex(parentRoot)
	if err != nil {
		return false
	}
	if builderIndex == params.BeaconConfig().BuilderIndexSelfBuild {
		return false
	}
	weight, err := fc.ConsensusNodeWeight(parentRoot)
	if err != nil {
		return false
	}
	committeeWeight := fc.CommitteeWeight()
	if committeeWeight == 0 {
		return false
	}
	if weight*100 <= committeeWeight*params.BeaconConfig().BuilderFailureWeightThreshold {
		return false
	}

	if !cb.RecordFailure(builderIndex, parentRoot, epoch) {
		return false
	}
	builderPayloadFailuresTotal.Inc()
	log.WithFields(logrus.Fields{
		"builderIndex": builderIndex,
		"parentRoot":   fmt.Sprintf("%#x", parentRoot),
		"parentSlot":   parentSlot,
	}).Warn("Builder failed to reveal payload, blacklisting it")
	return true
}
