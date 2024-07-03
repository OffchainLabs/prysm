package sync

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain/kzg"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/peerdas"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
)

const broadCastMissingDataColumnsTimeIntoSlot = 3 * time.Second

// recoverBlobs recovers the blobs from the data column sidecars.
func recoverBlobs(
	dataColumnSideCars []*ethpb.DataColumnSidecar,
	columnsCount int,
	blockRoot [fieldparams.RootLength]byte,
) ([]kzg.Blob, error) {
	if len(dataColumnSideCars) == 0 {
		return nil, errors.New("no data column sidecars")
	}

	// Check if all columns have the same length.
	blobCount := len(dataColumnSideCars[0].DataColumn)
	for _, sidecar := range dataColumnSideCars {
		length := len(sidecar.DataColumn)

		if length != blobCount {
			return nil, errors.New("columns do not have the same length")
		}
	}

	recoveredBlobs := make([]kzg.Blob, 0, blobCount)

	for blobIndex := 0; blobIndex < blobCount; blobIndex++ {
		start := time.Now()

		cellsId := make([]uint64, 0, columnsCount)
		cKzgCells := make([]kzg.Cell, 0, columnsCount)

		for _, sidecar := range dataColumnSideCars {
			// Build the cell ids.
			cellsId = append(cellsId, sidecar.ColumnIndex)

			// Get the cell.
			column := sidecar.DataColumn
			cell := column[blobIndex]

			// Transform the cell as a cKzg cell.
			var ckzgCell kzg.Cell
			for i := 0; i < kzg.FieldElementsPerCell; i++ {
				copy(ckzgCell[i][:], cell[32*i:32*(i+1)])
			}

			cKzgCells = append(cKzgCells, ckzgCell)
		}

		// Recover the blob.
		recoveredCells, err := kzg.RecoverAllCells(cellsId, cKzgCells)
		if err != nil {
			return nil, errors.Wrapf(err, "recover all cells for blob %d", blobIndex)
		}

		recoveredBlob, err := kzg.CellsToBlob(&recoveredCells)
		if err != nil {
			return nil, errors.Wrapf(err, "cells to blob for blob %d", blobIndex)
		}

		recoveredBlobs = append(recoveredBlobs, recoveredBlob)
		log.WithFields(logrus.Fields{
			"elapsed": time.Since(start),
			"index":   blobIndex,
			"root":    fmt.Sprintf("%x", blockRoot),
		}).Debug("Recovered blob")
	}

	return recoveredBlobs, nil
}

func (s *Service) reconstructDataColumns(ctx context.Context, verifiedRODataColumn blocks.VerifiedRODataColumn) error {
	// Lock to prevent concurrent reconstruction.
	s.dataColumsnReconstructionLock.Lock()
	defer s.dataColumsnReconstructionLock.Unlock()

	// Get the block root.
	blockRoot := verifiedRODataColumn.BlockRoot()

	// Get the columns we store.
	storedDataColumns, err := s.cfg.blobStorage.ColumnIndices(blockRoot)
	if err != nil {
		return errors.Wrap(err, "columns indices")
	}

	storedColumnsCount := len(storedDataColumns)
	numberOfColumns := fieldparams.NumberOfColumns

	// If less than half of the columns are stored, reconstruction is not possible.
	// If all columns are stored, no need to reconstruct.
	if storedColumnsCount < numberOfColumns/2 || storedColumnsCount == numberOfColumns {
		return nil
	}

	// Retrieve the custodied columns.
	custodiedColumns, err := peerdas.CustodyColumns(s.cfg.p2p.NodeID(), peerdas.CustodySubnetCount())
	if err != nil {
		return errors.Wrap(err, "custodied columns")
	}

	// Load the data columns sidecars.
	dataColumnSideCars := make([]*ethpb.DataColumnSidecar, 0, storedColumnsCount)
	for index := range storedDataColumns {
		dataColumnSidecar, err := s.cfg.blobStorage.GetColumn(blockRoot, index)
		if err != nil {
			return errors.Wrap(err, "get column")
		}

		dataColumnSideCars = append(dataColumnSideCars, dataColumnSidecar)
	}

	// Recover blobs.
	recoveredBlobs, err := recoverBlobs(dataColumnSideCars, storedColumnsCount, blockRoot)
	if err != nil {
		return errors.Wrap(err, "recover blobs")
	}

	// Reconstruct the data columns sidecars.
	dataColumnSidecars, err := peerdas.DataColumnSidecarsForReconstruct(
		verifiedRODataColumn.KzgCommitments,
		verifiedRODataColumn.SignedBlockHeader,
		verifiedRODataColumn.KzgCommitmentsInclusionProof,
		recoveredBlobs,
	)
	if err != nil {
		return errors.Wrap(err, "data column sidecars")
	}

	// Save the data columns sidecars in the database.
	for _, dataColumnSidecar := range dataColumnSidecars {
		shouldSave := custodiedColumns[dataColumnSidecar.ColumnIndex]
		if !shouldSave {
			// We do not custody this column, so we dot not need to save it.
			continue
		}

		roDataColumn, err := blocks.NewRODataColumnWithRoot(dataColumnSidecar, blockRoot)
		if err != nil {
			return errors.Wrap(err, "new read-only data column with root")
		}

		verifiedRoDataColumn := blocks.NewVerifiedRODataColumn(roDataColumn)
		if err := s.cfg.blobStorage.SaveDataColumn(verifiedRoDataColumn); err != nil {
			return errors.Wrap(err, "save column")
		}
	}

	log.WithField("root", fmt.Sprintf("%x", blockRoot)).Debug("Data columns reconstructed and saved successfully")

	// Schedule the broadcast.
	if err := s.scheduleReconstructedDataColumnsBroadcast(ctx, blockRoot, verifiedRODataColumn); err != nil {
		return errors.Wrap(err, "schedule reconstructed data columns broadcast")
	}

	return nil
}

