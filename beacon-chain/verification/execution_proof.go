package verification

import (
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
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

func proofToSignatureData(proof blocks.ROSignedExecutionProof) (executionProofSignatureData, error) {
	proofRoot, err := blocks.ExecutionProofHashTreeRoot(proof.Message)
	if err != nil {
		return executionProofSignatureData{}, fmt.Errorf("hash tree root: %w", err)
	}

	return executionProofSignatureData{
		ProofRoot:      proofRoot,
		Signature:      bytesutil.ToBytes96(proof.Signature),
		ValidatorIndex: proof.GetValidatorIndex(),
		Epoch:          proof.Epoch(),
	}, nil
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

func (v *ROSignedExecutionProofsVerifier) ValidProverSignature(ctx context.Context) (err error) {
	if ok, err := v.results.cached(RequireValidProverSignature); ok {
		return err
	}

	defer v.recordResult(RequireValidProverSignature, &err)

	// Get the head state once to access validator public keys.
	headState, err := v.hsp.HeadStateReadOnly(ctx)
	if err != nil {
		return fmt.Errorf("%w: could not get head state: %w", ErrInvalidProverSignature, err)
	}

	for _, proof := range v.proofs {
		// Extract signature data from the proof.
		signatureData, err := proofToSignatureData(proof)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidProverSignature, err)
		}

		// First check if there is a cached verification that can be reused.
		seen, err := v.sc.ExecutionProofSignatureVerified(signatureData)
		if err != nil {
			executionProofSignatureCache.WithLabelValues("hit-invalid").Inc()
			return fmt.Errorf("%w: %w", ErrInvalidProverSignature, err)
		}

		// If yes, we can skip the full verification.
		if seen {
			executionProofSignatureCache.WithLabelValues("hit-valid").Inc()
			continue
		}

		// Ensure the expensive signature verification is only performed once for
		// concurrent requests for the same signature data.
		if _, err, _ = v.sg.Do(signatureData.concat(), func() (any, error) {
			executionProofSignatureCache.WithLabelValues("miss").Inc()

			// Full verification, which will subsequently be cached.
			if err = v.sc.VerifyExecutionProofSignature(signatureData, headState); err != nil {
				return nil, fmt.Errorf("verify signature: %w", err)
			}

			return nil, nil
		}); err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidProverSignature, err)
		}
	}

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

// ProofVerified verifies each proof by calling the verifier's verification endpoint.
// If no verifier is configured, verification is skipped.
func (v *ROSignedExecutionProofsVerifier) ProofVerified() (err error) {
	if ok, err := v.results.cached(RequireProofVerified); ok {
		return err
	}

	defer v.recordResult(RequireProofVerified, &err)

	if v.zpv == nil {
		return proofErrBuilder(ErrProofVerificationEndpoint)
	}

	for _, proof := range v.proofs {
		if err := v.zpv.VerifyProof(proof); err != nil {
			return proofErrBuilder(err)
		}
	}

	return nil
}

func proofErrBuilder(baseErr error) error {
	return fmt.Errorf("%w: %w", errProofsInvalid, baseErr)
}
