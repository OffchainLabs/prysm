package sync

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed"
	opfeed "github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/operation"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

func (s *Service) executionProofSubscriber(_ context.Context, msg proto.Message) error {
	verifiedRoSignedExecutionProof, ok := msg.(blocks.VerifiedROSignedExecutionProof)
	if !ok {
		return errors.Errorf("incorrect type of message received, wanted %T but got %T", blocks.VerifiedROSignedExecutionProof{}, msg)
	}

	// Insert the execution proof into the pool
	s.setSeenValidProof(&verifiedRoSignedExecutionProof.ROSignedExecutionProof)

	// Save the proof to storage.
	if err := s.cfg.chain.ReceiveProof(verifiedRoSignedExecutionProof); err != nil {
		return errors.Wrap(err, "receive proof")
	}

	// Notify subscribers about the new execution proof
	s.cfg.operationNotifier.OperationFeed().Send(&feed.Event{
		Type: opfeed.ExecutionProofReceived,
		Data: &opfeed.ExecutionProofReceivedData{
			ExecutionProof: &verifiedRoSignedExecutionProof,
		},
	})

	return nil
}
