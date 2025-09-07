package sync

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	broadcastMissingDataColumnsTimeIntoSlotMin = 1 * time.Second
	broadcastMissingDataColumnsSlack           = 2 * time.Second
)

// reconstructSaveBroadcastDataColumnSidecars reconstructs, if possible,
// all data column sidecars. Then, it saves missing sidecars to the store.
// After a delay, it broadcasts in the background not seen via gossip
// (but reconstructed) sidecars.
func (s *Service) reconstructSaveBroadcastDataColumnSidecars(ctx context.Context, sidecar blocks.VerifiedRODataColumn) error {
	startTime := time.Now()
	samplesPerSlot := params.BeaconConfig().SamplesPerSlot

	root := sidecar.BlockRoot()
	slot := sidecar.Slot()
	proposerIndex := sidecar.ProposerIndex()

	// Lock to prevent concurrent reconstructions.
	s.reconstructionLock.Lock()
	defer s.reconstructionLock.Unlock()

	// Get the columns we store.
	storedDataColumns := s.cfg.dataColumnStorage.Summary(root)
	storedColumnsCount := storedDataColumns.Count()
	numberOfColumns := params.BeaconConfig().NumberOfColumns

	// If reconstruction is not possible or if all columns are already stored, exit early.
	if storedColumnsCount < peerdas.MinimumColumnCountToReconstruct() || storedColumnsCount == numberOfColumns {
		return nil
	}

	// Retrieve our local node info.
	nodeID := s.cfg.p2p.NodeID()
	custodyGroupCount, err := s.cfg.p2p.CustodyGroupCount()
	if err != nil {
		return errors.Wrap(err, "custody group count")
	}

	samplingSize := max(custodyGroupCount, samplesPerSlot)
	localNodeInfo, _, err := peerdas.Info(nodeID, samplingSize)
	if err != nil {
		return errors.Wrap(err, "peer info")
	}

	// Load all the possible data column sidecars, to minimize reconstruction time.
	verifiedSidecars, err := s.cfg.dataColumnStorage.Get(root, nil)
	if err != nil {
		return errors.Wrap(err, "get data column sidecars")
	}

	// Reconstruct all the data column sidecars.
	reconstructedSidecars, err := peerdas.ReconstructDataColumnSidecars(verifiedSidecars)
	if err != nil {
		return errors.Wrap(err, "reconstruct data column sidecars")
	}

	// Filter reconstructed sidecars to save.
	custodyColumns := localNodeInfo.CustodyColumns
	toSaveSidecars := make([]blocks.VerifiedRODataColumn, 0, len(custodyColumns))
	for _, sidecar := range reconstructedSidecars {
		if custodyColumns[sidecar.Index] {
			toSaveSidecars = append(toSaveSidecars, sidecar)
		}
	}

	// Save the data column sidecars to the database.
	// Note: We do not call `receiveDataColumn`, because it will ignore
	// incoming data columns via gossip while we did not broadcast (yet) the reconstructed data columns.
	if err := s.cfg.dataColumnStorage.Save(toSaveSidecars); err != nil {
		return errors.Wrap(err, "save data column sidecars")
	}

	slotStartTime, err := slots.StartTime(s.cfg.clock.GenesisTime(), slot)
	if err != nil {
		return errors.Wrap(err, "failed to calculate slot start time")
	}
	log.WithFields(logrus.Fields{
		"root":                          fmt.Sprintf("%#x", root),
		"slot":                          slot,
		"fromColumnsCount":              storedColumnsCount,
		"sinceSlotStartTime":            time.Since(slotStartTime),
		"reconstructionAndSaveDuration": time.Since(startTime),
	}).Debug("Data columns reconstructed and saved")

	// Update reconstruction metrics.
	dataColumnReconstructionHistogram.Observe(float64(time.Since(startTime).Milliseconds()))
	dataColumnReconstructionCounter.Add(float64(len(reconstructedSidecars) - len(verifiedSidecars)))

	// Schedule the broadcast.
	if err := s.scheduleMissingDataColumnSidecarsBroadcast(ctx, root, proposerIndex, slot); err != nil {
		return errors.Wrap(err, "schedule reconstructed data columns broadcast")
	}

	return nil
}

// scheduleMissingDataColumnSidecarsBroadcast schedules the broadcast of missing
// (aka. not seen via gossip but reconstructed) sidecars.
func (s *Service) scheduleMissingDataColumnSidecarsBroadcast(
	ctx context.Context,
	root [fieldparams.RootLength]byte,
	proposerIndex primitives.ValidatorIndex,
	slot primitives.Slot,
) error {
	log := log.WithFields(logrus.Fields{
		"root": fmt.Sprintf("%x", root),
		"slot": slot,
	})

	// Get the time corresponding to the start of the slot.
	genesisTime := s.cfg.clock.GenesisTime()
	slotStartTime, err := slots.StartTime(genesisTime, slot)
	if err != nil {
		return errors.Wrap(err, "failed to calculate slot start time")
	}

	// Compute the waiting time. This could be negative. In such a case, broadcast immediately.
	randFloat := s.reconstructionRandGen.Float64()
	timeIntoSlot := broadcastMissingDataColumnsTimeIntoSlotMin + time.Duration(float64(broadcastMissingDataColumnsSlack)*randFloat)
	broadcastTime := slotStartTime.Add(timeIntoSlot)
	waitingTime := time.Until(broadcastTime)
	time.AfterFunc(waitingTime, func() {
		// Return early if the context was canceled during the waiting time.
		if err := ctx.Err(); err != nil {
			return
		}

		if err := s.broadcastMissingDataColumnSidecars(slot, proposerIndex, root, timeIntoSlot); err != nil {
			log.WithError(err).Error("Failed to broadcast missing data column sidecars")
		}
	})

	return nil
}

