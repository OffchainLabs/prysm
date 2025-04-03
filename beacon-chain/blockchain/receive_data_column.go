package blockchain

import (
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
)

// ReceiveDataColumns receives a batch of data columns.
func (s *Service) ReceiveDataColumns(dataColumnSidecars []blocks.VerifiedRODataColumn) error {
	if err := s.dataColumnStorage.Save(dataColumnSidecars); err != nil {
		return errors.Wrap(err, "save data column sidecars")
	}

	return nil
}

// ReceiveDataColumn receives a single data column.
// (It is only a wrapper around ReceiveDataColumns.)
func (s *Service) ReceiveDataColumn(dataColumnSidecar blocks.VerifiedRODataColumn) error {
	if err := s.dataColumnStorage.Save([]blocks.VerifiedRODataColumn{dataColumnSidecar}); err != nil {
		return errors.Wrap(err, "save data column sidecars")
	}

	return nil
}
