package zkvmexecutionlayer

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
)

// TestEmptyRegistry translates test_empty_registry.
func TestEmptyRegistry(t *testing.T) {
	registry := NewVerifierRegistry()

	assert.Equal(t, registry.IsEmpty(), true, "Registry should be empty")
	assert.Equal(t, registry.Len(), 0, "Registry length should be 0")
}

// TestDummyVerifiersRegistry translates test_dummy_verifiers_registry.
func TestDummyVerifiersRegistry(t *testing.T) {
	registry := NewVerifierRegistryWithDummyVerifiers()
	assert.Equal(t, registry.IsEmpty(), false, "Registry should not be empty")

	// Check all subnets are registered
	for id := range primitives.EXECUTION_PROOF_TYPE_COUNT {
		subnetId := primitives.ExecutionProofId(id)
		assert.Equal(t, registry.HasVerifier(subnetId), true, "Registry missing verifier for subnet %d", id)
		_, ok := registry.GetVerifier(subnetId)
		assert.Equal(t, ok, true, "Expected to get verifier for subnet %d", id)
	}
}

// TestRegisterVerifier translates test_register_verifier.
func TestRegisterVerifier(t *testing.T) {
	registry := NewVerifierRegistry()
	subnetId := primitives.ExecutionProofId(0)
	verifier := NewDummyVerifier(subnetId) // From dummy_proof_verifier.go

	registry.RegisterVerifier(verifier)

	assert.Equal(t, registry.Len(), 1, "Registry length mismatch")
	assert.Equal(t, registry.HasVerifier(subnetId), true)
}

// TestVerifierProofIds translates test_subnet_ids (for verifiers).
func TestVerifierProofIds(t *testing.T) {
	registry := NewVerifierRegistryWithDummyVerifiers()
	subnetIds := registry.ProofIds()

	assert.Equal(t, len(subnetIds), int(primitives.EXECUTION_PROOF_TYPE_COUNT), "SubnetIds length mismatch")

	// Check that all returned IDs are valid subnets
	foundMap := make(map[primitives.ExecutionProofId]bool)
	for _, id := range subnetIds {
		foundMap[id] = true
	}

	for id := range primitives.EXECUTION_PROOF_TYPE_COUNT {
		subnet := primitives.ExecutionProofId(id)
		assert.Equal(t, true, foundMap[subnet], "Missing subnet ID %s from ProofIds() result", subnet)
	}
}
