package sync

import (
	"context"
	"fmt"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/feed"
	opfeed "github.com/prysmaticlabs/prysm/v5/beacon-chain/core/feed/operation"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"google.golang.org/protobuf/proto"
)

func (s *Service) dataColumnSubscriber(ctx context.Context, msg proto.Message) error {
	b, ok := msg.(*ethpb.DataColumnSidecar)
	if !ok {
		return fmt.Errorf("message was not type DataColumnSidecar, type=%T", msg)
	}

	// TODO:Change to new one for data columns
	s.setSeenBlobIndex(b.SignedBlockHeader.Header.Slot, b.SignedBlockHeader.Header.ProposerIndex, b.ColumnIndex)

	if err := s.cfg.chain.ReceiveDataColumn(ctx, b); err != nil {
		return err
	}

	s.cfg.operationNotifier.OperationFeed().Send(&feed.Event{
		Type: opfeed.DataColumnSidecarReceived,
		Data: &opfeed.DataColumnSidecarReceivedData{
			DataColumn: b,
		},
	})

	return nil
}
