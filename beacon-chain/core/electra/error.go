package electra

import "github.com/pkg/errors"

type execReqErr struct {
	error
}

// NewExecReqError creates a new execReqErr.
func NewExecReqError(msg string) error {
	return execReqErr{errors.New(msg)}
}

// IsExecutionRequestError returns true if the error has `execReqErr`.
func IsExecutionRequestError(e error) bool {
	if e == nil {
		return false
	}
	var d execReqErr
	return errors.As(e, &d)
}
