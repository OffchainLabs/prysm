package rlnc

import "errors"

var ErrInvalidSize = errors.New("invalid size")
var ErrNoData = errors.New("no data")
var ErrIncorrectCommitments = errors.New("incorrect commitments")
var ErrInvalidMessage = errors.New("invalid message")
var ErrLinearlyDependentMessage = errors.New("linearly dependent message")