func (s *Service) broadcastMissingDataColumnSidecars(
	slot primitives.Slot,
	proposerIndex primitives.ValidatorIndex,
	root [fieldparams.RootLength]byte,
	timeIntoSlot time.Duration,
) error {
	// Get the node ID.
	nodeID := s.cfg.p2p.NodeID()

	// Retrieve the local node info.
	custodyGroupCount, err := s.cfg.p2p.CustodyGroupCount()
	if err != nil {
		return errors.Wrap(err, "custody group count")
	}

	localNodeInfo, _, err := peerdas.Info(nodeID, custodyGroupCount)
	if err != nil {
		return errors.Wrap(err, "peerdas info")
	}

	// Compute the missing data columns (data columns we should custody but we did not received via gossip.)
	missingColumns := make([]uint64, 0, len(localNodeInfo.CustodyColumns))
	for column := range localNodeInfo.CustodyColumns {
		if !s.hasSeenDataColumnIndex(slot, proposerIndex, column) {
			missingColumns = append(missingColumns, column)
		}
	}

	// Return early if there are no missing data columns.
	if len(missingColumns) == 0 {
		return nil
	}

	// Load from the store the non received but reconstructed data column.
	verifiedRODataColumnSidecars, err := s.cfg.dataColumnStorage.Get(root, missingColumns)
	if err != nil {
		return errors.Wrap(err, "data column storage get")
	}

	broadcastedColumns := make([]uint64, 0, len(verifiedRODataColumnSidecars))
	for _, verifiedRODataColumn := range verifiedRODataColumnSidecars {
		broadcastedColumns = append(broadcastedColumns, verifiedRODataColumn.Index)
		// Compute the subnet for this column.
		subnet := peerdas.ComputeSubnetForDataColumnSidecar(verifiedRODataColumn.Index)

		// Broadcast the missing data column.
		if err := s.cfg.p2p.BroadcastDataColumnSidecar(subnet, verifiedRODataColumn); err != nil {
			log.WithError(err).Error("Broadcast data column")
		}

		// Now, we can set the data column as seen.
		s.setSeenDataColumnIndex(slot, proposerIndex, verifiedRODataColumn.Index)
	}

	if logrus.GetLevel() >= logrus.DebugLevel {
		// Sort for nice logging.
		slices.Sort(broadcastedColumns)
		slices.Sort(missingColumns)

		log.WithFields(logrus.Fields{
			"timeIntoSlot":   timeIntoSlot,
			"missingColumns": missingColumns,
			"broadcasted":    broadcastedColumns,
		}).Debug("Start broadcasting not seen via gossip but reconstructed data columns")
	}

	return nil
}

// processDataColumnSidecarsFromExecutionFromBlock retrieves (if available) data column sidecars data from the execution client,
// builds corresponding sidecars, save them to the storage, and broadcasts them over P2P if necessary.
func (s *Service) processDataColumnSidecarsFromExecutionFromBlock(ctx context.Context, roBlock blocks.ROBlock) error {
	const delay = 250 * time.Millisecond
	secondsPerHalfSlot := time.Duration(params.BeaconConfig().SecondsPerSlot/2) * time.Second

	numberOfColumns := params.BeaconConfig().NumberOfColumns

	root := roBlock.Root()
	block := roBlock.Block()
	slot := block.Slot()
	proposerIndex := block.ProposerIndex()

	commitments, err := block.Body().BlobKzgCommitments()
	if err != nil {
		return errors.Wrap(err, "blob kzg commitments")
	}

	ctx, cancel := context.WithTimeout(ctx, secondsPerHalfSlot)
	defer cancel()

	for {
		// Check if some data column sidecars to custody are missing.
		missingIndices, err := s.missingDataColumnSidecars(root, commitments)
		if err != nil {
			return errors.Wrap(err, "missing data column sidecars")
		}

		// Return early if all needed data column sidecars are already available in storage.
		if len(missingIndices) == 0 {
			return nil
		}

		// Try to reconstruct data column sidecars from the execution client.
		sidecars, err := s.cfg.executionReconstructor.ConstructDataColumnSidecarsFromBlock(ctx, roBlock)
		if err != nil {
			return errors.Wrap(err, "reconstruct data column sidecars")
		}

		// No sidecars are retrieved from the EL, retry later
		sidecarCount := uint64(len(sidecars))
		if sidecarCount == 0 {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			time.Sleep(delay)

			continue
		}

		// Boundary check.
		if sidecarCount != numberOfColumns {
			return errors.Errorf("reconstruct data column sidecars returned %d sidecars, expected %d - should never happen", sidecarCount, numberOfColumns)
		}

		// Broadcast and save data column sidecars to custody but not yet received.
		for index := range missingIndices {
			if index >= sidecarCount {
				return errors.Errorf("data column index %d >= sidecar count %d - should never happen", index, sidecarCount)
			}

			// This sidecar has been received in the meantime, skip it.
			if s.hasSeenDataColumnIndex(slot, proposerIndex, index) {
				continue
			}

			sidecar := sidecars[index]

			if err := s.cfg.p2p.BroadcastDataColumnSidecar(sidecar.Index, sidecar); err != nil {
				return errors.Wrap(err, "broadcast data column sidecar")
			}

			if err := s.receiveDataColumnSidecar(ctx, sidecar); err != nil {
				return errors.Wrap(err, "receive data column sidecar")
			}
		}

		return nil
	}
}

