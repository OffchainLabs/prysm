package sync

import (
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
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

	// 2. Verify proof slot is greater than finalized slot
	if err := s.proofAboveFinalizedSlot(ctx, executionProof); err != nil {
		return pubsub.ValidationReject, err
	}

	// 3. Check if proof is already known via gossip
	if s.hasSeenExecutionProofIndex(executionProof.ProofId, executionProof.Slot) {
		return pubsub.ValidationIgnore, nil
	}

	// 4. Check if the proof is already in the DA checker cache (execution proof pool)
	// If it exists in the cache, we know it has already passed validation.
	if s.isProofCachedInPool(executionProof.ProofId, executionProof.Slot) {
		return pubsub.ValidationIgnore, nil
	}

	// 5. Verify proof size limits
	if uint64(len(executionProof.ProofData)) > params.BeaconConfig().MaxProofDataBytes {
		return pubsub.ValidationReject, fmt.Errorf("execution proof data size %d exceeds maximum allowed %d", len(executionProof.ProofData), params.BeaconConfig().MaxProofDataBytes)
	}

	// 6. Run zkVM proof verification
	if err := s.verifyExecutionProof(executionProof); err != nil {
		return pubsub.ValidationReject, err
	}

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
		return fmt.Errorf("failed to compute start time for proof slot %d: %w", proofSlot, err)
	}

	earliestStart = earliestStart.Add(-1 * params.BeaconConfig().MaximumGossipClockDisparityDuration())
	// If the system time is still before earliestStart, we consider the proof from a future slot and return an error.
	if s.cfg.clock.Now().Before(earliestStart) {
		return fmt.Errorf("slot %d is too far in the future (current slot: %d)", proofSlot, currentSlot)
	}
	return nil
}

// proofAboveFinalizedSlot checks whether the execution proof's slot is after the finalized slot.
func (s *Service) proofAboveFinalizedSlot(ctx context.Context, executionProof *ethpb.ExecutionProof) error {
	finalizedCheckpoint, err := s.cfg.beaconDB.FinalizedCheckpoint(ctx)
	if err != nil {
		return fmt.Errorf("failed to get finalized checkpoint: %w", err)
	}
	fSlot, err := slots.EpochStart(finalizedCheckpoint.Epoch)
	if err != nil {
		return fmt.Errorf("failed to compute start slot for finalized epoch %d: %w", finalizedCheckpoint.Epoch, err)
	}

	if executionProof.Slot <= fSlot {
		return fmt.Errorf("execution proof slot %d is not after finalized slot %d", executionProof.Slot, fSlot)
	}
	return nil
}

// hasSeenExecutionProofIndex checks whether we have already seen the execution proof for the given slot.
func (s *Service) hasSeenExecutionProofIndex(proofId primitives.ExecutionProofId, slot primitives.Slot) bool {
	s.seenExecutionProofLock.RLock()
	defer s.seenExecutionProofLock.RUnlock()
	b := append(bytesutil.Bytes32(uint64(proofId)), bytesutil.Bytes32(uint64(slot))...)
	_, seen := s.seenExecutionProofCache.Get(string(b))
	return seen
}

// setSeenExecutionProofIndex marks the execution proof for the given slot as seen.
func (s *Service) setSeenExecutionProofIndex(proofId primitives.ExecutionProofId, slot primitives.Slot) {
	s.seenExecutionProofLock.Lock()
	defer s.seenExecutionProofLock.Unlock()
	b := append(bytesutil.Bytes32(uint64(proofId)), bytesutil.Bytes32(uint64(slot))...)
	s.seenExecutionProofCache.Add(string(b), true)
}

// isProofCachedInPool checks if the execution proof is already present in the pool.
func (s *Service) isProofCachedInPool(proofId primitives.ExecutionProofId, slot primitives.Slot) bool {
	return s.cfg.execProofPool.ProofExists(slot, proofId)
}

// verifyExecutionProof performs the actual verification of the execution proof.
// It uses verifier implementations to validate the proof.
// TODO: Implement the actual verification logic.
func (s *Service) verifyExecutionProof(executionProof *ethpb.ExecutionProof) error {
	return nil
}
