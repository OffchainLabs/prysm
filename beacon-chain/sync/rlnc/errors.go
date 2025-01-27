package rlnc

import "errors"

var ErrInvalidSize = errors.New("invalid size")
var ErrNoData = errors.New("no data")
var ErrIncorrectCommitments = errors.New("incorrect commitments")
var ErrInvalidMessage = errors.New("invalid message")
var ErrLinearlyDependentMessage = errors.New("linearly dependent message")
var ErrInvalidScalar = errors.New("invalid scalar encoding")
var ErrInvalidElement = errors.New("invalid element encoding")
var ErrSignatureNotVerified = errors.New("signature not verified") // ErrSignatureNotVerified is returned when the signature of a chunk is not yet verified.
