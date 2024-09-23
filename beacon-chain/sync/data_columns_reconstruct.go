package sync

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/patrickmn/go-cache"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/peerdas"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
)

const broadCastMissingDataColumnsTimeIntoSlot = 3 * time.Second

func (s *Service) reconstructDataColumns(ctx context.Context, verifiedRODataColumn blocks.VerifiedRODataColumn) error {
	// Get the block root.
	blockRoot := verifiedRODataColumn.BlockRoot()

	// Get the columns we store.
	storedDataColumns, err := s.storedDataColumns(blockRoot)
	if err != nil {
		return errors.Wrap(err, "stored data columns")
	}

	storedColumnsCount := len(storedDataColumns)
	numberOfColumns := fieldparams.NumberOfColumns

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

	// Retrieve the custody columns.
	nodeID := s.cfg.p2p.NodeID()
	custodySubnetCount := peerdas.CustodySubnetCount()
	custodyColumns, err := peerdas.CustodyColumns(nodeID, custodySubnetCount)
	if err != nil {
		return errors.Wrap(err, "custody columns")
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

	// Save the data columns sidecars in the database.
	for _, dataColumnSidecar := range dataColumnSidecars {
		shouldSave := custodyColumns[dataColumnSidecar.ColumnIndex]
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

		// Mark the data column as stored (but not received).
		if err := s.setStoredDataColumn(blockRoot, dataColumnSidecar.ColumnIndex); err != nil {
			return errors.Wrap(err, "set stored data column")
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
	log := log.WithField("root", fmt.Sprintf("%x", blockRoot))

	// Retrieve the slot of the block.
	slot := dataColumn.Slot()

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

		// Get the received by gossip data columns.
		receivedDataColumns, err := s.receivedDataColumns(blockRoot)
		if err != nil {
			log.WithError(err).Error("Received data columns")
			return
		}

		if receivedDataColumns == nil {
			log.Error("No received data columns")
			return
		}

		// Get the data columns we should store.
		nodeID := s.cfg.p2p.NodeID()
		custodySubnetCount := peerdas.CustodySubnetCount()
		custodyDataColumns, err := peerdas.CustodyColumns(nodeID, custodySubnetCount)
		if err != nil {
			log.WithError(err).Error("Custody columns")
		}

		// Get the data columns we actually store.
		storedDataColumns, err := s.storedDataColumns(blockRoot)
		if err != nil {
			log.WithField("root", fmt.Sprintf("%x", blockRoot)).WithError(err).Error("Columns indices")
			return
		}

		// Compute the missing data columns (data columns we should custody but we do not have received via gossip.)
		missingColumns := make(map[uint64]bool, len(custodyDataColumns))
		for column := range custodyDataColumns {
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
				}).Error("Data column not received nor reconstructed")
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
			if err := s.cfg.p2p.BroadcastDataColumn(ctx, blockRoot, subnet, dataColumnSidecar); err != nil {
				log.WithError(err).Error("Broadcast data column")
			}
		}

		// Get the missing data columns under sorted form.
		missingColumnsList := make([]uint64, 0, len(missingColumns))
		for column := range missingColumns {
			missingColumnsList = append(missingColumnsList, column)
		}

		// Sort the missing data columns.
		slices.Sort[[]uint64](missingColumnsList)

		log.WithFields(logrus.Fields{
			"root":         fmt.Sprintf("%x", blockRoot),
			"slot":         slot,
			"timeIntoSlot": broadCastMissingDataColumnsTimeIntoSlot,
			"columns":      missingColumnsList,
		}).Debug("Start broadcasting not seen via gossip but reconstructed data columns")
	})

	return nil
}

// setReceivedDataColumn marks the data column for a given root as received.
func (s *Service) setReceivedDataColumn(root [fieldparams.RootLength]byte, columnIndex uint64) error {
	s.receivedDataColumnsFromRootLock.Lock()
	defer s.receivedDataColumnsFromRootLock.Unlock()

	if err := setDataColumnCache(s.receivedDataColumnsFromRoot, root, columnIndex); err != nil {
		return errors.Wrap(err, "set data column cache")
	}

	return nil
}

// receivedDataColumns returns the received data columns for a given root.
func (s *Service) receivedDataColumns(root [fieldparams.RootLength]byte) (map[uint64]bool, error) {
	dataColumns, err := dataColumnsCache(s.receivedDataColumnsFromRoot, root)
	if err != nil {
		return nil, errors.Wrap(err, "data columns cache")
	}

	return dataColumns, nil
}

// setStorededDataColumn marks the data column for a given root as stored.
func (s *Service) setStoredDataColumn(root [fieldparams.RootLength]byte, columnIndex uint64) error {
	s.storedDataColumnsFromRootLock.Lock()
	defer s.storedDataColumnsFromRootLock.Unlock()

	if err := setDataColumnCache(s.storedDataColumnsFromRoot, root, columnIndex); err != nil {
		return errors.Wrap(err, "set data column cache")
	}

	return nil
}

// storedDataColumns returns the received data columns for a given root.
func (s *Service) storedDataColumns(root [fieldparams.RootLength]byte) (map[uint64]bool, error) {
	dataColumns, err := dataColumnsCache(s.storedDataColumnsFromRoot, root)
	if err != nil {
		return nil, errors.Wrap(err, "data columns cache")
	}

	return dataColumns, nil
}

// setDataColumnCache sets the data column for a given root in columnsCache.
// The caller should hold the lock for the cache.
func setDataColumnCache(columnsCache *cache.Cache, root [fieldparams.RootLength]byte, columnIndex uint64) error {
	if columnIndex >= fieldparams.NumberOfColumns {
		return errors.Errorf("column index out of bounds: got %d, expected < %d", columnIndex, fieldparams.NumberOfColumns)
	}

	rootString := fmt.Sprintf("%#x", root)

	// Get all the data columns for this root.
	items, ok := columnsCache.Get(rootString)
	if !ok {
		var columns [fieldparams.NumberOfColumns]bool
		columns[columnIndex] = true
		columnsCache.Set(rootString, columns, cache.DefaultExpiration)

		return nil
	}

	// Cast the array.
	columns, ok := items.([fieldparams.NumberOfColumns]bool)
	if !ok {
		return errors.New("cannot cast data columns from cache")
	}

	// Add the data column to the data columns.
	columns[columnIndex] = true

	// Update the data columns in the cache.
	columnsCache.Set(rootString, columns, cache.DefaultExpiration)

	return nil
}

// dataColumnsCache returns the data columns for a given root in columnsCache.
func dataColumnsCache(columnsCache *cache.Cache, root [fieldparams.RootLength]byte) (map[uint64]bool, error) {
	rootString := fmt.Sprintf("%#x", root)

	// Get all the data columns for this root.
	items, ok := columnsCache.Get(rootString)
	if !ok {
		return nil, nil
	}

	// Cast the array.
	dataColumns, ok := items.([fieldparams.NumberOfColumns]bool)
	if !ok {
		return nil, errors.New("Cannot cast data columns from cache")
	}

	// Convert to map.
	result := columnsArrayToMap(dataColumns)

	return result, nil
}

// columnsArrayToMap converts an array of columns to a map of columns.
func columnsArrayToMap(columnsArray [fieldparams.NumberOfColumns]bool) map[uint64]bool {
	columnsMap := make(map[uint64]bool)

	for i, v := range columnsArray {
		if v {
			columnsMap[uint64(i)] = v
		}
	}

	return columnsMap
}
