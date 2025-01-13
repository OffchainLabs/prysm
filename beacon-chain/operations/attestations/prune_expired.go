package attestations

import (
	"time"

	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	prysmTime "github.com/prysmaticlabs/prysm/v5/time"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
)

// pruneExpired prunes attestations pool on every slot interval.
func (s *Service) pruneExpired() {
	ticker := time.NewTicker(s.cfg.pruneInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.pruneExpiredAtts()
			s.updateMetrics()
		case <-s.ctx.Done():
			log.Debug("Context closed, exiting routine")
			return
		}
	}
}

// pruneExpiredExperimental prunes attestations on every prune interval.
func (s *Service) pruneExpiredExperimental() {
	ticker := time.NewTicker(s.cfg.pruneInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			expirySlot, err := s.expirySlot()
			if err != nil {
				log.WithError(err).Error("Could not get expiry slot")
				continue
			}
			numExpired := s.cfg.Cache.PruneBefore(expirySlot)
			s.updateMetricsExperimental(numExpired)
		case <-s.ctx.Done():
			log.Debug("Context closed, exiting routine")
			return
		}
	}
}

// This prunes expired attestations from the pool.
func (s *Service) pruneExpiredAtts() {
	aggregatedAtts := s.cfg.Pool.AggregatedAttestations()
	for _, att := range aggregatedAtts {
		if s.expired(att.GetData().Slot) {
			if err := s.cfg.Pool.DeleteAggregatedAttestation(att); err != nil {
				log.WithError(err).Error("Could not delete expired aggregated attestation")
			}
			expiredAggregatedAtts.Inc()
		}
	}

	if _, err := s.cfg.Pool.DeleteSeenUnaggregatedAttestations(); err != nil {
		log.WithError(err).Error("Cannot delete seen attestations")
	}
	unAggregatedAtts, err := s.cfg.Pool.UnaggregatedAttestations()
	if err != nil {
		log.WithError(err).Error("Could not get unaggregated attestations")
		return
	}
	for _, att := range unAggregatedAtts {
		if s.expired(att.GetData().Slot) {
			if err := s.cfg.Pool.DeleteUnaggregatedAttestation(att); err != nil {
				log.WithError(err).Error("Could not delete expired unaggregated attestation")
			}
			expiredUnaggregatedAtts.Inc()
		}
	}

	blockAtts := s.cfg.Pool.BlockAttestations()
	for _, att := range blockAtts {
		if s.expired(att.GetData().Slot) {
			if err := s.cfg.Pool.DeleteBlockAttestation(att); err != nil {
				log.WithError(err).Error("Could not delete expired block attestation")
			}
			expiredBlockAtts.Inc()
		}
	}
}

// Return true if the input slot has been expired.
// Expired is defined as one epoch behind than current time.
func (s *Service) expired(providedSlot primitives.Slot) bool {
	providedEpoch := slots.ToEpoch(providedSlot)
	currSlot := slots.CurrentSlot(s.genesisTime)
	currEpoch := slots.ToEpoch(currSlot)
	if currEpoch < params.BeaconConfig().DenebForkEpoch {
		return s.expiredPreDeneb(providedSlot)
	}
	return providedEpoch+1 < currEpoch
}

// Handles expiration of attestations before deneb.
func (s *Service) expiredPreDeneb(slot primitives.Slot) bool {
	expirationSlot := slot + params.BeaconConfig().SlotsPerEpoch
	expirationTime := s.genesisTime + uint64(expirationSlot.Mul(params.BeaconConfig().SecondsPerSlot))
	currentTime := uint64(prysmTime.Now().Unix())
	return currentTime >= expirationTime
}

// Attestations for a slot before the returned slot are considered expired.
func (s *Service) expirySlot() (primitives.Slot, error) {
	currSlot := slots.CurrentSlot(s.genesisTime)
	currEpoch := slots.ToEpoch(currSlot)
	if currEpoch == 0 {
		return 0, nil
	}
	if currEpoch < params.BeaconConfig().DenebForkEpoch {
		// Safe to subtract because we exited early for epoch 0.
		return currSlot - 31, nil
	}
	return slots.EpochStart(currEpoch - 1)
}
