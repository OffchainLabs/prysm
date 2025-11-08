package zkvmexecutionlayer

import executionproof "github.com/OffchainLabs/prysm/v6/consensus-types/execution-proof"

// VerifierRegistry maps subnet IDs to proof verifiers.
//
// Each subnet can have a different zkVM/proof system, and this registry
// maintains the mapping from subnet ID to the appropriate verifier implementation.
type VerifierRegistry struct {
	verifiers map[executionproof.ExecutionProofId]ProofVerifier
}

// NewVerifierRegistry creates a new empty verifier registry.
func NewVerifierRegistry() *VerifierRegistry {
	return &VerifierRegistry{
		verifiers: make(map[executionproof.ExecutionProofId]ProofVerifier),
	}
}

// NewVerifierRegistryWithDummyVerifiers creates a registry with dummy verifiers
// for all available proof IDs. This is useful for testing.
func NewVerifierRegistryWithDummyVerifiers() *VerifierRegistry {
	verifiers := make(map[executionproof.ExecutionProofId]ProofVerifier)

	// All() is defined in execution_proof_subnet_id.go
	allProofIds := executionproof.All()
	for _, proofId := range allProofIds {
		verifiers[proofId] = NewDummyVerifier(proofId)
	}
	return &VerifierRegistry{verifiers: verifiers}
}

// RegisterVerifier adds or replaces a verifier in the registry.
func (r *VerifierRegistry) RegisterVerifier(verifier ProofVerifier) {
	proofId := verifier.GetProofId()
	r.verifiers[proofId] = verifier
}

// GetVerifier retrieves a verifier by its subnet ID.
// The boolean return value indicates if the verifier was found.
func (r *VerifierRegistry) GetVerifier(proofId executionproof.ExecutionProofId) (ProofVerifier, bool) {
	gen, ok := r.verifiers[proofId]
	return gen, ok
}

// HasVerifier checks if a verifier is registered for a subnet.
func (r *VerifierRegistry) HasVerifier(proofId executionproof.ExecutionProofId) bool {
	_, ok := r.verifiers[proofId]
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

// ProofIds returns a slice of all registered subnet IDs.
func (r *VerifierRegistry) ProofIds() []executionproof.ExecutionProofId {
	ids := make([]executionproof.ExecutionProofId, 0, len(r.verifiers))
	for id := range r.verifiers {
		ids = append(ids, id)
	}
	return ids
}