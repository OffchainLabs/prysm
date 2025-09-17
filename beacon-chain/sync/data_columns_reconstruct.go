package sync

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/pkg/errors"
)

// processDataColumnSidecarsFromReconstruction, after a random delay, attempts to reconstruct,
// broadcast and receive missing data column sidecars for the given block root.
// https:github.com/ethereum/consensus-specs/blob/master/specs/fulu/das-core.md#reconstruction-and-cross-seeding
func (s *Service) processDataColumnSidecarsFromReconstruction(ctx context.Context, sidecar blocks.VerifiedRODataColumn) error {
	key := fmt.Sprintf("%#x", sidecar.BlockRoot())
	if _, err, _ := s.reconstructionSingleFlight.Do(key, func() (interface{}, error) {
		const maxReconstructionDelaySec = 2.

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
		randFloat := s.reconstructionRandGen.Float64()
		timeIntoSlot := time.Duration(maxReconstructionDelaySec*randFloat) * time.Second
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

			dataColumnReconstructionHistogram.Observe(float64(time.Since(startTime).Milliseconds()))
			dataColumnReconstructionCounter.Add(float64(len(reconstructedSidecars) - len(verifiedSidecars)))

			// Retrieve indices of data column sidecars to sample.
			columnIndicesToSample, err := s.columnIndicesToSample()
			if err != nil {
				log.WithError(err).Error("Failed to get column indices to sample")
				return
			}

			// Compute sidecars we need to broadcast and receive.
			unseenSidecars := make([]blocks.VerifiedRODataColumn, 0, len(reconstructedSidecars))
			for _, sidecar := range reconstructedSidecars {
				// Skip already seen data column sidecars.
				if s.hasSeenDataColumnIndex(slot, proposerIndex, sidecar.Index) {
					continue
				}

				if columnIndicesToSample[sidecar.Index] {
					unseenSidecars = append(unseenSidecars, sidecar)
				}
			}

			// Broadcast all the data column sidecars we reconstructed but did not see via gossip.
			for _, sidecar := range unseenSidecars {
				// Compute the subnet for this data column sidecar.
				subnet := peerdas.ComputeSubnetForDataColumnSidecar(sidecar.Index)

				// Broadcast the data column sidecar.
				if err := s.cfg.p2p.BroadcastDataColumnSidecar(subnet, sidecar); err != nil {
					log.WithError(err).Error("Broadcast data column")
				}
			}

			// Receive data column sidecars.
			if err := s.receiveDataColumnSidecars(ctx, unseenSidecars); err != nil {
				log.WithError(err).Error("Failed to receive data column sidecars")
				return
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

	// Do not reconstruct if we don't have enough columns or if we already have all columns.
	if storedColumnsCount < peerdas.MinimumColumnCountToReconstruct() || storedColumnsCount == numberOfColumns {
		return false
	}

	return true
}
