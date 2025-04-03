package sync

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/peerdas"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
)

const broadCastMissingDataColumnsTimeIntoSlot = 3 * time.Second

func (s *Service) reconstructDataColumns(ctx context.Context, verifiedRODataColumn blocks.VerifiedRODataColumn) error {
	// Get the block root and the slot.
	blockRoot := verifiedRODataColumn.BlockRoot()
	slot := verifiedRODataColumn.Slot()

	// Get the columns we store.
	storedDataColumns := s.cfg.dataColumnStorage.Summary(blockRoot)
	storedColumnsCount := storedDataColumns.Count()
	numberOfColumns := params.BeaconConfig().NumberOfColumns

	// If less than half of the columns are stored, reconstruction is not possible.
	// If all columns are stored, no need to reconstruct.
	if storedColumnsCount < numberOfColumns/2 || storedColumnsCount == numberOfColumns {
		return nil
	}

	// Reconstruction is possible.
	// Lock to prevent concurrent reconstruction.
	if !s.dataColumsnReconstructionLock.TryLock() {
		// If the mutex is already locked, it means that another goroutine is already reconstructing the data columns.
		// In this case, no need to reconstruct again.
		// TODO: Implement the (pathological) case where we want to reconstruct data columns corresponding to different blocks at the same time.
		//       This should be a rare case and we can ignore it for now, but it needs to be addressed in the future.
		return nil
	}

	defer s.dataColumsnReconstructionLock.Unlock()

	// Retrieve the node ID.
	nodeID := s.cfg.p2p.NodeID()

	// Prevent custody group count to change during the rest of the function.
	s.cfg.custodyInfo.Mut.RLock()
	defer s.cfg.custodyInfo.Mut.RUnlock()

	// Compute the custody group count.
	custodyGroupCount := s.cfg.custodyInfo.ActualGroupCount()

	// Retrieve our local node info.
	localNodeInfo, _, err := peerdas.Info(nodeID, custodyGroupCount)
	if err != nil {
		return errors.Wrap(err, "peer info")
	}

	// Load all the possible data columns sidecars, to minimize reconstruction time.
	verifiedRODataColumnSidecars, err := s.cfg.dataColumnStorage.Get(blockRoot, nil)
	if err != nil {
		return errors.Wrap(err, "get data column sidecars")
	}

	dataColumnSideCars := make([]*ethpb.DataColumnSidecar, 0, storedColumnsCount)
	for _, verifiedRODataColumn := range verifiedRODataColumnSidecars {
		dataColumnSideCars = append(dataColumnSideCars, verifiedRODataColumn.DataColumnSidecar)
	}

	// Recover cells and proofs.
	recoveredCellsAndProofs, err := peerdas.RecoverCellsAndProofs(dataColumnSideCars, blockRoot)
	if err != nil {
		return errors.Wrap(err, "recover cells and proofs")
	}

	// Reconstruct the data columns sidecars.
	dataColumnSidecars, err := peerdas.DataColumnSidecarsForReconstruct(
		verifiedRODataColumn.KzgCommitments,
		verifiedRODataColumn.SignedBlockHeader,
		verifiedRODataColumn.KzgCommitmentsInclusionProof,
		recoveredCellsAndProofs,
	)
	if err != nil {
		return errors.Wrap(err, "data column sidecars")
	}

	// Build verified read only data columns to save.
	verifiedRODataColumns := make([]blocks.VerifiedRODataColumn, 0, len(localNodeInfo.CustodyColumns))
	for _, dataColumnSidecar := range dataColumnSidecars {
		shouldSave := localNodeInfo.CustodyColumns[dataColumnSidecar.ColumnIndex]
		if !shouldSave {
			// We do not custody this column, so we dot not need to save it.
			continue
		}

		roDataColumn, err := blocks.NewRODataColumnWithRoot(dataColumnSidecar, blockRoot)
		if err != nil {
			return errors.Wrap(err, "new read-only data column with root")
		}

		verifiedRoDataColumn := blocks.NewVerifiedRODataColumn(roDataColumn)
		verifiedRODataColumns = append(verifiedRODataColumns, verifiedRoDataColumn)
	}

	// Save the data columns sidecars in the database.
	if err := s.cfg.dataColumnStorage.Save(verifiedRODataColumns); err != nil {
		return errors.Wrap(err, "save data column sidecars")
	}

	// Schedule the broadcast.
	if err := s.scheduleReconstructedDataColumnsBroadcast(ctx, verifiedRODataColumn); err != nil {
		return errors.Wrap(err, "schedule reconstructed data columns broadcast")
	}

	log.WithFields(logrus.Fields{
		"root":             fmt.Sprintf("%#x", blockRoot),
		"slot":             slot,
		"fromColumnsCount": storedColumnsCount,
	}).Debug("Data columns reconstructed and saved")

	return nil
}

