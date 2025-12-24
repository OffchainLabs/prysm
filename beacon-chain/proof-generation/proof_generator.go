package proofgeneration

import (
	"fmt"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

func (s *Service) GenerateProofs(slot primitives.Slot, payloadHash []byte, blockRoot []byte) ([]*ethpb.ExecutionProof, error) {
	// Check if proofs are required for this epoch
	requestedEpoch := slots.ToEpoch(slot)
	if !s.isProofRequiredForEpoch(requestedEpoch) {
		log.WithField("epoch", requestedEpoch).Info("Proof generation not required for this epoch")
		return nil, nil
	}

	// Get the list of proof types we should generate,
	// so check if we already have this proof in the pool
	requiredProofTypes := make([]primitives.ExecutionProofId, 0, len(s.cfg.ProofTypes))
	for _, proofType := range s.cfg.ProofTypes {
		if !s.cfg.ExecProofPool.ProofExists(slot, proofType) {
			requiredProofTypes = append(requiredProofTypes, proofType)
		}

	}

	// Generate the required proofs
	proofs := []*ethpb.ExecutionProof{}
	for _, proofType := range requiredProofTypes {
		generator, found := s.GeneratorRegistry.GetGenerator(proofType)
		if !found {
			return nil, fmt.Errorf("no proof generator registered for proof type %d", proofType)
		}
		proof, err := generator.Generate(
			slot,
			payloadHash,
			blockRoot,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to generate proof for type %d: %w", proofType, err)
		}
		proofs = append(proofs, proof)
	}

	return proofs, nil
}

// isProofRequiredForEpoch checks if proof generation is required for the given epoch.
// This checks against the current epoch and the configured retention policy.
func (s *Service) isProofRequiredForEpoch(epoch primitives.Epoch) bool {
	currentSlot := s.cfg.TimeFetcher.CurrentSlot()
	currentEpoch := slots.ToEpoch(currentSlot)

	proofRetentionEpoch := primitives.Epoch(0)
	if currentEpoch >= primitives.Epoch(params.BeaconConfig().MinEpochsForExecutionProofRequests) {
		proofRetentionEpoch = currentEpoch.Sub(params.BeaconConfig().MinEpochsForExecutionProofRequests)
	}

	boundaryEpoch := primitives.MaxEpoch(params.BeaconConfig().FuluForkEpoch, proofRetentionEpoch)

	return epoch >= boundaryEpoch
}
