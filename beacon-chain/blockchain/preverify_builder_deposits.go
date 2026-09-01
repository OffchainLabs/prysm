package blockchain

import (
	"context"
	"errors"
	"math"
	"time"

	coregloas "github.com/OffchainLabs/prysm/v7/beacon-chain/core/gloas"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/sirupsen/logrus"
)

const builderDepositPreverifyLimitBPS = primitives.BP(2000)

func (s *Service) runPreGloasBuilderDepositTasks() {
	if err := s.waitForSync(); err != nil {
		log.WithError(err).Error("Failed to wait for initial sync")
		return
	}
	cfg := params.BeaconConfig()
	if cfg.GloasForkEpoch == math.MaxUint64 {
		return
	}
	startEpoch := primitives.Epoch(0)
	if cfg.GloasForkEpoch > coregloas.GloasPreverifyWindowEpochs {
		startEpoch = cfg.GloasForkEpoch - coregloas.GloasPreverifyWindowEpochs
	}
	if err := s.waitUntilEpoch(startEpoch, cfg.SlotDuration()); err != nil {
		return
	}

	offset := cfg.SlotComponentDuration(cfg.AggregateDueBPS)
	ticker := slots.NewSlotTickerWithOffset(s.genesisTime, offset, cfg.SlotDuration())
	defer ticker.Done()
	for {
		select {
		case slot := <-ticker.C():
			if slots.ToEpoch(slot) >= cfg.GloasForkEpoch {
				return
			}
			if slots.IsEpochEnd(slot) {
				continue
			}
			s.preverifyBuilderDeposits(s.ctx, slot, cfg.SlotComponentDuration(builderDepositPreverifyLimitBPS))
		case <-s.ctx.Done():
			return
		}
	}
}

func (s *Service) preverifyBuilderDeposits(ctx context.Context, slot primitives.Slot, limit time.Duration) {
	if !s.inRegularSync() {
		return
	}
	st, err := s.HeadState(ctx)
	if err != nil {
		log.WithError(err).Debug("Could not get head state to pre-verify builder deposits")
		return
	}
	if st.Version() != version.Fulu {
		return
	}
	proposingSlot := slot + 1
	stateEpoch := slots.ToEpoch(st.Slot())
	proposingEpoch := slots.ToEpoch(proposingSlot)
	if stateEpoch < proposingEpoch && proposingEpoch-stateEpoch > 1 {
		return
	}
	proposer, err := s.checkIfProposing(st, proposingSlot)
	if err != nil || proposer != nil {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	started := time.Now()
	result, err := coregloas.PreverifyBuilderDeposits(ctx, st, coregloas.MaxBuilderDepositsPerSlot)
	builderDepositPreverifyDuration.Observe(time.Since(started).Seconds())
	builderDepositPreverifyPending.Set(float64(result.PendingDeposits))
	builderDepositPreverifyCached.Set(float64(result.CachedDeposits))
	builderDepositPreverifyScanned.Set(float64(result.ScannedPendingDeposits))
	builderDepositPreverifyBuilderPubkeys.Set(float64(result.BuilderPubkeys))
	builderDepositPreverifySignatures.WithLabelValues("builder", "valid").Add(float64(result.ValidBuilderSignatures))
	builderDepositPreverifySignatures.WithLabelValues("builder", "invalid").Add(float64(result.InvalidBuilderSignatures))
	builderDepositPreverifySignatures.WithLabelValues("validator", "valid").Add(float64(result.ValidValidatorSignatures))
	builderDepositPreverifySignatures.WithLabelValues("validator", "invalid").Add(float64(result.InvalidValidatorSignatures))

	fields := logrus.Fields{
		"slot":                       slot,
		"pendingDeposits":            result.PendingDeposits,
		"scannedPendingDeposits":     result.ScannedPendingDeposits,
		"cachedDeposits":             result.CachedDeposits,
		"builderPubkeys":             result.BuilderPubkeys,
		"validBuilderSignatures":     result.ValidBuilderSignatures,
		"invalidBuilderSignatures":   result.InvalidBuilderSignatures,
		"validValidatorSignatures":   result.ValidValidatorSignatures,
		"invalidValidatorSignatures": result.InvalidValidatorSignatures,
	}
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			log.WithFields(fields).Debug("Builder deposit pre-verification reached its slot budget")
			return
		}
		log.WithFields(fields).WithError(err).Warn("Could not pre-verify builder deposits")
		return
	}
	log.WithFields(fields).Debug("Pre-verified builder deposits before Gloas")
}
