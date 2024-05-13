package blockchain

import (
	"context"

	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
)

func (s *Service) ReceiveDataColumn(ctx context.Context, ds blocks.VerifiedRODataColumn) error {
	if err := s.blobStorage.SaveDataColumn(ds); err != nil {
		return err
	}

	// TODO use a custom event or new method of for data columns. For speed
	// we are simply reusing blob paths here.
	s.sendNewBlobEvent(ds.BlockRoot(), ds.ColumnIndex)
	return nil
}
