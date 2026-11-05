package blockchain

import (
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/pkg/errors"
)

// ReceiveDataColumns receives a batch of data columns.
func (s *Service) ReceiveDataColumns(dataColumnSidecars []blocks.VerifiedRODataColumn) error {
	// Storage prunes relative to the highest stored epoch, so future columns would prune current ones.
	for _, sidecar := range dataColumnSidecars {
		if sidecar.Slot() > s.CurrentSlot()+1 {
			return errors.Errorf("data column sidecar slot %d is in the future", sidecar.Slot())
		}
	}

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
