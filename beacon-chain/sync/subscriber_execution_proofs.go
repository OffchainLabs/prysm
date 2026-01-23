package sync

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed"
	opfeed "github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/operation"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

func (s *Service) executionProofSubscriber(_ context.Context, msg proto.Message) error {
	executionProof, ok := msg.(*ethpb.ExecutionProof)
	if !ok {
		return errors.Errorf("incorrect type of message received, wanted %T but got %T", &ethpb.ExecutionProof{}, msg)
	}

	// Insert the execution proof into the pool
	s.setSeenProof(executionProof.Slot, bytesutil.ToBytes32(executionProof.BlockRoot), executionProof.ProofId)

	// Save the proof to storage.
	if err := s.cfg.chain.ReceiveProof(executionProof); err != nil {
		return errors.Wrap(err, "receive proof")
	}

	// Notify subscribers about the new execution proof
	s.cfg.operationNotifier.OperationFeed().Send(&feed.Event{
		Type: opfeed.ExecutionProofReceived,
		Data: &opfeed.ExecutionProofReceivedData{
			ExecutionProof: executionProof,
		},
	})

	return nil
}
