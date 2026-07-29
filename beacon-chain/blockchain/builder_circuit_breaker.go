package blockchain

import (
	"fmt"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
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
	fc := s.cfg.ForkChoiceStore
	parentRoot := blk.ParentRoot()

	if fc.HasFullNode(parentRoot) {
		return
	}
	parentSlot, err := fc.Slot(parentRoot)
	if err != nil {
		return
	}
	if parentSlot+1 != blk.Slot() {
		return
	}
	builderIndex, err := fc.BuilderIndex(parentRoot)
	if err != nil {
		return
	}
	if builderIndex == params.BeaconConfig().BuilderIndexSelfBuild {
		return
	}
	weight, err := fc.ConsensusNodeWeight(parentRoot)
	if err != nil {
		return
	}
	committeeWeight := fc.CommitteeWeight()
	if committeeWeight == 0 {
		return
	}
	if weight*100 <= committeeWeight*params.BeaconConfig().BuilderFailureWeightThreshold {
		return
	}

	epoch := slots.ToEpoch(blk.Slot())
	if cb.RecordFailure(builderIndex, parentRoot, epoch) {
		builderPayloadFailuresTotal.Inc()
		log.WithFields(logrus.Fields{
			"builderIndex": builderIndex,
			"parentRoot":   fmt.Sprintf("%#x", parentRoot),
			"parentSlot":   parentSlot,
		}).Warn("Builder failed to reveal payload, blacklisting it")
	}
	cb.Prune(epoch)
	builderBlacklistedCount.Set(float64(cb.BlacklistedCount(epoch)))
	if cb.SelfBuildOnly(epoch) {
		builderSelfBuildOnly.Set(1)
		log.WithField("blacklistedBuilders", cb.BlacklistedCount(epoch)).
			Warn("Builder circuit breaker tripped, falling back to self-building")
	} else {
		builderSelfBuildOnly.Set(0)
	}
}
