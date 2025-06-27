package ssz

import "github.com/pkg/errors"

var (
	ErrInvalidEncodingLength     = errors.New("invalid encoded length")
	ErrInvalidFixedEncodingLen   = errors.Wrap(ErrInvalidEncodingLength, "not multiple of fixed size")
	ErrEncodingSmallerThanOffset = errors.Wrap(ErrInvalidEncodingLength, "smaller than a single offset")
	ErrInvalidOffset             = errors.New("invalid offset")
	ErrOffsetIntoFixed           = errors.Wrap(ErrInvalidOffset, "does not point past fixed section of encoding")
	ErrOffsetExceedsBuffer       = errors.Wrap(ErrInvalidOffset, "exceeds buffer length")
	ErrNegativeRelativeOffset    = errors.Wrap(ErrInvalidOffset, "less than previous offset")
	ErrOffsetInsufficient        = errors.Wrap(ErrInvalidOffset, "insufficient difference relative to previous")
	ErrOffsetSectionMisaligned   = errors.Wrap(ErrInvalidOffset, "offset bytes are not a multiple of offset size")

	ErrOffsetDecodedMismatch = errors.New("unmarshaled size does not relative offsets")
)
