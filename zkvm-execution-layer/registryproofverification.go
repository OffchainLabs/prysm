package zkvmexecutionlayer

import executionproof "github.com/OffchainLabs/prysm/v6/consensus-types/execution-proof"

// VerifierRegistry maps subnet IDs to proof verifiers.
//
// Each subnet can have a different zkVM/proof system, and this registry
// maintains the mapping from subnet ID to the appropriate verifier implementation.
type VerifierRegistry struct {
	verifiers map[executionproof.ExecutionProofSubnetId]ProofVerifier
}

// NewVerifierRegistry creates a new empty verifier registry.
func NewVerifierRegistry() *VerifierRegistry {
	return &VerifierRegistry{
		verifiers: make(map[executionproof.ExecutionProofSubnetId]ProofVerifier),
	}
}

// NewVerifierRegistryWithDummyVerifiers creates a registry with dummy verifiers
// for all available subnets. This is useful for testing.
func NewVerifierRegistryWithDummyVerifiers() *VerifierRegistry {
	verifiers := make(map[executionproof.ExecutionProofSubnetId]ProofVerifier)

	// All() is defined in execution_proof_subnet_id.go
	allSubnets := executionproof.All()
	for _, subnetId := range allSubnets {
		verifiers[subnetId] = NewDummyVerifier(subnetId)
	}
	return &VerifierRegistry{verifiers: verifiers}
}

// RegisterVerifier adds or replaces a verifier in the registry.
func (r *VerifierRegistry) RegisterVerifier(verifier ProofVerifier) {
	subnetId := verifier.SubnetId()
	r.verifiers[subnetId] = verifier
}

// GetVerifier retrieves a verifier by its subnet ID.
// The boolean return value indicates if the verifier was found.
func (r *VerifierRegistry) GetVerifier(subnetId executionproof.ExecutionProofSubnetId) (ProofVerifier, bool) {
	gen, ok := r.verifiers[subnetId]
	return gen, ok
}

// HasVerifier checks if a verifier is registered for a subnet.
func (r *VerifierRegistry) HasVerifier(subnetId executionproof.ExecutionProofSubnetId) bool {
	_, ok := r.verifiers[subnetId]
	return ok
}

// Len gets the number of registered verifiers.
func (r *VerifierRegistry) Len() int {
	return len(r.verifiers)
}

// IsEmpty checks if the registry is empty.
func (r *VerifierRegistry) IsEmpty() bool {
	return len(r.verifiers) == 0
}

// SubnetIds returns a slice of all registered subnet IDs.
func (r *VerifierRegistry) SubnetIds() []executionproof.ExecutionProofSubnetId {
	ids := make([]executionproof.ExecutionProofSubnetId, 0, len(r.verifiers))
	for id := range r.verifiers {
		ids = append(ids, id)
	}
	return ids
}