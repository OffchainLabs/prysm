package zkvmexecutionlayer

import (
	executionproof "github.com/OffchainLabs/prysm/v6/consensus-types/execution-proof"
)

/// Trait for proof verification (one implementation per zkVM+EL combination)
type ProofVerifier interface {
	// Verify that the proof is valid for the given execution payload
	//
    // Returns :
    // - true if valid,
    // - false if invalid (but well-formed)
    // - error if the proof is malformed or verification cannot be performed.
    Verifier(
     	proof executionproof.ExecutionProof,
    ) (bool, error)
    
    // GetProofId gets the proof ID this verifier produces proofs for.
	GetProofId() executionproof.ExecutionProofId
}