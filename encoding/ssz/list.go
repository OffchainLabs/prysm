package ssz

import (
	"encoding/binary"
	"math"

	"github.com/pkg/errors"
)

const offsetLen = 4 // each list element offset is a 4-byte uint32

type Marshalable interface {
	MarshalSSZTo(buf []byte) ([]byte, error)
	SizeSSZ() int
}

type Unmarshalable interface {
	UnmarshalSSZ(buf []byte) error
	SizeSSZ() int
}

// MarshalListFixedElement encodes a slice of fixed sized elements as an ssz list.
// A list of fixed-size elements is marshaled by concatenating the marshaled bytes
// of each element in the list.
//
// MarshalListVariableElement should be used for variable-sized elements.
// SSZ Lists have different encoding rules depending whether their elements are fixed- or variable-sized,
// and we can't differentiate them by the ssz interface, so it is the caller's responsibility to
// pick the correct method.
func MarshalListFixedElement[T Marshalable](elems []T) ([]byte, error) {
	if len(elems) == 0 {
		return nil, nil
	}
	size := elems[0].SizeSSZ()
	buf := make([]byte, 0, len(elems)*size)
	for _, elem := range elems {
		if elem.SizeSSZ() != size {
			return nil, ErrInvalidFixedEncodingLen
		}
		var err error
		buf, err = elem.MarshalSSZTo(buf)
		if err != nil {
			return nil, errors.Wrap(err, "marshal ssz")
		}
	}
	return buf, nil
}

// MarshalListVariableElement marshals a list of variable-sized elements.
// A list of variable-sized elements is marshaled by first writing the offsets of each element to the
// beginning of the byte sequence (the fixed size section of the variable sized list container), followed
// byt the encoded values of each element at the indicated offset relative to the beginning of the byte sequence.
//
// MarshalListFixedElement should be used for fixed-size elements.
// SSZ Lists have different encoding rules depending whether their elements are fixed- or variable-sized,
// and we can't differentiate them by the ssz interface, so it is the caller's responsibility to
// pick the correct method.
func MarshalListVariableElement[T Marshalable](elems []T) ([]byte, error) {
	var err error
	var total uint32
	nElems := len(elems)
	if nElems == 0 {
		return nil, nil
	}
	sizes := make([]uint32, nElems)
	for i, e := range elems {
		sizes[i], err = safeUint32(e.SizeSSZ())
		if err != nil {
			return nil, err
		}
		total += sizes[i]
	}
	nextOffset, err := safeUint32(nElems * offsetLen)
	if err != nil {
		return nil, err
	}
	buf := make([]byte, 0, total+nextOffset)
	for _, size := range sizes {
		buf = binary.LittleEndian.AppendUint32(buf, nextOffset)
		nextOffset += size
	}
	for _, elem := range elems {
		buf, err = elem.MarshalSSZTo(buf)
		if err != nil {
			return nil, err
		}
	}
	return buf, nil
}

