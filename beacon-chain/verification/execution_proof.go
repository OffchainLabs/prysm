package verification

import (
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/pkg/errors"
)

// GossipSignedExecutionProofRequirements defines the set of requirements that SignedExecutionProofs received on gossip
// must satisfy in order to upgrade an ROSignedExecutionProof to a VerifiedROSignedExecutionProof.
var GossipSignedExecutionProofRequirements = []Requirement{
	RequireActiveValidator,
	RequireValidProverSignature,
	RequireProofDataNonEmpty,
	RequireProofDataNotTooLarge,
	RequireProofVerified,
}

// ROSignedExecutionProofsVerifier verifies execution proofs.
type ROSignedExecutionProofsVerifier struct {
	*sharedResources
	results *results
	proofs  []blocks.ROSignedExecutionProof
}

var _ SignedExecutionProofsVerifier = &ROSignedExecutionProofsVerifier{}

// VerifiedROSignedExecutionProofs "upgrades" wrapped ROSignedExecutionProofs to VerifiedROSignedExecutionProofs.
// If any of the verifications ran against the proofs failed, or some required verifications
// were not run, an error will be returned.
func (v *ROSignedExecutionProofsVerifier) VerifiedROSignedExecutionProofs() ([]blocks.VerifiedROSignedExecutionProof, error) {
	if !v.results.allSatisfied() {
		return nil, v.results.errors(errProofsInvalid)
	}

	verifiedSignedProofs := make([]blocks.VerifiedROSignedExecutionProof, 0, len(v.proofs))
	for _, proof := range v.proofs {
		verifiedProof := blocks.NewVerifiedROSignedExecutionProof(proof)
		verifiedSignedProofs = append(verifiedSignedProofs, verifiedProof)
	}

	return verifiedSignedProofs, nil
}

// SatisfyRequirement allows the caller to assert that a requirement has been satisfied.
func (v *ROSignedExecutionProofsVerifier) SatisfyRequirement(req Requirement) {
	v.recordResult(req, nil)
}

func (v *ROSignedExecutionProofsVerifier) recordResult(req Requirement, err *error) {
	if err == nil || *err == nil {
		v.results.record(req, nil)
		return
	}
	v.results.record(req, *err)
}

func (v *ROSignedExecutionProofsVerifier) IsFromActiveValidator() (err error) {
	if ok, err := v.results.cached(RequireActiveValidator); ok {
		return err
	}

	defer v.recordResult(RequireActiveValidator, &err)

	// TODO: To implement
	return nil
}

func (v *ROSignedExecutionProofsVerifier) ValidProverSignature() (err error) {
	if ok, err := v.results.cached(RequireValidProverSignature); ok {
		return err
	}

	defer v.recordResult(RequireValidProverSignature, &err)

	// TODO: To implement
	return nil
}

func (v *ROSignedExecutionProofsVerifier) ProofDataNonEmpty() (err error) {
	if ok, err := v.results.cached(RequireProofDataNonEmpty); ok {
		return err
	}

	defer v.recordResult(RequireProofDataNonEmpty, &err)

	for _, proof := range v.proofs {
		if len(proof.Message.ProofData) == 0 {
			return proofErrBuilder(ErrProofDataEmpty)
		}
	}

	return nil
}

func (v *ROSignedExecutionProofsVerifier) ProofDataNotTooLarge() (err error) {
	if ok, err := v.results.cached(RequireProofDataNotTooLarge); ok {
		return err
	}

	defer v.recordResult(RequireProofDataNotTooLarge, &err)

	maxProofDataBytes := params.BeaconConfig().MaxProofDataBytes

	for _, proof := range v.proofs {
		if uint64(len(proof.Message.ProofData)) > maxProofDataBytes {
			return proofErrBuilder(ErrProofSizeTooLarge)
		}
	}

	return nil
}

// ProofVerified performs zkVM proof verification.
// Currently a no-op, will be implemented when actual proof verification is added.
func (v *ROSignedExecutionProofsVerifier) ProofVerified() (err error) {
	if ok, err := v.results.cached(RequireProofVerified); ok {
		return err
	}

	defer v.recordResult(RequireProofVerified, &err)

	// TODO: To implement
	return nil
}

func proofErrBuilder(baseErr error) error {
	return errors.Wrap(baseErr, errProofsInvalid.Error())
}
