package sync

import (
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/pkg/errors"
)

// fetchDataColumnSidecars retrieves data column sidecars from the database for the given map
// of block roots to data column indices. It checks if the requested data columns are available
// in the database and returns the corresponding ReadOnlyDataColumnSidecars.
// If some requested data is not available, it returns an error.
func (s *Service) fetchDataColumnSidecars(blockRootToIndices map[[fieldparams.RootLength]byte][]uint64) (map[[fieldparams.RootLength]byte][]blocks.VerifiedRODataColumn, error) {
	if s.cfg.dataColumnStorage == nil {
		return nil, errors.New("data column storage is nil")
	}

	if len(blockRootToIndices) == 0 {
		return map[[fieldparams.RootLength]byte][]blocks.VerifiedRODataColumn{}, nil
	}

	result := make(map[[fieldparams.RootLength]byte][]blocks.VerifiedRODataColumn)

	minColumnsForReconstruct := peerdas.MinimumColumnsCountToReconstruct()

	for blockRoot, indices := range blockRootToIndices {
		if len(indices) == 0 {
			continue
		}

		// First check cache to see what data is available
		storedDataColumns := s.cfg.dataColumnStorage.Summary(blockRoot)
		
		// Check if all requested indices are present in cache
		storedIndices := storedDataColumns.Stored()
		allRequestedPresent := true
		for _, requestedIndex := range indices {
			if !storedIndices[requestedIndex] {
				allRequestedPresent = false
				break
			}
		}

		if allRequestedPresent {
			// All requested data is present, retrieve only what we need
			verifiedRODataColumns, err := s.cfg.dataColumnStorage.Get(blockRoot, indices)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to get data columns for block root %#x", blockRoot)
			}
			result[blockRoot] = verifiedRODataColumns
			continue
		}

		// Not all requested data is present, check if we can reconstruct
		if storedDataColumns.Count() < minColumnsForReconstruct {
			return nil, errors.New("some requested data columns are not available and insufficient data for reconstruction")
		}

		// Retrieve all stored columns for reconstruction
		allStoredColumns, err := s.cfg.dataColumnStorage.Get(blockRoot, nil)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get all stored columns for reconstruction for block root %#x", blockRoot)
		}

		// Attempt reconstruction
		reconstructedColumns, err := peerdas.ReconstructDataColumnSidecars(allStoredColumns)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to reconstruct data columns for block root %#x", blockRoot)
		}

		// Health check: ensure we have the expected number of columns
		numberOfColumns := params.BeaconConfig().NumberOfColumns
		if uint64(len(reconstructedColumns)) != numberOfColumns {
			return nil, errors.Errorf("reconstructed %d columns but expected %d for block root %#x", len(reconstructedColumns), numberOfColumns, blockRoot)
		}

		// Extract only the requested indices from reconstructed data using direct indexing
		requestedColumns := make([]blocks.VerifiedRODataColumn, 0, len(indices))
		for _, requestedIndex := range indices {
			if requestedIndex >= numberOfColumns {
				return nil, errors.Errorf("requested column index %d exceeds maximum %d for block root %#x", requestedIndex, numberOfColumns-1, blockRoot)
			}
			requestedColumns = append(requestedColumns, reconstructedColumns[requestedIndex])
		}

		result[blockRoot] = requestedColumns
	}

	return result, nil
}