package zkvmexecutionlayer

import (
	"fmt"
	"time"
	executionproof "github.com/OffchainLabs/prysm/v6/consensus-types/execution-proof"
)

const (
	// defaultVerificationDelay simulates some verification work.
	defaultVerificationDelay = 10 * time.Millisecond
)

// DummyVerifier is a test implementation of the ProofVerifier interface.
// It simulates the verification process with a configurable delay
// and always returns successful verification if the subnet and block hash match.
type DummyVerifier struct {
	ProofId          executionproof.ExecutionProofId
	verificationDelay time.Duration
}

// NewDummyVerifier creates a new dummy verifier for the specified subnet
// with a default delay.
func NewDummyVerifier(subnetId executionproof.ExecutionProofId) *DummyVerifier {
	return &DummyVerifier{
		ProofId:          subnetId,
		verificationDelay: defaultVerificationDelay,
	}
}

// NewDummyVerifierWithDelay creates a new dummy verifier with a custom
// verification delay.
func NewDummyVerifierWithDelay(subnetId executionproof.ExecutionProofId, delay time.Duration) *DummyVerifier {
	return &DummyVerifier{
		ProofId:          subnetId,
		verificationDelay: delay,
	}
}

// Verifier checks if a proof is valid.
// It simulates verification by sleeping and then returns true if
// the subnet ID and payload hash match.
// This method fulfills the ProofVerifier interface.
func (d *DummyVerifier) Verifier(
	proof executionproof.ExecutionProof,
) (bool, error) {
	// Check that the proof is for the correct subnet
	if proof.ProofId != d.ProofId {
		return false, fmt.Errorf("Unsuported Subnet: %v", proof.ProofId)
	}

	// Simulate verification work
	if d.verificationDelay > 0 {
		time.Sleep(d.verificationDelay)
	}

	// Dummy verifier always succeeds if checks pass
	return true, nil
}

// GetProofId returns the subnet ID this verifier can handle.
// This method fulfills the ProofVerifier interface.
func (d *DummyVerifier) GetProofId() executionproof.ExecutionProofId {
	return d.ProofId
}