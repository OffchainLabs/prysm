package sync

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed"
	opfeed "github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/operation"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
)

func (s *Service) blsToExecutionChangeSubscriber(_ context.Context, msg any) error {
	blsMsg, ok := msg.(*ethpb.SignedBLSToExecutionChange)
	if !ok {
		return errors.Errorf("incorrect type of message received, wanted %T but got %T", &ethpb.SignedBLSToExecutionChange{}, msg)
	}
	s.cfg.operationNotifier.OperationFeed().Send(&feed.Event{
		Type: opfeed.BLSToExecutionChangeReceived,
		Data: &opfeed.BLSToExecutionChangeReceivedData{
			Change: blsMsg,
		},
	})
	s.cfg.blsToExecPool.InsertBLSToExecChange(blsMsg)
	return nil
}
