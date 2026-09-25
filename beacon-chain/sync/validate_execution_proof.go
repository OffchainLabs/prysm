package sync

import (
	"context"
	"errors"
	"strconv"

	"github.com/OffchainLabs/prysm/v7/api/client/proofnode"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/eip8025"
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
	proof, err := blocks.NewROSignedExecutionProofEnvelope(signed)
	if err != nil {
		return pubsub.ValidationReject, err
	}

	blockRoot := proof.BeaconBlockRoot()
	proofType := proof.ProofType()

	// [IGNORE] The proof has not already been processed.
	envelopeRoot, err := proof.EnvelopeRoot()
	if err != nil {
		return pubsub.ValidationReject, err
	}
	if s.hasSeenExecutionProof(envelopeRoot) {
		return pubsub.ValidationIgnore, nil
	}

	// [IGNORE] This is the prover's first proof for this block and proof type.
	proverKey := executionProofProverKey(blockRoot, proofType, signed.ValidatorIndex)
	if s.hasSeenExecutionProofProver(proverKey) {
		return pubsub.ValidationIgnore, nil
	}

	// [IGNORE] The proof's beacon block has been seen.
	if !s.cfg.chain.HasBlock(ctx, blockRoot) {
		return pubsub.ValidationIgnore, nil
	}

	// [IGNORE] No verified proof of this type is known for this beacon block.
	if s.cfg.executionProofCache.Has(blockRoot, proofType) {
		return pubsub.ValidationIgnore, nil
	}

	// [IGNORE] The proof's execution payload is available. Its new payload
	// request root is recorded when the payload envelope is imported, so a miss
	// means this node cannot yet judge proofs for the block.
	newPayloadRequestRoot, ok := s.cfg.executionProofCache.NewPayloadRequestRoot(blockRoot)
	if !ok {
		return pubsub.ValidationIgnore, nil
	}

	// [REJECT] The proof's beacon block has passed consensus validation, and the
	// envelope passes validation against its state.
	blockSlot, err := s.cfg.chain.RecentBlockSlot(blockRoot)
	if err != nil {
		return pubsub.ValidationIgnore, err
	}
	st, err := s.cfg.chain.PtcLookupState(ctx, blockRoot, blockSlot)
	if err != nil {
		return pubsub.ValidationIgnore, err
	}
	if err := eip8025.VerifyExecutionProofEnvelope(st, proof); err != nil {
		return pubsub.ValidationReject, err
	}

	// Mark the authenticated proof and prover attempt as seen, so that a proof
	// the node is already verifying is not verified again concurrently.
	s.setSeenExecutionProof(envelopeRoot)
	s.setSeenExecutionProofProver(proverKey)

	// [REJECT] The execution proof is valid.
	if err := s.verifyExecutionProof(ctx, newPayloadRequestRoot, proof); err != nil {
		if errors.Is(err, proofnode.ErrProofInvalid) {
			return pubsub.ValidationReject, err
		}
		// The proof node could not reach a verdict. Withhold judgement rather
		// than penalise the sender for this node's own unavailability.
		log.WithError(err).Debug("Could not verify execution proof")
		return pubsub.ValidationIgnore, err
	}

	// The subscriber re-wraps the message, so hand on the proto itself.
	msg.ValidatorData = signed
	return pubsub.ValidationAccept, nil
}

// verifyExecutionProof asks the proof node to verify the proof against the new
// payload request the payload envelope produced.
func (s *Service) verifyExecutionProof(
	ctx context.Context,
	newPayloadRequestRoot [32]byte,
	proof blocks.ROSignedExecutionProofEnvelope,
) error {
	if s.cfg.proofNode == nil {
		return errors.New("no proof node configured")
	}
	return s.cfg.proofNode.VerifyProof(
		ctx,
		newPayloadRequestRoot,
		proof.ProofType(),
		proof.Message.ProofData,
		proofnode.ChainConfigAt(s.cfg.clock.GenesisTime()),
	)
}

func executionProofProverKey(blockRoot [32]byte, proofType ethpb.ProofType, prover primitives.ValidatorIndex) string {
	return string(blockRoot[:]) + string(byte(proofType)) + strconv.FormatUint(uint64(prover), 10)
}

func (s *Service) hasSeenExecutionProof(proofRoot [32]byte) bool {
	_, seen := s.seenExecutionProofCache.Get(proofRoot)
	return seen
}

func (s *Service) setSeenExecutionProof(proofRoot [32]byte) {
	s.seenExecutionProofCache.Add(proofRoot, true)
}

func (s *Service) hasSeenExecutionProofProver(key string) bool {
	_, seen := s.seenExecutionProofProverCache.Get(key)
	return seen
}

func (s *Service) setSeenExecutionProofProver(key string) {
	s.seenExecutionProofProverCache.Add(key, true)
}
