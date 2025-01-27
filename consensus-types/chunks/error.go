package chunks

import "errors"

var ErrNilObject = errors.New("nil object")     // ErrNilObject is returned when a nil object is received instead of a valid chunk
var ErrInvalidType = errors.New("invalid type") // ErrInvalidType is returned when an invalid type is received instead of a valid chunk
