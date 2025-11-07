package zkvmexecutionlayer

import (
	"testing"
	executionproof "github.com/OffchainLabs/prysm/v6/consensus-types/execution-proof"
	"github.com/ethereum/go-ethereum/common"
)

// createTestProof helper for verifier tests.
func createTestProofForVerify(subnetId executionproof.ExecutionProofSubnetId, blockHash common.Hash) *executionproof.ExecutionProof {
	// We can ignore the error here as we're in a test.
	proof, _ := executionproof.NewExecutionProof(subnetId, blockHash, newBlockHashOrRoot(1), []byte{1, 2, 3})
	return proof
}


// TestDummyVerifierSuccess translates test_dummy_verifier_success.
func TestDummyVerifierSuccess(t *testing.T) {
	subnet, _ := executionproof.NewExecutionProofSubnetId(0)
	verifier := NewDummyVerifier(subnet)
	blockHash := newBlockHashOrRoot(1)
	proof := createTestProofForVerify(subnet, blockHash)

	valid, err := verifier.Verifier(blockHash, *proof)
	if err != nil {
		t.Fatalf("Expected verification to succeed, but got error: %v", err)
	}
	if !valid {
		t.Fatal("Expected verification to return true, but got false")
	}
}

// TestDummyVerifierMismatchedSubnet translates test_dummy_verifier_mismatched_subnet.
func TestDummyVerifierMismatchedSubnet(t *testing.T) {
	subnet0, _ := executionproof.NewExecutionProofSubnetId(0)
	subnet1, _ := executionproof.NewExecutionProofSubnetId(1)
	verifier := NewDummyVerifier(subnet0) // Verifier for subnet 0
	blockHash := newBlockHashOrRoot(1)
	proof := createTestProofForVerify(subnet1, blockHash) // Proof from subnet 1

	_, err := verifier.Verifier(blockHash, *proof)
	if err == nil {
		t.Fatal("Expected verification to fail, but got no error")
	}


	if proof.SubnetId != subnet1 {
		t.Errorf("Expected error for subnet %s, but got %s", subnet1, proof.SubnetId)
	}
}