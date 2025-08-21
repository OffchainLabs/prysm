package blockchain

import (
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/pkg/errors"
)

// ReceiveDataColumns receives a batch of data columns.
func (s *Service) ReceiveDataColumns(dataColumnSidecars []blocks.VerifiedRODataColumn) error {
	if err := s.dataColumnStorage.Save(dataColumnSidecars); err != nil {
		return errors.Wrap(err, "save data column sidecars")
	}

	return nil
}

// ReceiveDataColumn receives a single data column.
func (s *Service) ReceiveDataColumn(dataColumnSidecar blocks.VerifiedRODataColumn) error {
	if err := s.dataColumnStorage.Save([]blocks.VerifiedRODataColumn{dataColumnSidecar}); err != nil {
		return errors.Wrap(err, "save data column sidecar")
	}

	return nil
}
