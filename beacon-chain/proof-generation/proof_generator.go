package proofgeneration

import (
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

func (s *Service) GenerateProofs() ([]*ethpb.ExecutionProof, error) {
	// Check if proofs are required for this epoch
	// Get the list of proof types we should generate

	// Check which proofs are missing/we haven't received yet
	// Check if we already have this proof

	// Generate the required proofs

	// For now, return nil
	return nil, nil
}
