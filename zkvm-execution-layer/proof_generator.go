package zkvmexecutionlayer

import (
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// Each proof system (e.g., RISC Zero, SP1) implements this interface
// to generate proofs for execution payloads from their subnet.
type ProofGenerator interface {
	// Generate a proof for the given execution payload.
	// This is a computationally expensive operation and should be run
	// in a background task (goroutine) by the caller.
	Generate(
		slot primitives.Slot,
		payloadHash []byte,
		blockRoot []byte,
	) (*ethpb.ExecutionProof, error)

	// GetProofId gets the subnet ID this generator produces proofs for.
	GetProofId() primitives.ExecutionProofId
}
