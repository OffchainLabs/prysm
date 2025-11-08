package zkvmexecutionlayer

import (
	"fmt"
	"time"

	executionproof "github.com/OffchainLabs/prysm/v6/consensus-types/execution-proof"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/ethereum/go-ethereum/common"
)


const (
	// defaultGenerationDelay simulates some proof generation work.
	defaultGenerationDelay = 50 * time.Millisecond
)

// DummyProofGenerator is a test implementation of the ProofGenerator interface.
// It simulates the proof generation process with a configurable delay
// and creates dummy proofs.
type DummyProofGenerator struct {
	ProofId        executionproof.ExecutionProofId
	GenerationDelay time.Duration
}

// NewDummyProofGenerator creates a new dummy generator for the specified subnet
// with a default delay.
func NewDummyProofGenerator(proofId executionproof.ExecutionProofId) *DummyProofGenerator {
	return &DummyProofGenerator{
		ProofId:        proofId,
		GenerationDelay: defaultGenerationDelay,
	}
}

// NewDummyProofGeneratorWithDelay creates a new dummy generator with a custom
// generation delay.
func NewDummyProofGeneratorWithDelay(proofId executionproof.ExecutionProofId, delay time.Duration) *DummyProofGenerator {
	return &DummyProofGenerator{
		ProofId:        proofId,
		GenerationDelay: delay,
	}
}

// Generate simulates proof generation by sleeping and then creating a
// deterministic, dummy proof.
// This method fulfills the ProofGenerator interface.
func (d *DummyProofGenerator) Generate(
	slot primitives.Slot,
	payloadHash common.Hash,
	blockRoot common.Hash,
) (*executionproof.ExecutionProof, error) {
	// Simulate proof generation work
	if d.GenerationDelay > 0 {
		time.Sleep(d.GenerationDelay)
	}

	// Create a dummy proof with some deterministic data
	proofData := []byte{
		0xFF, // Magic byte for dummy proof
		d.ProofId.AsU8(),
		// Include some payload hash bytes
		payloadHash[0],
		payloadHash[1],
		payloadHash[2],
		payloadHash[3],
	}

	proof, err := executionproof.NewExecutionProof(d.ProofId, slot, payloadHash, blockRoot, proofData)
	if err != nil {
		return nil, fmt.Errorf("proof generation failed: %v", err)
	}
	return proof, nil
}

// SubnetId returns the subnet ID this generator produces proofs for.
// This method fulfills the ProofGenerator interface.
func (d *DummyProofGenerator) GetProofId() executionproof.ExecutionProofId {
	return d.ProofId
}