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

	s.setSeenDataColumnIndex(dc.SignedBlockHeader.Header.Slot, dc.SignedBlockHeader.Header.ProposerIndex, dc.ColumnIndex)
	s.setReceivedDataColumn(dc.BlockRoot(), dc.ColumnIndex)

	if err := s.cfg.chain.ReceiveDataColumn(dc); err != nil {
		return errors.Wrap(err, "receive data column")
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
