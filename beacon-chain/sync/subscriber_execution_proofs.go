package sync

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed"
	opfeed "github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/operation"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

func (s *Service) executionProofSubscriber(_ context.Context, msg proto.Message) error {
	executionProof, ok := msg.(*ethpb.ExecutionProof)
	if !ok {
		return errors.Errorf("incorrect type of message received, wanted %T but got %T", &ethpb.ExecutionProof{}, msg)
	}

	// Mark the proof as seen to avoid reprocessing
	s.setSeenExecutionProofIndex(executionProof.ProofId, executionProof.Slot)

	// Notify subscribers about the new execution proof
	s.cfg.operationNotifier.OperationFeed().Send(&feed.Event{
		Type: opfeed.ExecutionProofReceived,
		Data: &opfeed.ExecutionProofReceivedData{
			ExecutionProof: executionProof,
		},
	})

	// Insert the execution proof into the pool
	s.cfg.execProofPool.InsertExecutionProof(executionProof)
	return nil
}
