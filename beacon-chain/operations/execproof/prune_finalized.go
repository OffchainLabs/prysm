package execproof

import (
	"fmt"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

// pruneFinalizedProofs prunes execution proofs pool on every epoch.
// It removes proofs older than the finalized checkpoint to prevent unbounded
// memory growth.
func (s *Service) pruneFinalizedProofs() {
	defer s.slotTicker.Done()

	for {
		select {
		case slot := <-s.slotTicker.C():
			// Only prune at the start of each epoch
			if !slots.IsEpochStart(slot) {
				continue
			}

			finalizedSlot, err := s.getFinalizedSlot()
			if err != nil {
				log.WithError(err).Error("Could not get finalized slot")
				continue
			}

			log.WithField("finalizedSlot", finalizedSlot).Debug("Pruning finalized execution proofs")

			// Prune proofs older than finalized slot
			pruned := s.cfg.Pool.PruneFinalizedProofs(finalizedSlot)
			if pruned > 0 {
				log.WithField("count", pruned).WithField("finalizedSlot", finalizedSlot).
					Debug("Pruned finalized execution proofs")
			}

		case <-s.ctx.Done():
			log.Debug("Context closed, exiting routine")
			return
		}
	}
}

// getFinalizedSlot returns the current finalized slot from the blockchain.
func (s *Service) getFinalizedSlot() (primitives.Slot, error) {
	if s.cfg.FinalizedFetcher == nil {
		// If no finalized fetcher is provided, return 0 (no pruning)
		return 0, fmt.Errorf("no finalized checkpoint fetcher provided")
	}

	cp := s.cfg.FinalizedFetcher.FinalizedCheckpt()
	if cp == nil {
		return 0, fmt.Errorf("finalized checkpoint is nil")
	}

	return slots.EpochStart(cp.Epoch)
}
