package verification

import (
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/pkg/errors"
)

// GossipExecutionProofRequirements defines the set of requirements that ExecutionProofs received on gossip
// must satisfy in order to upgrade an ROExecutionProof to a VerifiedROExecutionProof.
var GossipExecutionProofRequirements = []Requirement{
	RequireNotFromFutureSlot,
	RequireProofSizeLimits,
	RequireProofVerified,
}

// ROExecutionProofsVerifier verifies execution proofs.
type ROExecutionProofsVerifier struct {
	*sharedResources
	results *results
	proofs  []blocks.ROExecutionProof
}

var _ ExecutionProofsVerifier = &ROExecutionProofsVerifier{}

// VerifiedROExecutionProofs "upgrades" wrapped ROExecutionProofs to VerifiedROExecutionProofs.
// If any of the verifications ran against the proofs failed, or some required verifications
// were not run, an error will be returned.
func (v *ROExecutionProofsVerifier) VerifiedROExecutionProofs() ([]blocks.VerifiedROExecutionProof, error) {
	if !v.results.allSatisfied() {
		return nil, v.results.errors(errProofsInvalid)
	}

	verifiedProofs := make([]blocks.VerifiedROExecutionProof, 0, len(v.proofs))
	for _, proof := range v.proofs {
		verifiedProof := blocks.NewVerifiedROExecutionProof(proof)
		verifiedProofs = append(verifiedProofs, verifiedProof)
	}

	return verifiedProofs, nil
}

// SatisfyRequirement allows the caller to assert that a requirement has been satisfied.
func (v *ROExecutionProofsVerifier) SatisfyRequirement(req Requirement) {
	v.recordResult(req, nil)
}

func (v *ROExecutionProofsVerifier) recordResult(req Requirement, err *error) {
	if err == nil || *err == nil {
		v.results.record(req, nil)
		return
	}
	v.results.record(req, *err)
}

// NotFromFutureSlot verifies that the execution proof is not from a future slot.
func (v *ROExecutionProofsVerifier) NotFromFutureSlot() (err error) {
	if ok, err := v.results.cached(RequireNotFromFutureSlot); ok {
		return err
	}

	defer v.recordResult(RequireNotFromFutureSlot, &err)

	currentSlot := v.clock.CurrentSlot()
	now := v.clock.Now()
	maximumGossipClockDisparity := params.BeaconConfig().MaximumGossipClockDisparityDuration()

	for _, proof := range v.proofs {
		proofSlot := proof.Slot()

		if currentSlot == proofSlot {
			continue
		}

		earliestStart, err := v.clock.SlotStart(proofSlot)
		if err != nil {
			return proofErrBuilder(errors.Wrap(err, "failed to determine slot start time from clock"))
		}
		earliestStart = earliestStart.Add(-maximumGossipClockDisparity)

		if now.Before(earliestStart) {
			return proofErrBuilder(errFromFutureSlot)
		}
	}

	return nil
}

// ProofSizeLimits verifies that the execution proof data does not exceed the maximum allowed size.
func (v *ROExecutionProofsVerifier) ProofSizeLimits() (err error) {
	if ok, err := v.results.cached(RequireProofSizeLimits); ok {
		return err
	}

	defer v.recordResult(RequireProofSizeLimits, &err)

	maxProofDataBytes := params.BeaconConfig().MaxProofDataBytes

	for _, proof := range v.proofs {
		if uint64(len(proof.ProofData)) > maxProofDataBytes {
			return proofErrBuilder(ErrProofSizeTooLarge)
		}
	}

	return nil
}

// ProofVerified performs zkVM proof verification.
// Currently a no-op, will be implemented when actual proof verification is added.
func (v *ROExecutionProofsVerifier) ProofVerified() (err error) {
	if ok, err := v.results.cached(RequireProofVerified); ok {
		return err
	}

	defer v.recordResult(RequireProofVerified, &err)

	// For now, all proofs are considered valid.
	// TODO: Implement actual zkVM proof verification.
	return nil
}

func proofErrBuilder(baseErr error) error {
	return errors.Wrap(baseErr, errProofsInvalid.Error())
}
