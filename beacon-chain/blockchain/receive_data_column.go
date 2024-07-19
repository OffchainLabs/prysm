package blockchain

import (
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
)

func (s *Service) ReceiveDataColumn(ds blocks.VerifiedRODataColumn) error {
	if err := s.blobStorage.SaveDataColumn(ds); err != nil {
		return errors.Wrap(err, "save data column")
	}

	return nil
}
