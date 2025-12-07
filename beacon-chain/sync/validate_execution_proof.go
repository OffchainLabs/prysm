package sync

import (
	"context"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
)

func (s *Service) validateExecutionProof(ctx context.Context, pid peer.ID, msg *pubsub.Message) (pubsub.ValidationResult, error) {
	// Validation runs on publish (not just subscriptions), so we should approve any message from
	// ourselves.
	if pid == s.cfg.p2p.PeerID() {
		return pubsub.ValidationAccept, nil
	}

	// The head state will be too far away to validate any execution change.
	if s.cfg.initialSync.Syncing() {
		return pubsub.ValidationIgnore, nil
	}

	ctx, span := trace.StartSpan(ctx, "sync.validateExecutionProof")
	defer span.End()

	m, err := s.decodePubsubMessage(msg)
	if err != nil {
		tracing.AnnotateError(span, err)
		return pubsub.ValidationReject, err
	}

	executionProof, ok := m.(*ethpb.ExecutionProof)
	if !ok {
		return pubsub.ValidationReject, errWrongMessage
	}

	// #ETODO
	// For each (block_hash, subnet_id) pair, the node should only forward the first valid proof it sees.
	// 
	// We only accept execution proofs if they come from a validator that actually exists in the current validator set.
	// 
	// Verify that the validator really signed this proof.
	// 
	// The zk-proof must actually contain real proof bytes. (It must not be empty)
	// 
	// Proof type must match the subnet ID
	// 
	// Execution proof must be valid
	
	msg.ValidatorData = executionProof
	return pubsub.ValidationAccept, nil
}
