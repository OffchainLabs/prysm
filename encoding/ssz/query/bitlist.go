package query

import (
	"errors"
	"fmt"

	"github.com/prysmaticlabs/go-bitfield"
)

// bitlistInfo holds information about a SSZ Bitlist type.
//
// Same as listInfo, but limit/length are in bits, not elements.
type bitlistInfo struct {
	// limit is the maximum number of bits in the bitlist.
	limit uint64
	// length is the actual number of bits at runtime (0 if not set).
	length uint64
}

func (l *bitlistInfo) Limit() uint64 {
	if l == nil {
		return 0
	}
	return l.limit
}

func (l *bitlistInfo) Length() uint64 {
	if l == nil {
		return 0
	}
	return l.length
}

func (l *bitlistInfo) SetLength(length uint64) error {
	if l == nil {
		return errors.New("bitlistInfo is nil")
	}

	if length > l.limit {
		return fmt.Errorf("length %d exceeds limit %d", length, l.limit)
	}

	l.length = length
	return nil
}

// SetLengthFromBytes determines the actual bitlist length from SSZ-encoded bytes.
func (l *bitlistInfo) SetLengthFromBytes(rawBytes []byte) error {
	// Wrap rawBytes in a Bitlist to use existing methods.
	bl := bitfield.Bitlist(rawBytes)
	return l.SetLength(bl.Len())
}

// Size returns the size in bytes for this bitlist.
// Note that while serializing, 1 bit is added for the delimiter bit,
// which results in ceil((length + 1) / 8) bytes.
// Note that `(length / 8) + 1` is equivalent to `ceil((length + 1) / 8)`.
// Example: length=0 -> size=1, length=7 -> size=1, length=8 -> size=2
// Reference: https://github.com/ethereum/consensus-specs/blob/master/ssz/simple-serialize.md#bitlistn-progressivebitlist
func (l *bitlistInfo) Size() uint64 {
	if l == nil {
		return 0
	}
	return (l.length / 8) + 1
}
