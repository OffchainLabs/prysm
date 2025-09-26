package sync

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// processDataColumnSidecarsFromReconstruction, after a random delay, attempts to reconstruct,
// broadcast and receive missing data column sidecars for the given block root.
// https:github.com/ethereum/consensus-specs/blob/master/specs/fulu/das-core.md#reconstruction-and-cross-seeding
func (s *Service) processDataColumnSidecarsFromReconstruction(ctx context.Context, sidecar blocks.VerifiedRODataColumn) error {
	key := fmt.Sprintf("%#x", sidecar.BlockRoot())
	if _, err, _ := s.reconstructionSingleFlight.Do(key, func() (interface{}, error) {
		var wg sync.WaitGroup

		root := sidecar.BlockRoot()
		slot := sidecar.Slot()
		proposerIndex := sidecar.ProposerIndex()

		// Return early if reconstruction is not needed.
		if !s.shouldReconstruct(root) {
			return nil, nil
		}

		// Compute the slot start time.
		slotStartTime, err := slots.StartTime(s.cfg.clock.GenesisTime(), slot)
		if err != nil {
			return nil, errors.Wrap(err, "failed to calculate slot start time")
		}

		// Randomly choose value before starting reconstruction.
		timeIntoSlot := s.computeRandomDelay(slotStartTime)
		broadcastTime := slotStartTime.Add(timeIntoSlot)
		waitingTime := time.Until(broadcastTime)

		wg.Add(1)
		time.AfterFunc(waitingTime, func() {
			defer wg.Done()

			// Return early if the context was canceled during the waiting time.
			if err := ctx.Err(); err != nil {
				return
			}

			// Return early if reconstruction is not needed anymore.
			if !s.shouldReconstruct(root) {
				return
			}

			// Load all the stored data column sidecars for this root.
			verifiedSidecars, err := s.cfg.dataColumnStorage.Get(root, nil)
			if err != nil {
				log.WithError(err).Error("Failed to get data column sidecars")
				return
			}

			// Reconstruct all the data column sidecars.
			startTime := time.Now()
			reconstructedSidecars, err := peerdas.ReconstructDataColumnSidecars(verifiedSidecars)
			if err != nil {
				log.WithError(err).Error("Failed to reconstruct data column sidecars")
				return
			}

			duration := time.Since(startTime)
			dataColumnReconstructionHistogram.Observe(float64(duration.Milliseconds()))
			dataColumnReconstructionCounter.Add(float64(len(reconstructedSidecars) - len(verifiedSidecars)))

			// Retrieve indices of data column sidecars to sample.
			columnIndicesToSample, err := s.columnIndicesToSample()
			if err != nil {
				log.WithError(err).Error("Failed to get column indices to sample")
				return
			}

			unseenIndices, err := s.broadcastAndReceiveUnseenDataColumnSidecars(ctx, slot, proposerIndex, columnIndicesToSample, reconstructedSidecars)
			if err != nil {
				log.WithError(err).Error("Failed to broadcast and receive unseen data column sidecars")
				return
			}

			if len(unseenIndices) > 0 {
				log.WithFields(logrus.Fields{
					"root":          fmt.Sprintf("%#x", root),
					"slot":          slot,
					"proposerIndex": proposerIndex,
					"count":         len(unseenIndices),
					"indices":       helpers.SortedPrettySliceFromMap(unseenIndices),
					"duration":      duration,
				}).Debug("Reconstructed data column sidecars")
			}
		})

		wg.Wait()
		return nil, nil
	}); err != nil {
		return err
	}

	return nil
}

// shouldReconstruct returns true if we should attempt to reconstruct the data columns for the given block root.
func (s *Service) shouldReconstruct(root [fieldparams.RootLength]byte) bool {
	numberOfColumns := params.BeaconConfig().NumberOfColumns

	// Get the columns we store.
	storedDataColumns := s.cfg.dataColumnStorage.Summary(root)
	storedColumnsCount := storedDataColumns.Count()

	// Reconstruct only if we have at least the minimum number of columns to reconstruct, but not all the columns.
	// (If we have not enough columns, reconstruction is impossible. If we have all the columns, reconstruction is unnecessary.)
	return storedColumnsCount >= peerdas.MinimumColumnCountToReconstruct() && storedColumnsCount != numberOfColumns
}

// computeRandomDelay computes a random delay duration to wait before reconstructing data column sidecars.
func (s *Service) computeRandomDelay(slotStartTime time.Time) time.Duration {
	const maxReconstructionDelaySec = 2.

	randFloat := s.reconstructionRandGen.Float64()
	timeIntoSlot := time.Duration(maxReconstructionDelaySec * randFloat * float64(time.Second))
	return timeIntoSlot
}
