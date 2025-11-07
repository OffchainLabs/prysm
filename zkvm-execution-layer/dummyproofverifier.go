package zkvmexecutionlayer

import (
	"fmt"
	"time"
	executionproof "github.com/OffchainLabs/prysm/v6/consensus-types/execution-proof"
	"github.com/ethereum/go-ethereum/common"
)

const (
	// defaultVerificationDelay simulates some verification work.
	defaultVerificationDelay = 10 * time.Millisecond
)

// DummyVerifier is a test implementation of the ProofVerifier interface.
// It simulates the verification process with a configurable delay
// and always returns successful verification if the subnet and block hash match.
type DummyVerifier struct {
	subnetId          executionproof.ExecutionProofSubnetId
	verificationDelay time.Duration
}

// NewDummyVerifier creates a new dummy verifier for the specified subnet
// with a default delay.
func NewDummyVerifier(subnetId executionproof.ExecutionProofSubnetId) *DummyVerifier {
	return &DummyVerifier{
		subnetId:          subnetId,
		verificationDelay: defaultVerificationDelay,
	}
}

// NewDummyVerifierWithDelay creates a new dummy verifier with a custom
// verification delay.
func NewDummyVerifierWithDelay(subnetId executionproof.ExecutionProofSubnetId, delay time.Duration) *DummyVerifier {
	return &DummyVerifier{
		subnetId:          subnetId,
		verificationDelay: delay,
	}
}

// Verifier checks if a proof is valid.
// It simulates verification by sleeping and then returns true if
// the subnet ID and payload hash match.
// This method fulfills the ProofVerifier interface.
func (d *DummyVerifier) Verifier(
	payloadHash common.Hash,
	proof executionproof.ExecutionProof,
) (bool, error) {
	// Check that the proof is for the correct subnet
	if proof.SubnetId != d.subnetId {
		return false, fmt.Errorf("Unsuported Subnet: %v", proof.SubnetId)
	}

	// Check that the proof is for the correct payload
	if proof.BlockHash != payloadHash {
		return false, fmt.Errorf("proof block hash mismatch: expected %s, got %s",
		payloadHash.String(), proof.BlockHash.String())
	}

	// Simulate verification work
	if d.verificationDelay > 0 {
		time.Sleep(d.verificationDelay)
	}

	// Dummy verifier always succeeds if checks pass
	return true, nil
}

// SubnetId returns the subnet ID this verifier can handle.
// This method fulfills the ProofVerifier interface.
func (d *DummyVerifier) SubnetId() executionproof.ExecutionProofSubnetId {
	return d.subnetId
}