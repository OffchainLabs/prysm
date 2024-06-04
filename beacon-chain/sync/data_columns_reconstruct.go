package sync

import (
	"context"
	"fmt"
	"time"

	cKzg4844 "github.com/ethereum/c-kzg-4844/bindings/go"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/feed"
	statefeed "github.com/prysmaticlabs/prysm/v5/beacon-chain/core/feed/state"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/peerdas"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/sirupsen/logrus"
)

// recoverBlobs recovers the blobs from the data column sidecars.
func recoverBlobs(
	dataColumnSideCars []*ethpb.DataColumnSidecar,
	columnsCount int,
	blockRoot [fieldparams.RootLength]byte,
) ([]cKzg4844.Blob, error) {
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

	recoveredBlobs := make([]cKzg4844.Blob, 0, blobCount)

	for blobIndex := 0; blobIndex < blobCount; blobIndex++ {
		start := time.Now()

		cellsId := make([]uint64, 0, columnsCount)
		cKzgCells := make([]cKzg4844.Cell, 0, columnsCount)

		for _, sidecar := range dataColumnSideCars {
			// Build the cell ids.
			cellsId = append(cellsId, sidecar.ColumnIndex)

			// Get the cell.
			column := sidecar.DataColumn
			cell := column[blobIndex]

			// Transform the cell as a cKzg cell.
			var ckzgCell cKzg4844.Cell
			for i := 0; i < cKzg4844.FieldElementsPerCell; i++ {
				copy(ckzgCell[i][:], cell[32*i:32*(i+1)])
			}

			cKzgCells = append(cKzgCells, ckzgCell)
		}

		// Recover the blob.
		recoveredCells, err := cKzg4844.RecoverAllCells(cellsId, cKzgCells)
		if err != nil {
			return nil, errors.Wrapf(err, "recover all cells for blob %d", blobIndex)
		}

		recoveredBlob, err := cKzg4844.CellsToBlob(recoveredCells)
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

// getSignedBlock retrieves the signed block corresponding to the given root.
// If the block is not available, it waits for it.
func (s *Service) getSignedBlock(
	ctx context.Context,
	blockRoot [fieldparams.RootLength]byte,
) (interfaces.ReadOnlySignedBeaconBlock, error) {
	blocksChannel := make(chan *feed.Event, 1)
	blockSub := s.cfg.blockNotifier.BlockFeed().Subscribe(blocksChannel)
	defer blockSub.Unsubscribe()

	// Get the signedBlock corresponding to this root.
	signedBlock, err := s.cfg.beaconDB.Block(ctx, blockRoot)
	if err != nil {
		return nil, errors.Wrap(err, "block")
	}

	// If the block is here, return it.
	if signedBlock != nil {
		return signedBlock, nil
	}

	// Wait for the block to be available.
	for {
		select {
		case blockEvent := <-blocksChannel:
			// Check the type of the event.
			data, ok := blockEvent.Data.(*statefeed.BlockProcessedData)
			if !ok || data == nil {
				continue
			}

			// Check if the block is the one we are looking for.
			if data.BlockRoot != blockRoot {
				continue
			}

			// This is the block we are looking for.
			return data.SignedBlock, nil
		case err := <-blockSub.Err():
			return nil, errors.Wrap(err, "block subscriber error")
		case <-ctx.Done():
			return nil, errors.New("context canceled")
		}
	}
}

func (s *Service) reconstructDataColumns(ctx context.Context, verifiedRODataColumn blocks.VerifiedRODataColumn) error {
	// Lock to prevent concurrent reconstruction.
	s.dataColumsnReconstructionLock.Lock()
	defer s.dataColumsnReconstructionLock.Unlock()

	// Get the block root.
	blockRoot := verifiedRODataColumn.BlockRoot()

	// Get the columns we store.
	storedColumnsIndices, err := s.cfg.blobStorage.ColumnIndices(blockRoot)
	if err != nil {
		return errors.Wrap(err, "columns indices")
	}

	storedColumnsCount := len(storedColumnsIndices)
	numberOfColumns := fieldparams.NumberOfColumns

	// If less than half of the columns are stored, reconstruction is not possible.
	// If all columns are stored, no need to reconstruct.
	if storedColumnsCount < numberOfColumns/2 || storedColumnsCount == numberOfColumns {
		return nil
	}

	// Load the data columns sidecars.
	dataColumnSideCars := make([]*ethpb.DataColumnSidecar, 0, storedColumnsCount)
	for index := range storedColumnsIndices {
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

	// Get the signed block.
	signedBlock, err := s.getSignedBlock(ctx, blockRoot)
	if err != nil {
		return errors.Wrap(err, "get signed block")
	}

	// Reconstruct the data columns sidecars.
	dataColumnSidecars, err := peerdas.DataColumnSidecars(signedBlock, recoveredBlobs)
	if err != nil {
		return errors.Wrap(err, "data column sidecars")
	}

	// Save the data columns sidecars in the database.
	for _, dataColumnSidecar := range dataColumnSidecars {
		roDataColumn, err := blocks.NewRODataColumnWithRoot(dataColumnSidecar, blockRoot)
		if err != nil {
			return errors.Wrap(err, "new read-only data column with root")
		}

		verifiedRoDataColumn := blocks.NewVerifiedRODataColumn(roDataColumn)
		if err := s.cfg.blobStorage.SaveDataColumn(verifiedRoDataColumn); err != nil {
			return errors.Wrap(err, "save column")
		}
	}

	log.WithField("root", fmt.Sprintf("%x", blockRoot)).Debug("Data columns reconstructed successfully")

	return nil
}
