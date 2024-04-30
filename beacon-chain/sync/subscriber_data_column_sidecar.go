package sync

import (
	"context"
	"fmt"

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

	if err := s.cfg.chain.ReceiveDataColumn(ctx, dc); err != nil {
		return err
	}

	s.cfg.operationNotifier.OperationFeed().Send(&feed.Event{
		Type: opfeed.DataColumnSidecarReceived,
		Data: &opfeed.DataColumnSidecarReceivedData{
			DataColumn: &dc,
		},
	})

	return nil
}
