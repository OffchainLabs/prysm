package filters

import "errors"

var (
	ErrIncompatibleFilters = errors.New("combination of filters is not valid")
	ErrNotSet              = errors.New("filter was not set")
	ErrInvalidQuery        = errors.New("invalid query")
)
