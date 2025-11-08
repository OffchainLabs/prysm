package executionproof

import (
	"bytes"
	"strings"
	"testing"

	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/ethereum/go-ethereum/common"
)

// TestExecutionProofTooLarge translates test_execution_proof_too_large.
func TestExecutionProofTooLarge(t *testing.T) {
	subnetId, _ := NewExecutionProofId(0)
	// Use common.Hash{} for the zero value
	blockHash := common.Hash{}
	blockRoot := common.Hash{}
	proofData := make([]byte, MAX_PROOF_DATA_BYTES+1)
	slot := primitives.Slot(0)

	_, err := NewExecutionProof(subnetId, slot, blockHash, blockRoot, proofData)

	if err == nil {
		t.Fatal("Expected an error for proof data being too large, but got nil")
	}

	if !strings.Contains(err.Error(), "Proof data too large") {
		t.Errorf("Expected error message to contain 'Proof data too large', but got: %s", err.Error())
	}
}

// TestExecutionProofMaxSize translates test_execution_proof_max_size.
func TestExecutionProofMaxSize(t *testing.T) {
	subnetId, _ := NewExecutionProofId(0)
	// Use common.Hash{} for the zero value
	blockHash := common.Hash{}
	blockRoot := common.Hash{}
	proofData := make([]byte, MAX_PROOF_DATA_BYTES)
	slot := primitives.Slot(0)

	proof, err := NewExecutionProof(subnetId, slot, blockHash, blockRoot, proofData)

	if err != nil {
		t.Fatalf("Expected no error for proof data at max size, but got: %v", err)
	}

	if proof == nil {
		t.Fatal("Expected proof to be non-nil, but got nil")
	}

	if proof.ProofDataSize() != MAX_PROOF_DATA_BYTES {
		t.Errorf("Expected proof data size to be %d, but got %d", MAX_PROOF_DATA_BYTES, proof.ProofDataSize())
	}
}

// TestExecutionProofMethods checks the helper methods.
func TestExecutionProofMethods(t *testing.T) {
	subnetId0, _ := NewExecutionProofId(0)
	subnetId1, _ := NewExecutionProofId(1)
	// Use common.Hash{} for the zero value
	blockHash0 := common.Hash{}
	var blockHash1 common.Hash
	blockHash1[0] = 0x01 // Set a non-zero value
	slot := primitives.Slot(0)

	proof, err := NewExecutionProof(subnetId0, slot, blockHash0, common.Hash{}, []byte{1, 2, 3})
	if err != nil {
		t.Fatalf("Failed to create proof: %v", err)
	}

	// Test ProofDataSize
	if proof.ProofDataSize() != 3 {
		t.Errorf("Expected ProofDataSize 3, got %d", proof.ProofDataSize())
	}

	// Test ProofDataSlice
	if !bytes.Equal(proof.ProofDataSlice(), []byte{1, 2, 3}) {
		t.Errorf("Expected ProofDataSlice [1, 2, 3], got %v", proof.ProofDataSlice())
	}

	// Test IsForBlock (note: passing pointer to hash)
	if !proof.IsForBlock(&blockHash0) {
		t.Error("IsForBlock returned false for correct hash")
	}
	if proof.IsForBlock(&blockHash1) {
		t.Error("IsForBlock returned true for incorrect hash")
	}

	// Test IsFromSubnet
	if !proof.IsFromProofType(subnetId0) {
		t.Error("IsFromSubnet returned false for correct subnet")
	}
	if proof.IsFromProofType(subnetId1) {
		t.Error("IsFromSubnet returned true for incorrect subnet")
	}
}