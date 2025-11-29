package zkvmexecutionlayer

import (
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// / Trait for proof verification (one implementation per zkVM+EL combination)
type ProofVerifier interface {
	// Verify that the proof is valid for the given execution payload
	//
	// Returns :
	// - true if valid,
	// - false if invalid (but well-formed)
	// - error if the proof is malformed or verification cannot be performed.
	Verify(
		proof *ethpb.ExecutionProof,
	) (bool, error)

	// GetProofId gets the proof ID this verifier produces proofs for.
	GetProofId() primitives.ExecutionProofId
}
