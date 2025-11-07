package zkvmexecutionlayer

import (
	"testing"
	executionproof "github.com/OffchainLabs/prysm/v6/consensus-types/execution-proof"
)

// TestDummyGeneratorsRegistry translates test_dummy_generators_registry.
func TestDummyGeneratorsRegistry(t *testing.T) {
	subnet0, _ := executionproof.NewExecutionProofSubnetId(0)
	subnet1, _ := executionproof.NewExecutionProofSubnetId(1)
	subnet2, _ := executionproof.NewExecutionProofSubnetId(2)

	enabledSubnets := map[executionproof.ExecutionProofSubnetId]struct{}{
		subnet0: {},
		subnet1: {},
	}

	registry := NewGeneratorRegistryWithDummyGenerators(enabledSubnets)
	if registry.IsEmpty() {
		t.Error("Expected registry to not be empty, but it was")
	}
	if registry.Len() != 2 {
		t.Errorf("Expected registry len 2, got %d", registry.Len())
	}

	if !registry.HasGenerator(subnet0) {
		t.Error("Expected registry to have generator for subnet 0")
	}
	if !registry.HasGenerator(subnet1) {
		t.Error("Expected registry to have generator for subnet 1")
	}
	if registry.HasGenerator(subnet2) {
		t.Error("Expected registry to not have generator for subnet 2")
	}
}

// TestRegisterGenerator translates test_register_generator.
func TestRegisterGenerator(t *testing.T) {
	registry := NewGeneratorRegistry()
	subnetId, _ := executionproof.NewExecutionProofSubnetId(0)
	generator := NewDummyProofGenerator(subnetId)

	registry.RegisterGenerator(generator)

	if registry.Len() != 1 {
		t.Errorf("Expected registry len 1, got %d", registry.Len())
	}
	if !registry.HasGenerator(subnetId) {
		t.Error("Expected registry to have generator for subnet 0")
	}
}

// TestGetGenerator translates test_get_generator.
func TestGetGenerator(t *testing.T) {
	subnetId, _ := executionproof.NewExecutionProofSubnetId(3)
	enabledSubnets := map[executionproof.ExecutionProofSubnetId]struct{}{
		subnetId: {},
	}

	registry := NewGeneratorRegistryWithDummyGenerators(enabledSubnets)

	generator, ok := registry.GetGenerator(subnetId)
	if !ok {
		t.Fatal("Expected to get generator, but 'ok' was false")
	}
	if generator.SubnetId() != subnetId {
		t.Errorf("Expected generator for subnet %s, but got one for %s", subnetId, generator.SubnetId())
	}
}

// TestSubnetIds translates test_subnet_ids.
func TestSubnetIds(t *testing.T) {
	subnet0, _ := executionproof.NewExecutionProofSubnetId(0)
	subnet5, _ := executionproof.NewExecutionProofSubnetId(5)

	enabledSubnets := map[executionproof.ExecutionProofSubnetId]struct{}{
		subnet0: {},
		subnet5: {},
	}

	registry := NewGeneratorRegistryWithDummyGenerators(enabledSubnets)
	subnetIds := registry.SubnetIds()

	if len(subnetIds) != 2 {
		t.Fatalf("Expected 2 subnet IDs, got %d", len(subnetIds))
	}

	// Check that all returned IDs are in the original map
	foundMap := make(map[executionproof.ExecutionProofSubnetId]bool)
	for _, id := range subnetIds {
		foundMap[id] = true
	}

	for id := range enabledSubnets {
		if !foundMap[id] {
			t.Errorf("Missing subnet ID %s from SubnetIds() result", id)
		}
	}
}