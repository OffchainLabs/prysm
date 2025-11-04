package server

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrIndexedValidationFail = "One or more messages failed validation"
	ErrIndexedBroadcastFail  = "One or more messages failed broadcast"
)

// DecodeError represents an error resulting from trying to decode an HTTP request.
// It tracks the full field name for which decoding failed.
type DecodeError struct {
	path []string
	err  error
}

// NewDecodeError wraps an error (either the initial decoding error or another DecodeError).
// The current field that failed decoding must be passed in.
func NewDecodeError(err error, field string) *DecodeError {
	var de *DecodeError
	ok := errors.As(err, &de)
	if ok {
		return &DecodeError{path: append([]string{field}, de.path...), err: de.err}
	}
	return &DecodeError{path: []string{field}, err: err}
}

// Error returns the formatted error message which contains the full field name and the actual decoding error.
func (e *DecodeError) Error() string {
	return fmt.Sprintf("could not decode %s: %s", strings.Join(e.path, "."), e.err.Error())
}

// IndexedErrorContainer wraps a collection of indexed errors.
type IndexedErrorContainer struct {
	Message  string          `json:"message"`
	Code     int             `json:"code"`
	Failures []*IndexedError `json:"failures"`
}

func (e *IndexedErrorContainer) StatusCode() int {
	return e.Code
}

// IndexedError represents an issue when processing a single indexed object e.g. an item in an array.
type IndexedError struct {
	Index   int    `json:"index"`
	Message string `json:"message"`
}

// BroadcastFailedError represents an error scenario where broadcasting a published message failed.
type BroadcastFailedError struct {
	msg string
	err error
}

// NewBroadcastFailedError creates a new instance of BroadcastFailedError.
func NewBroadcastFailedError(msg string, err error) *BroadcastFailedError {
	return &BroadcastFailedError{
		msg: msg,
		err: err,
	}
}

// Error returns the underlying error message.
func (e *BroadcastFailedError) Error() string {
	return fmt.Sprintf("could not broadcast %s: %s", e.msg, e.err.Error())
}
