package sync

import (
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/sirupsen/logrus"
)

func (s *Service) validateExecutionProof(ctx context.Context, pid peer.ID, msg *pubsub.Message) (pubsub.ValidationResult, error) {
	// Always accept messages our own messages.
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

	// 1. Verify proof is not from the future
	if err := s.proofNotFromFutureSlot(executionProof); err != nil {
		return pubsub.ValidationReject, err
	}

	// 3. Check if the proof is already in the DA checker cache (execution proof pool)
	// If it exists in the cache, we know it has already passed validation.
	blockRoot := bytesutil.ToBytes32(executionProof.BlockRoot)
	if s.hasSeenProof(blockRoot, executionProof.ProofId) {
		return pubsub.ValidationIgnore, nil
	}

	// 4. Verify proof size limits
	if uint64(len(executionProof.ProofData)) > params.BeaconConfig().MaxProofDataBytes {
		return pubsub.ValidationReject, fmt.Errorf("execution proof data size %d exceeds maximum allowed %d", len(executionProof.ProofData), params.BeaconConfig().MaxProofDataBytes)
	}

	// 5. Run zkVM proof verification
	if err := s.verifyExecutionProof(executionProof); err != nil {
		return pubsub.ValidationReject, err
	}

	log.WithFields(logrus.Fields{
		"root": fmt.Sprintf("%#x", blockRoot),
		"slot": executionProof.Slot,
		"id":   executionProof.ProofId,
	}).Debug("Accepted execution proof")

	// Validation successful, return accept
	msg.ValidatorData = executionProof
	return pubsub.ValidationAccept, nil
}

// TODO: Do we need encapsulation for all those verification functions?

// proofNotFromFutureSlot checks whether the execution proof is from a future slot.
func (s *Service) proofNotFromFutureSlot(executionProof *ethpb.ExecutionProof) error {
	currentSlot := s.cfg.clock.CurrentSlot()
	proofSlot := executionProof.Slot

	if currentSlot == proofSlot {
		return nil
	}

	earliestStart, err := s.cfg.clock.SlotStart(proofSlot)
	if err != nil {
		// TODO: Should we penalize the peer for this?
		return fmt.Errorf("failed to compute start time for proof slot %d: %w", proofSlot, err)
	}

	earliestStart = earliestStart.Add(-1 * params.BeaconConfig().MaximumGossipClockDisparityDuration())
	// If the system time is still before earliestStart, we consider the proof from a future slot and return an error.
	if s.cfg.clock.Now().Before(earliestStart) {
		return fmt.Errorf("slot %d is too far in the future (current slot: %d)", proofSlot, currentSlot)
	}
	return nil
}

// Returns true if the column with the same slot, proposer index, and column index has been seen before.
func (s *Service) hasSeenProof(blockRoot [32]byte, proofId primitives.ExecutionProofId) bool {
	key := computeProofCacheKey(blockRoot, proofId)
	_, seen := s.seenProofCache.Get(key)
	return seen
}

// Sets the data column with the same slot, proposer index, and data column index as seen.
func (s *Service) setSeenProof(slot primitives.Slot, blockRoot [32]byte, proofId primitives.ExecutionProofId) {
	key := computeProofCacheKey(blockRoot, proofId)
	s.seenProofCache.Add(slot, key, true)
}

// verifyExecutionProof performs the actual verification of the execution proof.
func (s *Service) verifyExecutionProof(_ *ethpb.ExecutionProof) error {
	// For now, say all proof are valid.
	return nil
}

func computeProofCacheKey(blockRoot [32]byte, proofId primitives.ExecutionProofId) string {
	key := make([]byte, 0, 33)

	key = append(key, blockRoot[:]...)
	key = append(key, bytesutil.Bytes1(uint64(proofId))...)

	return string(key)
}
