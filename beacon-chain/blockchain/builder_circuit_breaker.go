package blockchain

import (
	"fmt"
	"strings"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/io/logs"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/sirupsen/logrus"
)

// The caller MUST hold the forkchoice write lock.
func (s *Service) checkBuilderPayloadFailure(blk interfaces.ReadOnlyBeaconBlock, st state.ReadOnlyBeaconState) {
	cb := s.cfg.BuilderCircuitBreaker
	if cb == nil || blk.Version() < version.Gloas {
		return
	}
	epoch := slots.ToEpoch(blk.Slot())
	blacklisted := s.recordBuilderPayloadFailure(cb, blk, epoch)

	// Upkeep runs on every block, not just on failures, so bans and gauges expire on time.
	cb.Prune(epoch)
	cb.DropInactiveBuilders(epoch, st.IsActiveBuilder)
	count := cb.BlacklistedCount(epoch)
	builderBlacklistedCount.Set(float64(count))
	builderRelaysBannedCount.Set(float64(cb.RelayBannedCount(epoch)))
	builderCollateralBlacklistedCount.Set(float64(cb.CollateralBlacklistedCount(epoch)))
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
	// The parent revealed no payload, so every bail out below is logged: after the fact the question
	// is always "this builder clearly failed at slot N, why was it not charged".
	entry := log.WithFields(logrus.Fields{
		"slot":       blk.Slot(),
		"parentRoot": fmt.Sprintf("%#x", parentRoot),
	})
	parentSlot, err := fc.Slot(parentRoot)
	if err != nil {
		entry.WithError(err).Debug("Not charging builder, parent slot unknown")
		return false
	}
	if parentSlot+1 != blk.Slot() {
		entry.WithField("parentSlot", parentSlot).Debug("Not charging builder, slots skipped after parent")
		return false
	}
	builderIndex, err := fc.BuilderIndex(parentRoot)
	if err != nil {
		entry.WithError(err).Debug("Not charging builder, builder index unknown")
		return false
	}
	entry = entry.WithField("builderIndex", builderIndex)
	if builderIndex == params.BeaconConfig().BuilderIndexSelfBuild {
		entry.Debug("Not charging builder, parent was self-built")
		return false
	}
	if fc.CouldBuilderWithhold(parentRoot) {
		entry.Debug("Not charging builder, parent drew too little committee support")
		return false
	}

	outcome := cb.RecordFailure(builderIndex, parentRoot, epoch)
	if !outcome.Blacklisted {
		entry.Debug("Builder not blacklisted for this failure")
		return false
	}
	builderPayloadFailuresTotal.Inc()
	fields := logrus.Fields{
		"builderIndex": builderIndex,
		"parentRoot":   fmt.Sprintf("%#x", parentRoot),
		"parentSlot":   parentSlot,
	}
	if len(outcome.BannedRelays) > 0 {
		masked := make([]string, len(outcome.BannedRelays))
		for i, r := range outcome.BannedRelays {
			masked[i] = logs.MaskCredentialsLogging(r)
		}
		fields["bannedRelays"] = strings.Join(masked, ",")
		fields["collateralBuilders"] = outcome.Collateral
	}
	log.WithFields(fields).Warn("Builder failed to reveal payload, blacklisting it")
	return true
}