func (s *Service) processDataColumnSidecarsFromExecutionFromColumnSidecar(ctx context.Context, sidecar blocks.VerifiedRODataColumn) error {
	const delay = 250 * time.Millisecond
	secondsPerHalfSlot := time.Duration(params.BeaconConfig().SecondsPerSlot/2) * time.Second

	numberOfColumns := params.BeaconConfig().NumberOfColumns

	commitments := sidecar.KzgCommitments
	root := sidecar.BlockRoot()

	ctx, cancel := context.WithTimeout(ctx, secondsPerHalfSlot)
	defer cancel()

	for {
		// Check if some data column sidecars to custody are missing.
		missingIndices, err := s.missingDataColumnSidecars(root, commitments)
		if err != nil {
			return errors.Wrap(err, "missing data column sidecars")
		}

		// Return early if all needed data column sidecars are already available in storage.
		if len(missingIndices) == 0 {
			return nil
		}

		// Try to reconstruct data column sidecars from the execution client.
		sidecars, err := s.cfg.executionReconstructor.ConstructDataColumnSidecarsFromColumnSidecar(ctx, sidecar)
		if err != nil {
			return errors.Wrap(err, "reconstruct data column sidecars")
		}

		// No sidecars are retrieved from the EL, retry later
		sidecarCount := uint64(len(sidecars))
		if sidecarCount == 0 {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			time.Sleep(delay)

			continue
		}

		// Boundary check.
		if sidecarCount != numberOfColumns {
			return errors.Errorf("reconstruct data column sidecars returned %d sidecars, expected %d - should never happen", sidecarCount, numberOfColumns)
		}

		blockSlot, proposerIndex := sidecar.Slot(), sidecar.ProposerIndex()

		// Broadcast and save data column sidecars to custody but not yet received.
		for index := range missingIndices {
			log := log.WithField("columnIndex", index)
			if index >= sidecarCount {
				return errors.Errorf("data column index %d >= sidecar count %d - should never happen", index, sidecarCount)
			}

			// This sidecar has been received in the meantime, skip it.
			if s.hasSeenDataColumnIndex(blockSlot, proposerIndex, index) {
				continue
			}

			sidecar := sidecars[index]

			if err := s.cfg.p2p.BroadcastDataColumnSidecar(sidecar.Index, sidecar); err != nil {
				log.WithError(err).Error("Failed to broadcast data column")
			}

			if err := s.receiveDataColumnSidecar(ctx, sidecar); err != nil {
				log.WithError(err).Error("Failed to receive data column")
			}
		}

		return nil
	}
}

// missingDataColumnSidecars returns the data column indices we should custody and that are missing in our storage
// for the given read only beacon block.
func (s *Service) missingDataColumnSidecars(root [fieldparams.RootLength]byte, commitments [][]byte) (map[uint64]bool, error) {
	// Return early if there is not commitments.
	if len(commitments) == 0 {
		return nil, nil
	}

	// Retrieve our node ID.
	nodeID := s.cfg.p2p.NodeID()

	// Get the custody group sampling size for the node.
	custodyGroupCount, err := s.cfg.p2p.CustodyGroupCount()
	if err != nil {
		return nil, errors.Wrap(err, "custody group count")
	}

	// Compute the sampling size.
	// https://github.com/ethereum/consensus-specs/blob/master/specs/fulu/das-core.md#custody-sampling
	samplesPerSlot := params.BeaconConfig().SamplesPerSlot
	samplingSize := max(samplesPerSlot, custodyGroupCount)

	// Get the peer info for the node.
	peerInfo, _, err := peerdas.Info(nodeID, samplingSize)
	if err != nil {
		return nil, errors.Wrap(err, "peer info")
	}

	// Get the indices of the data column sidecars we have in the store.
	storedIndices := s.cfg.dataColumnStorage.Summary(root).Stored()

	// List indices we should custody and that are missing in our storage.
	missingIndices := make(map[uint64]bool, samplingSize)
	for index := range peerInfo.CustodyColumns {
		if !storedIndices[index] {
			missingIndices[index] = true
		}
	}

	return missingIndices, nil
}
