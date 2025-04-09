package interfaces

import (
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
)

var ErrInvalidCast = errors.New("unable to cast between types")

type InvalidCastError struct {
	from int
	to   int
}

func (e *InvalidCastError) Error() string {
	return errors.Wrapf(ErrInvalidCast,
		"from=%s(%d), to=%s(%d)", version.String(e.from), e.from, version.String(e.to), e.to).
		Error()
}

func (*InvalidCastError) Is(err error) bool {
	return errors.Is(err, ErrInvalidCast)
}

func NewInvalidCastError(from, to int) *InvalidCastError {
	return &InvalidCastError{from: from, to: to}
}
