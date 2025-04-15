package blockchain

import (
	"context"
	"fmt"
	"time"

	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/sirupsen/logrus"
)

const updateInclusionListBlockInterval = time.Second

// Routine that updates block building with inclusion lists one second before the slot starts.
func (s *Service) updateBlockWithInclusionListRoutine() {
	if err := s.waitForSync(); err != nil {
		log.WithError(err).Error("Failed to wait for initial sync")
		return
	}

	interval := time.Second*time.Duration(params.BeaconConfig().SecondsPerSlot) - updateInclusionListBlockInterval
	ticker := slots.NewSlotTickerWithIntervals(s.genesisTime, []time.Duration{interval})

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C():
			s.updateBlockWithInclusionList(context.Background())
		}
	}
}

// Updates block building with inclusion lists, the current payload ID, and the new upload ID.
func (s *Service) updateBlockWithInclusionList(ctx context.Context) {
	currentSlot := s.CurrentSlot()

	// Skip update if not in or past the FOCIL fork epoch.
	if slots.ToEpoch(currentSlot) < params.BeaconConfig().Eip7805ForkEpoch {
		return
	}

	s.cfg.ForkChoiceStore.RLock()
	defer s.cfg.ForkChoiceStore.RUnlock()

	headRoot := s.headRoot()
	id, found := s.cfg.PayloadIDCache.PayloadID(currentSlot+1, headRoot)
	if !found {
		return
	}

	txs := s.inclusionListCache.Get(currentSlot)
	if len(txs) == 0 {
		log.WithField("slot", currentSlot).Warn("No inclusion list transactions found to update block")
		return
	}

	newID, err := s.cfg.ExecutionEngineCaller.UpdatePayloadWithInclusionList(ctx, id, txs)
	if err != nil {
		log.WithError(err).Error("Failed to update block with inclusion list")
		return
	}

	log.WithFields(logrus.Fields{
		"slot":     currentSlot,
		"headRoot": fmt.Sprintf("%x", headRoot),
		"txs":      len(txs),
	}).Info("Updated block with inclusion list")

	s.cfg.PayloadIDCache.Set(currentSlot+1, headRoot, *newID)
}
