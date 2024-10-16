package primitives

import (
	"fmt"
	"math"

	fssz "github.com/prysmaticlabs/fastssz"
)

var _ fssz.HashRoot = (PTCStatus)(0)
var _ fssz.Marshaler = (*PTCStatus)(nil)
var _ fssz.Unmarshaler = (*PTCStatus)(nil)

// PTCStatus represents a single payload status. These are the
// possible votes that the Payload Timeliness Committee can cast
// in ePBS when attesting for an execution payload.
type PTCStatus uint64

// Defined constants
const (
	PAYLOAD_ABSENT         PTCStatus = 0
	PAYLOAD_PRESENT        PTCStatus = 1
	PAYLOAD_WITHHELD       PTCStatus = 2
	PAYLOAD_INVALID_STATUS PTCStatus = 3
)

// HashTreeRoot --
func (s PTCStatus) HashTreeRoot() ([32]byte, error) {
	return fssz.HashWithDefaultHasher(s)
}

// HashTreeRootWith --
func (s PTCStatus) HashTreeRootWith(hh *fssz.Hasher) error {
	if s > math.MaxUint8 {
		return fmt.Errorf("expected uint8 value, received %d", uint64(s))
	}
	hh.PutUint8(uint8(s))
	return nil
}

// UnmarshalSSZ --
func (s *PTCStatus) UnmarshalSSZ(buf []byte) error {
	if len(buf) != s.SizeSSZ() {
		return fmt.Errorf("expected buffer of length %d received %d", s.SizeSSZ(), len(buf))
	}
	*s = PTCStatus(fssz.UnmarshallUint8(buf))
	return nil
}

// MarshalSSZTo --
func (s *PTCStatus) MarshalSSZTo(dst []byte) ([]byte, error) {
	marshalled, err := s.MarshalSSZ()
	if err != nil {
		return nil, err
	}
	return append(dst, marshalled...), nil
}

// MarshalSSZ --
func (s *PTCStatus) MarshalSSZ() ([]byte, error) {
	if *s > math.MaxUint8 {
		return nil, fmt.Errorf("expected uint8 value, received %d", uint64(*s))
	}
	marshalled := fssz.MarshalUint8([]byte{}, uint8(*s))
	return marshalled, nil
}

// SizeSSZ --
func (s *PTCStatus) SizeSSZ() int {
	return 1
}
