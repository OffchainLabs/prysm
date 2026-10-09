package sync

import (
	"context"
	"errors"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
)

// validateExecutionProof validates a SignedExecutionProofEnvelope for gossip
// propagation.
// https://github.com/ethereum/consensus-specs/blob/master/specs/_features/eip8025/p2p-interface.md#new-execution_proof
func (s *Service) validateExecutionProof(ctx context.Context, pid peer.ID, msg *pubsub.Message) (pubsub.ValidationResult, error) {
	if pid == s.cfg.p2p.PeerID() {
		return pubsub.ValidationAccept, nil
	}
	if s.cfg.initialSync.Syncing() {
		return pubsub.ValidationIgnore, nil
	}

	ctx, span := trace.StartSpan(ctx, "sync.validateExecutionProof")
	defer span.End()

	if msg.Topic == nil {
		return pubsub.ValidationReject, p2p.ErrInvalidTopic
	}

	m, err := s.decodePubsubMessage(msg)
	if err != nil {
		return pubsub.ValidationReject, err
	}
	signed, ok := m.(*ethpb.SignedExecutionProofEnvelope)
	if !ok {
		return pubsub.ValidationReject, errWrongMessage
	}
	// [REJECT] The proof data is non-empty.
	// [REJECT] The proof type is supported.
	proof, err := blocks.NewROSignedExecutionProofEnvelope(signed)
	if err != nil {
		return pubsub.ValidationReject, err
	}

	blockRoot := proof.BeaconBlockRoot()
	proofType := proof.ProofType()

	// [IGNORE] The proof's beacon block has been seen.
	if !s.cfg.chain.HasBlock(ctx, blockRoot) {
		return pubsub.ValidationIgnore, nil
	}

	// [IGNORE] No valid proof is known for this beacon block and proof type.
	if s.cfg.chain.HasExecutionProofType(blockRoot, proofType) {
		return pubsub.ValidationIgnore, nil
	}

	// [IGNORE] The proof has not already been processed.
	envelopeRoot, err := proof.EnvelopeRoot()
	if err != nil {
		return pubsub.ValidationReject, err
	}
	if _, seen := s.seenExecutionProofCache.Get(envelopeRoot); seen {
		return pubsub.ValidationIgnore, nil
	}

	// [IGNORE] This is the prover's first valid or invalid proof for this beacon block and proof type.
	proverKey := executionProofProverKey{blockRoot: blockRoot, proofType: proofType, prover: signed.ValidatorIndex}
	if _, seen := s.seenExecutionProofProverCache.Get(proverKey); seen {
		return pubsub.ValidationIgnore, nil
	}

	// [IGNORE] The proof's execution payload is available.
	// [REJECT] The execution proof envelope passes validation.
	newPayloadRequestRoot, err := s.cfg.chain.CheckExecutionProofEnvelope(ctx, proof)
	switch {
	case errors.Is(err, blockchain.ErrExecutionProofBlockUnknown), errors.Is(err, blockchain.ErrExecutionProofPayloadUnavailable):
		return pubsub.ValidationIgnore, nil
	case errors.Is(err, blockchain.ErrInvalidExecutionProofEnvelope):
		return pubsub.ValidationReject, err
	case err != nil:
		return pubsub.ValidationIgnore, err
	}

	// Mark the authenticated proof and prover attempt as seen, so that a proof
	// the node is already verifying is not verified again concurrently.
	s.seenExecutionProofCache.Add(envelopeRoot, true)
	s.seenExecutionProofProverCache.Add(proverKey, true)

	// [REJECT] The execution proof is valid.
	err = s.cfg.chain.VerifyExecutionProof(ctx, newPayloadRequestRoot, proof)
	if errors.Is(err, blockchain.ErrInvalidExecutionProof) {
		return pubsub.ValidationReject, err
	}
	if err != nil {
		// The node could not reach a verdict, for instance because no verifier is
		// configured for this proof type. Withhold judgement rather than penalise
		// the sender for this node's own configuration.
		log.WithError(err).Debug("Could not verify execution proof")
		return pubsub.ValidationIgnore, err
	}

	msg.ValidatorData = proof
	return pubsub.ValidationAccept, nil
}

// executionProofProverKey identifies a prover's proof of one type for one beacon block.
type executionProofProverKey struct {
	blockRoot [32]byte
	proofType ethpb.ProofType
	prover    primitives.ValidatorIndex
}
