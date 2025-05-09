package sync

import (
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/feed"
	opfeed "github.com/OffchainLabs/prysm/v6/beacon-chain/core/feed/operation"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

func (s *Service) dataColumnSubscriber(ctx context.Context, msg proto.Message) error {
	dc, ok := msg.(blocks.VerifiedRODataColumn)
	if !ok {
		return fmt.Errorf("message was not type blocks.VerifiedRODataColumn, type=%T", msg)
	}

	if err := s.receiveDataColumn(ctx, dc); err != nil {
		return errors.Wrap(err, "receive data column")
	}

	// Reconstruct the data columns if needed.
	if err := s.reconstructDataColumns(ctx, dc); err != nil {
		return errors.Wrap(err, "reconstruct data columns")
	}

	return nil
}

func (s *Service) receiveDataColumn(ctx context.Context, dc blocks.VerifiedRODataColumn) error {
	slot := dc.SignedBlockHeader.Header.Slot
	proposerIndex := dc.SignedBlockHeader.Header.ProposerIndex
	columnIndex := dc.Index

	s.setSeenDataColumnIndex(slot, proposerIndex, columnIndex)

	if err := s.cfg.chain.ReceiveDataColumn(dc); err != nil {
		return errors.Wrap(err, "receive data column")
	}

	s.cfg.operationNotifier.OperationFeed().Send(&feed.Event{
		Type: opfeed.DataColumnSidecarReceived,
		Data: &opfeed.DataColumnSidecarReceivedData{
			DataColumn: &dc,
		},
	})

	return nil
}
