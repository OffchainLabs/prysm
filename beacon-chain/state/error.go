package state

import "errors"

var (
	// ErrNilValidatorsInState returns when accessing validators in the state while the state has a
	// nil slice for the validators field.
	ErrNilValidatorsInState = errors.New("state has nil validator slice")

	// ErrFieldElementProofUnsupported indicates that the state's optimized element proof path
	// cannot prove the requested field element.
	ErrFieldElementProofUnsupported = errors.New("field element proof unsupported")
)
