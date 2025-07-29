package sync

import (
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/pkg/errors"
)

// fetchDataColumnSidecars retrieves data column sidecars from the database for the given map
// of block roots to data column indices. It checks if the requested data columns are available
// in the database and returns the corresponding ReadOnlyDataColumnSidecars.
// If some requested data is not available, it returns an error.
func (s *Service) fetchDataColumnSidecars(blockRootToIndices map[[fieldparams.RootLength]byte][]uint64) ([]blocks.VerifiedRODataColumn, error) {
	if s.cfg.dataColumnStorage == nil {
		return nil, errors.New("data column storage is nil")
	}

	if len(blockRootToIndices) == 0 {
		return []blocks.VerifiedRODataColumn{}, nil
	}

	// Calculate total capacity needed for all requested columns
	totalCapacity := 0
	for _, indices := range blockRootToIndices {
		totalCapacity += len(indices)
	}
	allDataColumns := make([]blocks.VerifiedRODataColumn, 0, totalCapacity)

	for blockRoot, indices := range blockRootToIndices {
		if len(indices) == 0 {
			continue
		}

		verifiedRODataColumns, err := s.cfg.dataColumnStorage.Get(blockRoot, indices)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get data columns for block root %#x", blockRoot)
		}

		if len(verifiedRODataColumns) != len(indices) {
			return nil, errors.New("some requested data columns are not available")
		}

		allDataColumns = append(allDataColumns, verifiedRODataColumns...)
	}

	return allDataColumns, nil
}