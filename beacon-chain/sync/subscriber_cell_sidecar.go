package sync

import (
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"google.golang.org/protobuf/proto"
)

func (s *Service) cellSubscriber(ctx context.Context, msg proto.Message) error {
	// Check message type
	vrc, ok := msg.(blocks.VerifiedROCell)
	if !ok {
		return fmt.Errorf("message was not type blocks.VerifiedROCell, type=%T", msg)
	}

	// Prevent dup / Store
	s.receiveCell(ctx, vrc)

	// No logic construction yet

	return nil
}

func (s *Service) receiveCell(ctx context.Context, vrc blocks.VerifiedROCell) {
	txHash := vrc.TxHash
	blobIndex := vrc.BlobIndex
	columnIndex := vrc.ColumnIndex

	s.setSeenCellIndex(txHash, blobIndex, columnIndex)

	// 스토리지 저장
	s.cfg.stagedCellCache.Set(vrc)

	// Removed event notifier
}
