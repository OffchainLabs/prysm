package sync

import (
	"math"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

// runLatePayloadRequest retries missing head envelopes each slot after the payload-timeliness deadline.
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
	ticker := slots.NewSlotTickerWithOffset(clock.GenesisTime(), offset, cfg.SlotDuration())
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

// requestLatePayload also recovers a head left without its payload when initial sync completed.
func (s *Service) requestLatePayload(slot primitives.Slot) {
	if !s.chainIsStarted() {
		return
	}
	if s.cfg.initialSync.Syncing() {
		return
	}
	headSlot := s.cfg.chain.HeadSlot()
	if headSlot > slot || slots.ToEpoch(headSlot) < params.BeaconConfig().GloasForkEpoch {
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
