package blockchain

import (
	"context"

	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
)

// SendNewBlobEvent sends a message to the BlobNotifier channel that the blob
// for the block root `root` is ready in the database
func (s *Service) sendNewBlobEvent(root [32]byte, index uint64, slot primitives.Slot) {
	s.blobNotifiers.notifyIndex(root, index, slot)
}

// ReceiveBlob saves the blob to database and sends the new event
func (s *Service) ReceiveBlob(ctx context.Context, b blocks.VerifiedROBlob) error {
	if err := s.blobStorage.Save(b); err != nil {
		return err
	}

	s.sendNewBlobEvent(b.BlockRoot(), b.Index, b.Slot())
	return nil
}
