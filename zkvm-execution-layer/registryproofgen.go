package zkvmexecutionlayer

import executionproof "github.com/OffchainLabs/prysm/v6/consensus-types/execution-proof"

// GeneratorRegistry maps subnet IDs to proof generators.
//
// Each subnet can have a different zkVM/proof system, and this registry
// maintains the mapping from subnet ID to the appropriate generator implementation.
// Not all subnets need generators - nodes can verify without generating.
type GeneratorRegistry struct {
	generators map[executionproof.ExecutionProofSubnetId]ProofGenerator
}

// NewGeneratorRegistry creates a new empty generator registry.
// This is equivalent to `default()` or `new()`.
func NewGeneratorRegistry() *GeneratorRegistry {
	return &GeneratorRegistry{
		generators: make(map[executionproof.ExecutionProofSubnetId]ProofGenerator),
	}
}

// NewGeneratorRegistryWithDummyGenerators creates a registry with dummy generators
// for the specified subnets. This is useful for testing.
// The `enabledSubnets` map acts as a HashSet.
func NewGeneratorRegistryWithDummyGenerators(enabledSubnets map[executionproof.ExecutionProofSubnetId]struct{}) *GeneratorRegistry {
	generators := make(map[executionproof.ExecutionProofSubnetId]ProofGenerator, len(enabledSubnets))
	for subnetId := range enabledSubnets {
		// NewDummyProofGenerator is defined in dummy_proof_generator.go
		generators[subnetId] = NewDummyProofGenerator(subnetId)
	}
	return &GeneratorRegistry{generators: generators}
}

// RegisterGenerator adds or replaces a generator in the registry.
func (r *GeneratorRegistry) RegisterGenerator(generator ProofGenerator) {
	subnetId := generator.SubnetId()
	r.generators[subnetId] = generator
}

// GetGenerator retrieves a generator by its subnet ID.
// The boolean return value indicates if the generator was found.
func (r *GeneratorRegistry) GetGenerator(subnetId executionproof.ExecutionProofSubnetId) (ProofGenerator, bool) {
	gen, ok := r.generators[subnetId]
	return gen, ok
}

// HasGenerator checks if a generator is registered for a subnet.
func (r *GeneratorRegistry) HasGenerator(subnetId executionproof.ExecutionProofSubnetId) bool {
	_, ok := r.generators[subnetId]
	return ok
}

// Len gets the number of registered generators.
func (r *GeneratorRegistry) Len() int {
	return len(r.generators)
}

// IsEmpty checks if the registry is empty.
func (r *GeneratorRegistry) IsEmpty() bool {
	return len(r.generators) == 0
}

// SubnetIds returns a slice of all registered subnet IDs.
func (r *GeneratorRegistry) SubnetIds() []executionproof.ExecutionProofSubnetId {
	ids := make([]executionproof.ExecutionProofSubnetId, 0, len(r.generators))
	for id := range r.generators {
		ids = append(ids, id)
	}
	return ids
}