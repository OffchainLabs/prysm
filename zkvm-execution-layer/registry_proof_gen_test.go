package zkvmexecutionlayer

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
)

// TestDummyGeneratorsRegistry translates test_dummy_generators_registry.
func TestDummyGeneratorsRegistry(t *testing.T) {
	subnet0 := primitives.ExecutionProofId(0)
	subnet1 := primitives.ExecutionProofId(1)
	subnet2 := primitives.ExecutionProofId(2)

	enabledSubnets := map[primitives.ExecutionProofId]struct{}{
		subnet0: {},
		subnet1: {},
	}

	registry := NewGeneratorRegistryWithDummyGenerators(enabledSubnets)

	assert.Equal(t, registry.Len(), 2, "Registry length mismatch")
	assert.Equal(t, registry.HasGenerator(subnet0), true)
	assert.Equal(t, registry.HasGenerator(subnet1), true)
	assert.Equal(t, registry.HasGenerator(subnet2), false)
}

// TestRegisterGenerator translates test_register_generator.
func TestRegisterGenerator(t *testing.T) {
	registry := NewGeneratorRegistry()
	subnetId := primitives.ExecutionProofId(0)
	generator := NewDummyProofGenerator(subnetId)

	registry.RegisterGenerator(generator)

	assert.Equal(t, registry.Len(), 1, "Registry length mismatch")
	assert.Equal(t, registry.HasGenerator(subnetId), true)
}

// TestGetGenerator translates test_get_generator.
func TestGetGenerator(t *testing.T) {
	proofId := primitives.ExecutionProofId(3)
	enabledSubnets := map[primitives.ExecutionProofId]struct{}{
		proofId: {},
	}

	registry := NewGeneratorRegistryWithDummyGenerators(enabledSubnets)

	generator, ok := registry.GetGenerator(proofId)
	assert.Equal(t, true, ok, "Expected to find generator for subnet %s", proofId)
	assert.Equal(t, proofId, generator.GetProofId(), "Generator ProofId mismatch")
}

// TestSubnetIds translates test_subnet_ids.
func TestSubnetIds(t *testing.T) {
	subnet0 := primitives.ExecutionProofId(0)
	subnet5 := primitives.ExecutionProofId(5)

	enabledSubnets := map[primitives.ExecutionProofId]struct{}{
		subnet0: {},
		subnet5: {},
	}

	registry := NewGeneratorRegistryWithDummyGenerators(enabledSubnets)
	subnetIds := registry.SubnetIds()

	assert.Equal(t, len(subnetIds), 2, "SubnetIds length mismatch")

	// Check that all returned IDs are in the original map
	foundMap := make(map[primitives.ExecutionProofId]bool)
	for _, id := range subnetIds {
		foundMap[id] = true
	}

	for id := range enabledSubnets {
		assert.Equal(t, true, foundMap[id], "Missing subnet ID %s from SubnetIds() result", id)
	}
}