func (s *Service) scheduleReconstructedDataColumnsBroadcast(
	ctx context.Context,
	dataColumnSidecar blocks.VerifiedRODataColumn,
) error {
	// Extract the block root, the proposer index and the slot from the data column sidecar
	root := dataColumnSidecar.BlockRoot()
	proposerIndex := dataColumnSidecar.ProposerIndex()
	slot := dataColumnSidecar.Slot()

	log := log.WithFields(logrus.Fields{
		"root": fmt.Sprintf("%x", root),
		"slot": slot,
	})

	// Get the time corresponding to the start of the slot.
	genesisTime := uint64(s.cfg.chain.GenesisTime().Unix())
	slotStartTime, err := slots.ToTime(genesisTime, slot)
	if err != nil {
		return errors.Wrap(err, "to time")
	}

	// Compute when to broadcast the missing data columns.
	broadcastTime := slotStartTime.Add(broadCastMissingDataColumnsTimeIntoSlot)

	// Compute the waiting time. This could be negative. In such a case, broadcast immediately.
	waitingTime := time.Until(broadcastTime)

	time.AfterFunc(waitingTime, func() {
		s.dataColumsnReconstructionLock.Lock()
		defer s.dataColumsnReconstructionLock.Unlock()

		// Get the node ID.
		nodeID := s.cfg.p2p.NodeID()

		// Prevent custody group count to change during the rest of the function.
		s.cfg.custodyInfo.Mut.RLock()
		defer s.cfg.custodyInfo.Mut.RUnlock()

		// Get the custody group count.
		custodyGroupCount := s.cfg.custodyInfo.ActualGroupCount()

		// Retrieve the local node info.
		localNodeInfo, _, err := peerdas.Info(nodeID, custodyGroupCount)
		if err != nil {
			log.WithError(err).Error("Peer info")
			return
		}

		// Get the data columns we actually store.
		summary := s.cfg.dataColumnStorage.Summary(root)

		// Compute the missing data columns (data columns we should custody but we do not have received via gossip.)
		missingColumns := make([]uint64, 0, len(localNodeInfo.CustodyColumns))
		for column := range localNodeInfo.CustodyColumns {
			if !s.hasSeenDataColumnIndex(slot, proposerIndex, column) {
				missingColumns = append(missingColumns, column)
			}
		}

		// Exit early if there are no missing data columns.
		// This is the happy path.
		if len(missingColumns) == 0 {
			return
		}

		for _, column := range missingColumns {
			if !summary.HasIndex(column) {
				// This column was not received nor reconstructed. This should not happen.
				log.WithField("column", column).Error("Data column not received nor reconstructed")
			}
		}

		// Get the non received but reconstructed data column.
		verifiedRODataColumnSidecars, err := s.cfg.dataColumnStorage.Get(root, missingColumns)
		if err != nil {
			log.WithError(err).Error("get data column sidecars")
			return
		}

		for _, verifiedRODataColumn := range verifiedRODataColumnSidecars {
			// Compute the subnet for this column.
			subnet := peerdas.ComputeSubnetForDataColumnSidecar(verifiedRODataColumn.ColumnIndex)

			// Broadcast the missing data column.
			if err := s.cfg.p2p.BroadcastDataColumn(ctx, root, subnet, verifiedRODataColumn.DataColumnSidecar); err != nil {
				log.WithError(err).Error("Broadcast data column")
			}
		}

		// Sort the missing data columns.
		slices.Sort[[]uint64](missingColumns)

		log.WithFields(logrus.Fields{
			"timeIntoSlot": broadCastMissingDataColumnsTimeIntoSlot,
			"columns":      missingColumns,
		}).Debug("Start broadcasting not seen via gossip but reconstructed data columns")
	})

	return nil
}
