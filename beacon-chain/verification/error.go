package verification

import "github.com/pkg/errors"

var (
	// ErrFromFutureSlot means RequireSlotNotTooEarly failed.
	ErrFromFutureSlot = errors.New("slot is too far in the future")

	// ErrSlotNotAfterFinalized means RequireSlotAboveFinalized failed.
	ErrSlotNotAfterFinalized = errors.New("slot <= finalized checkpoint")

	// ErrInvalidProposerSignature means RequireValidProposerSignature failed.
	ErrInvalidProposerSignature = errors.New("proposer signature could not be verified")

	// ErrSidecarParentNotSeen means RequireSidecarParentSeen failed.
	ErrSidecarParentNotSeen = errors.New("parent root has not been seen")

	// ErrSidecarParentInvalid means RequireSidecarParentValid failed.
	ErrSidecarParentInvalid = errors.New("parent block is not valid")

	// ErrSlotNotAfterParent means RequireSidecarParentSlotLower failed.
	ErrSlotNotAfterParent = errors.New("slot <= slot")

	// ErrSidecarNotFinalizedDescendent means RequireSidecarDescendsFromFinalized failed.
	ErrSidecarNotFinalizedDescendent = errors.New("parent is not descended from the finalized block")

	// ErrSidecarInclusionProofInvalid means RequireSidecarInclusionProven failed.
	ErrSidecarInclusionProofInvalid = errors.New("sidecar inclusion proof verification failed")

	// ErrSidecarKzgProofInvalid means RequireSidecarKzgProofVerified failed.
	ErrSidecarKzgProofInvalid = errors.New("sidecar kzg commitment proof verification failed")

	// ErrSidecarUnexpectedProposer means RequireSidecarProposerExpected failed.
	ErrSidecarUnexpectedProposer = errors.New("sidecar was not proposed by the expected proposer_index")

	// ErrMissingVerification indicates that the given verification function was never performed on the value.
	ErrMissingVerification = errors.New("verification was not performed for requirement")
)

// VerificationMultiError is a custom error that can be used to access individual verification failures.
type VerificationMultiError struct {
	r   *results
	err error
}

// Unwrap is used by errors.Is to unwrap errors.
func (ve VerificationMultiError) Unwrap() error {
	if ve.err == nil {
		return nil
	}
	return ve.err
}

// Error satisfies the standard error interface.
func (ve VerificationMultiError) Error() string {
	if ve.err == nil {
		return ""
	}
	return ve.err.Error()
}

// Failures provides access to map of Requirements->error messages
// so that calling code can introspect on what went wrong.
func (ve VerificationMultiError) Failures() map[Requirement]error {
	return ve.r.failures()
}

func newVerificationMultiError(r *results, err error) VerificationMultiError {
	return VerificationMultiError{r: r, err: err}
}