// UnmarshalListVariableElement unmarshals a ssz-encoded list of variable-sized elements.
// Because this generic method is parameterized by a [T Unmarshalable] interface type,
// it is unable to initialize elements of the list internally. That is why the caller must
// provide the `newt` function that returns a new instance of the type [T] to be unmarshaled.
// This func will be called for each element in the list to create a new instance of [T].
//
// UnmarshalListFixedElement should be used for fixed-size elements.
// SSZ Lists have different encoding rules depending whether their elements are fixed- or variable-sized,
// and we can't differentiate them by the ssz interface, so it is the caller's responsibility to
// pick the correct method.
func UnmarshalListVariableElement[T Unmarshalable](buf []byte, newt func() T) ([]T, error) {
	bufLen := len(buf)
	if bufLen == 0 {
		return nil, nil
	}
	if bufLen < offsetLen {
		return nil, ErrEncodingSmallerThanOffset
	}
	fixedSize := uint32(newt().SizeSSZ())
	bufLen32 := uint32(bufLen)

	first := binary.LittleEndian.Uint32(buf)
	// Rather than just return a zero element list in this case,
	// we want to explicitly reject this input as invalid
	if first < offsetLen {
		return nil, ErrOffsetIntoFixed
	}
	if first%offsetLen != 0 {
		return nil, ErrOffsetSectionMisaligned
	}
	if first > bufLen32 {
		return nil, ErrOffsetExceedsBuffer
	}

	nElems := int(first) / offsetLen // lint:ignore uintcast -- int has higher precision than uint32 on 64 bit systems, so this is 100% safe
	if nElems == 0 {
		return nil, nil
	}
	buf = buf[offsetLen:]
	sizes := make([]uint32, nElems)

	// We've already looked at the offset of the first element (to perform validation on it)
	// so we just need to iterate over the remaining offsets, aka nElems-1 times.
	// The size of each element is computed relative to the next offset, so this loop is effectively
	// looking ahead +1 (starting with a `buf` that has already had the first offset sliced off),
	// with the final element handled as a special case outside the loop (using the size of the entire buffer
	// as the ending bound).
	previous := first
	for i := 0; i < nElems-1; i++ {
		next := binary.LittleEndian.Uint32(buf)
		if next > bufLen32 {
			return nil, ErrOffsetExceedsBuffer
		}
		if next < previous {
			return nil, ErrNegativeRelativeOffset
		}
		sizes[i] = next - previous
		if sizes[i] < fixedSize {
			return nil, ErrOffsetInsufficient
		}
		buf = buf[offsetLen:]
		previous = next
	}
	sizes[len(sizes)-1] = bufLen32 - previous
	elements := make([]T, nElems)
	for i, size := range sizes {
		elem := newt()
		if err := elem.UnmarshalSSZ(buf[:size]); err != nil {
			return nil, errors.Wrap(err, "unmarshal ssz")
		}
		szi := int(size) // lint:ignore uintcast -- int has higher precision than uint32 on 64 bit systems, so this is 100% safe
		if elem.SizeSSZ() != szi {
			return nil, ErrOffsetDecodedMismatch
		}
		elements[i] = elem
		buf = buf[size:]
	}
	return elements, nil
}

// UnmarshalListFixedElement unmarshals a ssz-encoded list of variable-sized elements.
// A List of fixed-size elements is encoded as a concatenation of the marshaled bytes of each
// element, so after performing some safety checks on the alignment and size of the buffer,
// we simply iterate over the buffer in chunks of the fixed size and unmarshal each element.
// Because this generic method is parameterized by a [T Unmarshalable] interface type,
// it is unable to initialize elements of the list internally. That is why the caller must
// provide the `newt` function that returns a new instance of the type [T] to be unmarshaled.
// This func will be called for each element in the list to create a new instance of [T].
//
// UnmarshalListFixedElement should be used for fixed-size elements.
// SSZ Lists have different encoding rules depending whether their elements are fixed- or variable-sized,
// and we can't differentiate them by the ssz interface, so it is the caller's responsibility to
// pick the correct method.
func UnmarshalListFixedElement[T Unmarshalable](buf []byte, newt func() T) ([]T, error) {
	bufLen := len(buf)
	if bufLen == 0 {
		return nil, nil
	}
	fixedSize := newt().SizeSSZ()
	if bufLen%fixedSize != 0 {
		return nil, ErrInvalidFixedEncodingLen
	}
	nElems := bufLen / fixedSize
	elements := make([]T, nElems)
	for i := 0; i < nElems; i++ {
		elem := newt()
		if err := elem.UnmarshalSSZ(buf[i*fixedSize : (i+1)*fixedSize]); err != nil {
			return nil, errors.Wrap(err, "unmarshal ssz")
		}
		elements[i] = elem
	}
	return elements, nil
}

func safeUint32(val int) (uint32, error) {
	if val < 0 || val > math.MaxUint32 {
		return 0, errors.New("value exceeds uint32 range")
	}
	return uint32(val), nil // lint:ignore uintcast -- integer value explicitly checked to prevent truncation
}
