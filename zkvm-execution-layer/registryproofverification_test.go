package zkvmexecutionlayer

import (
	"testing"
	executionproof "github.com/OffchainLabs/prysm/v6/consensus-types/execution-proof"
)

// TestEmptyRegistry translates test_empty_registry.
func TestEmptyRegistry(t *testing.T) {
	registry := NewVerifierRegistry()
	if !registry.IsEmpty() {
		t.Fatal("Expected registry to be empty, but it was not")
	}
	if registry.Len() != 0 {
		t.Errorf("Expected registry len 0, got %d", registry.Len())
	}
}

// TestDummyVerifiersRegistry translates test_dummy_verifiers_registry.
func TestDummyVerifiersRegistry(t *testing.T) {
	registry := NewVerifierRegistryWithDummyVerifiers()
	if registry.IsEmpty() {
		t.Fatal("Expected registry to not be empty, but it was")
	}
	if registry.Len() != int(executionproof.EXECUTION_PROOF_TYPE_COUNT) {
		t.Errorf("Expected registry len %d, got %d", executionproof.EXECUTION_PROOF_TYPE_COUNT, registry.Len())
	}

	// Check all subnets are registered
	for id := range executionproof.EXECUTION_PROOF_TYPE_COUNT {
		subnetId, _ := executionproof.NewExecutionProofId(id)
		if !registry.HasVerifier(subnetId) {
			t.Errorf("Expected registry to have verifier for subnet %d", id)
		}
		if _, ok := registry.GetVerifier(subnetId); !ok {
			t.Errorf("Expected to get verifier for subnet %d", id)
		}
	}
}

// TestRegisterVerifier translates test_register_verifier.
func TestRegisterVerifier(t *testing.T) {
	registry := NewVerifierRegistry()
	subnetId, _ := executionproof.NewExecutionProofId(0)
	verifier := NewDummyVerifier(subnetId) // From dummy_proof_verifier.go

	registry.RegisterVerifier(verifier)

	if registry.Len() != 1 {
		t.Errorf("Expected registry len 1, got %d", registry.Len())
	}
	if !registry.HasVerifier(subnetId) {
		t.Error("Expected registry to have verifier for subnet 0")
	}
}

// TestVerifierProofIds translates test_subnet_ids (for verifiers).
func TestVerifierProofIds(t *testing.T) {
	registry := NewVerifierRegistryWithDummyVerifiers()
	subnetIds := registry.ProofIds()

	if len(subnetIds) != int(executionproof.EXECUTION_PROOF_TYPE_COUNT) {
		t.Fatalf("Expected %d subnet IDs, got %d", executionproof.EXECUTION_PROOF_TYPE_COUNT, len(subnetIds))
	}

	// Check that all returned IDs are valid subnets
	foundMap := make(map[executionproof.ExecutionProofId]bool)
	for _, id := range subnetIds {
		foundMap[id] = true
	}

	for id := range executionproof.EXECUTION_PROOF_TYPE_COUNT {
		subnet, _ := executionproof.NewExecutionProofId(id)
		if !foundMap[subnet] {
			t.Errorf("Missing subnet ID %s from ProofIds() result", subnet)
		}
	}
}