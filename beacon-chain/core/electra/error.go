package electra

import "github.com/pkg/errors"

type execReqErr struct {
	error
}

// IsExecutionRequestError returns true if the error has `execReqErr`.
func IsExecutionRequestError(e error) bool {
	if e == nil {
		return false
	}
	var d execReqErr
	return errors.As(e, &d)
}
