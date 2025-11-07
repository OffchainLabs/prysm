package zkvmexecutionlayer

import (
	executionproof "github.com/OffchainLabs/prysm/v6/consensus-types/execution-proof"
	"github.com/ethereum/go-ethereum/common"
)

// Each proof system (e.g., RISC Zero, SP1) implements this interface
// to generate proofs for execution payloads from their subnet.
type ProofGenerator interface {
	// Generate a proof for the given execution payload.
	// This is a computationally expensive operation and should be run
	// in a background task (goroutine) by the caller.
	Generate(
		payloadHash common.Hash,
		blockRoot common.Hash,
	) (*executionproof.ExecutionProof, error)

	// SubnetId gets the subnet ID this generator produces proofs for.
	SubnetId() executionproof.ExecutionProofSubnetId
}