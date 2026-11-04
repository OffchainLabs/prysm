package forkchoice

import "github.com/pkg/errors"

var ErrUnknownCommonAncestor = errors.New("unknown common ancestor")

// ErrExecutionProofTypeKnown is returned when a verified execution proof of the same type is already known for the
// payload.
var ErrExecutionProofTypeKnown = errors.New("execution proof type already known")
