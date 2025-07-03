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
	sidecar, ok := msg.(blocks.VerifiedRODataColumn)
	if !ok {
		return fmt.Errorf("message was not type blocks.VerifiedRODataColumn, type=%T", msg)
	}

	if err := s.receiveDataColumnSidecar(ctx, sidecar); err != nil {
		return errors.Wrap(err, "receive data column")
	}

	slot := sidecar.Slot()
	proposerIndex := sidecar.ProposerIndex()
	root := sidecar.BlockRoot()

	if err := s.reconstructSaveBroadcastDataColumnSidecars(ctx, slot, proposerIndex, root); err != nil {
		return errors.Wrap(err, "reconstruct data columns")
	}

	return nil
}

func (s *Service) receiveDataColumnSidecar(ctx context.Context, sidecar blocks.VerifiedRODataColumn) error {
	slot := sidecar.SignedBlockHeader.Header.Slot
	proposerIndex := sidecar.SignedBlockHeader.Header.ProposerIndex
	columnIndex := sidecar.Index

	s.setSeenDataColumnIndex(slot, proposerIndex, columnIndex)

	if err := s.cfg.chain.ReceiveDataColumn(sidecar); err != nil {
		return errors.Wrap(err, "receive data column")
	}

	s.cfg.operationNotifier.OperationFeed().Send(&feed.Event{
		Type: opfeed.DataColumnSidecarReceived,
		Data: &opfeed.DataColumnSidecarReceivedData{
			DataColumn: &sidecar,
		},
	})

	return nil
}