func (s *Service) scheduleReconstructedDataColumnsBroadcast(
	ctx context.Context,
	blockRoot [fieldparams.RootLength]byte,
	dataColumn blocks.VerifiedRODataColumn,
) error {
	// Retrieve the slot of the block.
	slot := dataColumn.Slot()

	// Get the time corresponding to the start of the slot.
	slotStart, err := slots.ToTime(uint64(s.cfg.chain.GenesisTime().Unix()), slot)
	if err != nil {
		return errors.Wrap(err, "to time")
	}

	// Compute when to broadcast the missing data columns.
	broadcastTime := slotStart.Add(broadCastMissingDataColumnsTimeIntoSlot)

	// Compute the waiting time. This could be negative. In such a case, broadcast immediately.
	waitingTime := time.Until(broadcastTime)

	time.AfterFunc(waitingTime, func() {
		s.dataColumsnReconstructionLock.Lock()
		defer s.deleteReceivedDataColumns(blockRoot)
		defer s.dataColumsnReconstructionLock.Unlock()

		// Get the received by gossip data columns.
		receivedDataColumns := s.receivedDataColumns(blockRoot)
		if receivedDataColumns == nil {
			log.WithField("root", fmt.Sprintf("%x", blockRoot)).Error("No received data columns")
		}

		// Get the data columns we should store.
		custodiedDataColumns, err := peerdas.CustodyColumns(s.cfg.p2p.NodeID(), peerdas.CustodySubnetCount())
		if err != nil {
			log.WithError(err).Error("Custody columns")
		}

		// Get the data columns we actually store.
		storedDataColumns, err := s.cfg.blobStorage.ColumnIndices(blockRoot)
		if err != nil {
			log.WithField("root", fmt.Sprintf("%x", blockRoot)).WithError(err).Error("Columns indices")
			return
		}

		// Compute the missing data columns (data columns we should custody but we do not have received via gossip.)
		missingColumns := make(map[uint64]bool, len(custodiedDataColumns))
		for column := range custodiedDataColumns {
			if ok := receivedDataColumns[column]; !ok {
				missingColumns[column] = true
			}
		}

		// Exit early if there are no missing data columns.
		// This is the happy path.
		if len(missingColumns) == 0 {
			return
		}

		for column := range missingColumns {
			if ok := storedDataColumns[column]; !ok {
				// This column was not received nor reconstructed. This should not happen.
				log.WithFields(logrus.Fields{
					"root":   fmt.Sprintf("%x", blockRoot),
					"slot":   slot,
					"column": column,
				}).Error("Data column not received nor reconstructed.")
				continue
			}

			// Get the non received but reconstructed data column.
			dataColumnSidecar, err := s.cfg.blobStorage.GetColumn(blockRoot, column)
			if err != nil {
				log.WithError(err).Error("Get column")
				continue
			}

			// Compute the subnet for this column.
			subnet := column % params.BeaconConfig().DataColumnSidecarSubnetCount

			// Broadcast the missing data column.
			if err := s.cfg.p2p.BroadcastDataColumn(ctx, subnet, dataColumnSidecar); err != nil {
				log.WithError(err).Error("Broadcast data column")
			}
		}

		// Get the missing data columns under sorted form.
		missingColumnsList := make([]uint64, 0, len(missingColumns))
		for column := range missingColumns {
			missingColumnsList = append(missingColumnsList, column)
		}

		// Sort the missing data columns.
		sort.Slice(missingColumnsList, func(i, j int) bool {
			return missingColumnsList[i] < missingColumnsList[j]
		})

		log.WithFields(logrus.Fields{
			"root":         fmt.Sprintf("%x", blockRoot),
			"slot":         slot,
			"timeIntoSlot": broadCastMissingDataColumnsTimeIntoSlot,
			"columns":      missingColumnsList,
		}).Debug("Broadcasting not seen via gossip but reconstructed data columns.")
	})

	return nil
}

// setReceivedDataColumn marks the data column for a given root as received.
func (s *Service) setReceivedDataColumn(root [fieldparams.RootLength]byte, columnIndex uint64) {
	s.receivedDataColumnsFromRootLock.Lock()
	defer s.receivedDataColumnsFromRootLock.Unlock()

	// Get all the received data columns for this root.
	receivedDataColumns, ok := s.receivedDataColumnsFromRoot[root]
	if !ok {
		// Create the map for this block root if needed.
		receivedDataColumns = make(map[uint64]bool, params.BeaconConfig().NumberOfColumns)
		s.receivedDataColumnsFromRoot[root] = receivedDataColumns
	}

	// Mark the data column as received.
	receivedDataColumns[columnIndex] = true
}

// receivedDataColumns returns the received data columns for a given root.
func (s *Service) receivedDataColumns(root [fieldparams.RootLength]byte) map[uint64]bool {
	s.receivedDataColumnsFromRootLock.RLock()
	defer s.receivedDataColumnsFromRootLock.RUnlock()

	// Get all the received data columns for this root.
	receivedDataColumns, ok := s.receivedDataColumnsFromRoot[root]
	if !ok {
		return nil
	}

	// Copy the received data columns.
	copied := make(map[uint64]bool, len(receivedDataColumns))
	for column, received := range receivedDataColumns {
		copied[column] = received
	}

	return copied
}

// deleteReceivedDataColumns deletes the received data columns for a given root.
func (s *Service) deleteReceivedDataColumns(root [fieldparams.RootLength]byte) {
	s.receivedDataColumnsFromRootLock.Lock()
	defer s.receivedDataColumnsFromRootLock.Unlock()

	delete(s.receivedDataColumnsFromRoot, root)
}
