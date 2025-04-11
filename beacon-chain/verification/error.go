package verification

import (
	"errors"

	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
)

// ErrInvalid is a general purpose verification failure that can be wrapped or joined to indicate
// a verification failure that should impact peer scoring.
var ErrInvalid = errors.New("verification failure")

// AsVerification joins the given error with the base ErrVerificationFailure error
// so that it can be tested with errors.Is()
func AsVerificationFailure(err error) error {
	return errors.Join(ErrInvalid, err)
}

var (
	// ErrBlobInvalid is joined with all other blob verification errors. This enables other packages to check for any sort of
	// verification error at one point, like sync code checking for peer scoring purposes.
	ErrBlobInvalid = AsVerificationFailure(errors.New("invalid blob"))

	// ErrBlobIndexInvalid means RequireBlobIndexInBounds failed.
	ErrBlobIndexInvalid = errors.Join(ErrBlobInvalid, errors.New("incorrect blob sidecar index"))

	// ErrFromFutureSlot means RequireSlotNotTooEarly failed.
	ErrFromFutureSlot = errors.Join(ErrBlobInvalid, errors.New("slot is too far in the future"))

	// ErrSlotNotAfterFinalized means RequireSlotAboveFinalized failed.
	ErrSlotNotAfterFinalized = errors.Join(ErrBlobInvalid, errors.New("slot <= finalized checkpoint"))

	// ErrInvalidProposerSignature means RequireValidProposerSignature failed.
	ErrInvalidProposerSignature = errors.Join(ErrBlobInvalid, errors.New("proposer signature could not be verified"))

	// ErrSidecarParentNotSeen means RequireSidecarParentSeen failed.
	ErrSidecarParentNotSeen = errors.Join(ErrBlobInvalid, errors.New("parent root has not been seen"))

	// ErrSidecarParentInvalid means RequireSidecarParentValid failed.
	ErrSidecarParentInvalid = errors.Join(ErrBlobInvalid, errors.New("parent block is not valid"))

	// ErrSlotNotAfterParent means RequireSidecarParentSlotLower failed.
	ErrSlotNotAfterParent = errors.Join(ErrBlobInvalid, errors.New("slot <= slot"))

	// ErrSidecarNotFinalizedDescendent means RequireSidecarDescendsFromFinalized failed.
	ErrSidecarNotFinalizedDescendent = errors.Join(ErrBlobInvalid, errors.New("parent is not descended from the finalized block"))

	// ErrSidecarInclusionProofInvalid means RequireSidecarInclusionProven failed.
	ErrSidecarInclusionProofInvalid = errors.Join(ErrBlobInvalid, errors.New("sidecar inclusion proof verification failed"))

	// ErrSidecarKzgProofInvalid means RequireSidecarKzgProofVerified failed.
	ErrSidecarKzgProofInvalid = errors.Join(ErrBlobInvalid, errors.New("sidecar kzg commitment proof verification failed"))

	// ErrSidecarUnexpectedProposer means RequireSidecarProposerExpected failed.
	ErrSidecarUnexpectedProposer = errors.Join(ErrBlobInvalid, errors.New("sidecar was not proposed by the expected proposer_index"))

	// ErrMissingVerification indicates that the given verification function was never performed on the value.
	ErrMissingVerification = errors.Join(ErrBlobInvalid, errors.New("verification was not performed for requirement"))

	// ErrBatchSignatureMismatch is returned by VerifiedROBlobs when any of the blobs in the batch have a signature
	// which does not match the signature for the block with a corresponding root.
	ErrBatchSignatureMismatch = errors.Join(ErrBlobInvalid, errors.New("Sidecar block header signature does not match signed block"))
	// ErrBatchBlockRootMismatch is returned by VerifiedROBlobs in the scenario where the root of the given signed block
	// does not match the block header in one of the corresponding sidecars.
	ErrBatchBlockRootMismatch = errors.Join(ErrBlobInvalid, errors.New("Sidecar block header root does not match signed block"))
)

// errVerificationImplementationFault indicates that a code path yielding VerifiedROBlobs has an implementation
// error, leading it to call VerifiedROBlobError with a nil error.
var errVerificationImplementationFault = errors.New("could not verify blob data or create a valid VerifiedROBlob.")

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

// VerifiedROBlobError can be used by methods that have a VerifiedROBlob return type but do not have permission to
// create a value of that type in order to generate an error return value.
func VerifiedROBlobError(err error) (blocks.VerifiedROBlob, error) {
	if err == nil {
		return blocks.VerifiedROBlob{}, errVerificationImplementationFault
	}
	return blocks.VerifiedROBlob{}, err
}
