package sync

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/feed"
	opfeed "github.com/prysmaticlabs/prysm/v5/beacon-chain/core/feed/operation"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"google.golang.org/protobuf/proto"
)

func (s *Service) dataColumnSubscriber(ctx context.Context, msg proto.Message) error {
	dc, ok := msg.(blocks.VerifiedRODataColumn)
	if !ok {
		return fmt.Errorf("message was not type blocks.VerifiedRODataColumn, type=%T", msg)
	}

	slot := dc.SignedBlockHeader.Header.Slot
	proposerIndex := dc.SignedBlockHeader.Header.ProposerIndex
	columnIndex := dc.ColumnIndex
	blockRoot := dc.BlockRoot()

	s.setSeenDataColumnIndex(slot, proposerIndex, columnIndex)

	if err := s.cfg.chain.ReceiveDataColumn(dc); err != nil {
		return errors.Wrap(err, "receive data column")
	}

	// Mark the data column as both received and stored.
	if err := s.setReceivedDataColumn(blockRoot, columnIndex); err != nil {
		return errors.Wrap(err, "set received data column")
	}

	if err := s.setStoredDataColumn(blockRoot, columnIndex); err != nil {
		return errors.Wrap(err, "set stored data column")
	}

	s.cfg.operationNotifier.OperationFeed().Send(&feed.Event{
		Type: opfeed.DataColumnSidecarReceived,
		Data: &opfeed.DataColumnSidecarReceivedData{
			DataColumn: &dc,
		},
	})

	// Reconstruct the data columns if needed.
	if err := s.reconstructDataColumns(ctx, dc); err != nil {
		return errors.Wrap(err, "reconstruct data columns")
	}

	return nil
}
