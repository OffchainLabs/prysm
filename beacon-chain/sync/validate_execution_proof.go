package sync

import (
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/sirupsen/logrus"
)

func (s *Service) validateExecutionProof(ctx context.Context, pid peer.ID, msg *pubsub.Message) (pubsub.ValidationResult, error) {
	// Always accept our own messages.
	if pid == s.cfg.p2p.PeerID() {
		return pubsub.ValidationAccept, nil
	}

	// Ignore messages during initial sync.
	if s.cfg.initialSync.Syncing() {
		return pubsub.ValidationIgnore, nil
	}

	// Reject messages with a nil topic.
	if msg.Topic == nil {
		return pubsub.ValidationReject, p2p.ErrInvalidTopic
	}

	// Decode the message, reject if it fails.
	m, err := s.decodePubsubMessage(msg)
	if err != nil {
		log.WithError(err).Error("Failed to decode message")
		return pubsub.ValidationReject, err
	}

	// Reject messages that are not of the expected type.
	executionProof, ok := m.(*ethpb.ExecutionProof)
	if !ok {
		log.WithField("message", m).Error("Message is not of type *ethpb.ExecutionProof")
		return pubsub.ValidationReject, errWrongMessage
	}

	// Convert to ROExecutionProof.
	roProof, err := blocks.NewROExecutionProof(executionProof)
	if err != nil {
		return pubsub.ValidationReject, err
	}

	// Check if the proof has already been seen.
	if s.hasSeenProof(roProof.BlockRoot(), roProof.ProofId()) {
		return pubsub.ValidationIgnore, nil
	}

	// Create the verifier with gossip requirements.
	verifier := s.newProofsVerifier([]blocks.ROExecutionProof{roProof}, verification.GossipExecutionProofRequirements)

	// Run verifications.
	if err := verifier.NotFromFutureSlot(); err != nil {
		return pubsub.ValidationReject, err
	}
	if err := verifier.ProofSizeLimits(); err != nil {
		return pubsub.ValidationReject, err
	}
	if err := verifier.ProofVerified(); err != nil {
		return pubsub.ValidationReject, err
	}

	// Get verified proofs.
	verifiedProofs, err := verifier.VerifiedROExecutionProofs()
	if err != nil {
		return pubsub.ValidationIgnore, err
	}

	log.WithFields(logrus.Fields{
		"root": fmt.Sprintf("%#x", roProof.BlockRoot()),
		"slot": roProof.Slot(),
		"id":   roProof.ProofId(),
	}).Debug("Accepted execution proof")

	// Set validator data to the verified proof.
	msg.ValidatorData = verifiedProofs[0]
	return pubsub.ValidationAccept, nil
}

// hasSeenProof returns true if the proof with the same block root and proof ID has been seen before.
func (s *Service) hasSeenProof(blockRoot [32]byte, proofId primitives.ExecutionProofId) bool {
	key := computeProofCacheKey(blockRoot, proofId)
	_, seen := s.seenProofCache.Get(key)
	return seen
}

// setSeenProof marks the proof with the given block root and proof ID as seen.
func (s *Service) setSeenProof(slot primitives.Slot, blockRoot [32]byte, proofId primitives.ExecutionProofId) {
	key := computeProofCacheKey(blockRoot, proofId)
	s.seenProofCache.Add(slot, key, true)
}

func computeProofCacheKey(blockRoot [32]byte, proofId primitives.ExecutionProofId) string {
	key := make([]byte, 0, 33)

	key = append(key, blockRoot[:]...)
	key = append(key, bytesutil.Bytes1(uint64(proofId))...)

	return string(key)
}
