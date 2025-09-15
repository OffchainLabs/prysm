package sync

import (
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/feed"
	opfeed "github.com/OffchainLabs/prysm/v6/beacon-chain/core/feed/operation"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
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
		return errors.Wrap(err, "receive data column sidecar")
	}

	if err := s.reconstructSaveBroadcastDataColumnSidecars(ctx, sidecar); err != nil {
		return errors.Wrap(err, "reconstruct/save/broadcast data column sidecars")
	}

	source := peerdas.PopulateFromSidecar(sidecar)

	key := fmt.Sprintf("%#x", sidecar.BlockRoot())
	if _, err, _ := s.columnSidecarsExecSingleFlight.Do(key, func() (interface{}, error) {
		if err := s.processDataColumnSidecarsFromExecution(ctx, source); err != nil {
			return nil, err
		}

		return nil, nil
	}); err != nil {
		return errors.Wrap(err, "process data column sidecars from execution from sidecar")
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
