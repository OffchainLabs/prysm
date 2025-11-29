package zkvmexecutionlayer

import (
	"time"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

const (
	// defaultGenerationDelay simulates some proof generation work.
	defaultGenerationDelay = 50 * time.Millisecond
)

// DummyProofGenerator is a test implementation of the ProofGenerator interface.
// It simulates the proof generation process with a configurable delay
// and creates dummy proofs.
type DummyProofGenerator struct {
	ProofId         primitives.ExecutionProofId
	GenerationDelay time.Duration
}

// NewDummyProofGenerator creates a new dummy generator for the specified subnet
// with a default delay.
func NewDummyProofGenerator(proofId primitives.ExecutionProofId) *DummyProofGenerator {
	return &DummyProofGenerator{
		ProofId:         proofId,
		GenerationDelay: defaultGenerationDelay,
	}
}

// NewDummyProofGeneratorWithDelay creates a new dummy generator with a custom
// generation delay.
func NewDummyProofGeneratorWithDelay(proofId primitives.ExecutionProofId, delay time.Duration) *DummyProofGenerator {
	return &DummyProofGenerator{
		ProofId:         proofId,
		GenerationDelay: delay,
	}
}

// Generate simulates proof generation by sleeping and then creating a
// deterministic, dummy proof.
// This method fulfills the ProofGenerator interface.
func (d *DummyProofGenerator) Generate(
	slot primitives.Slot,
	payloadHash []byte,
	blockRoot []byte,
) (*ethpb.ExecutionProof, error) {
	// Simulate proof generation work
	if d.GenerationDelay > 0 {
		time.Sleep(d.GenerationDelay)
	}

	// Create a dummy proof with some deterministic data
	proofData := []byte{
		0xFF, // Magic byte for dummy proof
		byte(d.ProofId),
		// Include some payload hash bytes
		payloadHash[0],
		payloadHash[1],
		payloadHash[2],
		payloadHash[3],
	}

	return &ethpb.ExecutionProof{
		ProofId:   d.ProofId,
		Slot:      slot,
		BlockHash: payloadHash,
		BlockRoot: blockRoot,
		ProofData: proofData,
	}, nil
}

// SubnetId returns the subnet ID this generator produces proofs for.
// This method fulfills the ProofGenerator interface.
func (d *DummyProofGenerator) GetProofId() primitives.ExecutionProofId {
	return d.ProofId
}
