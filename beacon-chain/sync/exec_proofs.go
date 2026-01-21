package sync

import (
	"fmt"
	"time"

	"errors"

	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// generateExecProof returns a dummy execution proof after the specified delay.
func generateExecProof(roBlock blocks.ROBlock, proofID primitives.ExecutionProofId, delay time.Duration) (*ethpb.ExecutionProof, error) {
	// Simulate proof generation work
	time.Sleep(delay)

	// Create a dummy proof with some deterministic data
	block := roBlock.Block()
	if block == nil {
		return nil, errors.New("nil block")
	}

	body := block.Body()
	if body == nil {
		return nil, errors.New("nil block body")
	}

	executionData, err := body.Execution()
	if err != nil {
		return nil, fmt.Errorf("execution: %w", err)
	}

	if executionData == nil {
		return nil, errors.New("nil execution data")
	}

	hash, err := executionData.HashTreeRoot()
	if err != nil {
		return nil, fmt.Errorf("hash tree root: %w", err)
	}

	proofData := []byte{
		0xFF, // Magic byte for dummy proof
		byte(proofID),
		// Include some payload hash bytes
		hash[0],
		hash[1],
		hash[2],
		hash[3],
	}

	blockRoot := roBlock.Root()

	proof := &ethpb.ExecutionProof{
		ProofId:   proofID,
		Slot:      block.Slot(),
		BlockHash: hash[:],
		BlockRoot: blockRoot[:],
		ProofData: proofData,
	}

	return proof, nil
}
