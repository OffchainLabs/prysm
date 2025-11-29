package zkvmexecutionlayer

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
)

// createTestProof helper for verifier tests.
func createTestProofForVerify(subnetId primitives.ExecutionProofId, blockHash []byte) *ethpb.ExecutionProof {
	return &ethpb.ExecutionProof{
		ProofId:   subnetId,
		Slot:      primitives.Slot(0),
		BlockHash: blockHash,
		BlockRoot: newBlockHashOrRoot(1),
		ProofData: []byte{1, 2, 3},
	}
}

// TestDummyVerifierSuccess translates test_dummy_verifier_success.
func TestDummyVerifierSuccess(t *testing.T) {
	subnet := primitives.ExecutionProofId(0)
	verifier := NewDummyVerifier(subnet)
	blockHash := newBlockHashOrRoot(1)
	proof := createTestProofForVerify(subnet, blockHash)

	valid, err := verifier.Verify(proof)
	assert.NoError(t, err)
	assert.Equal(t, valid, true)
}

// TestDummyVerifierMismatchedSubnet translates test_dummy_verifier_mismatched_subnet.
func TestDummyVerifierMismatchedSubnet(t *testing.T) {
	subnet0 := primitives.ExecutionProofId(0)
	subnet1 := primitives.ExecutionProofId(1)
	verifier := NewDummyVerifier(subnet0) // Verifier for subnet 0
	blockHash := newBlockHashOrRoot(1)
	proof := createTestProofForVerify(subnet1, blockHash) // Proof from subnet 1

	_, err := verifier.Verify(proof)
	if err == nil {
		t.Fatal("Expected verification to fail, but got no error")
	}

	assert.Equal(t, proof.ProofId, subnet1, "ProofId mismatch")
}
