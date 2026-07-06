package sync

import (
	"math"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

// runLatePayloadRequest requests the head block's envelope by root each slot if it
// hasn't been seen by the payload-timeliness deadline (PayloadDueBPS).
func (s *Service) runLatePayloadRequest() {
	clock, err := s.clockWaiter.WaitForClock(s.ctx)
	if err != nil {
		log.WithError(err).Error("Failed to receive clock for late payload request routine")
		return
	}
	cfg := params.BeaconConfig()
	if cfg.GloasForkEpoch == math.MaxUint64 {
		return
	}
	offset := cfg.SlotComponentDuration(cfg.PayloadDueBPS)
	ticker := slots.NewSlotTickerWithOffset(clock.GenesisTime(), offset, cfg.SecondsPerSlot)
	defer ticker.Done()
	for {
		select {
		case slot := <-ticker.C():
			if slots.ToEpoch(slot) < cfg.GloasForkEpoch {
				continue
			}
			s.requestLatePayload(slot)
		case <-s.ctx.Done():
			log.Debug("Context closed, exiting late payload request routine")
			return
		}
	}
}

// requestLatePayload requests the current head block's envelope by root when missing.
func (s *Service) requestLatePayload(slot primitives.Slot) {
	if !s.chainIsStarted() {
		return
	}
	if s.cfg.initialSync.Syncing() {
		return
	}
	if slot != s.cfg.chain.HeadSlot() {
		return
	}
	headRoot, err := s.cfg.chain.HeadRoot(s.ctx)
	if err != nil {
		log.WithError(err).Debug("Could not get head root for late payload request")
		return
	}
	// requestPayloadEnvelope short-circuits if already present and dedups in-flight requests.
	go s.requestPayloadEnvelope([32]byte(headRoot))
}
