package proofgeneration

import (
	"fmt"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

func (s *Service) GenerateProofs(slot primitives.Slot, payloadHash []byte, blockRoot []byte) ([]*ethpb.ExecutionProof, error) {
	// Check if proofs are required for this epoch
	// Get the list of proof types we should generate

	// TODO: For now, we generate all proofs configured in the service.
	requiredProofTypes := s.cfg.ProofTypes

	// Check which proofs are missing/we haven't received yet
	// Check if we already have this proof

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
