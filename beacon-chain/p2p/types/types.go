// Package types contains all the respective p2p types that are required for sync
// but cannot be represented as a protobuf schema. This package also contains those
// types associated fast ssz methods.
package types

import (
	"bytes"
	"encoding/binary"
	"sort"

	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	eth "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
	ssz "github.com/prysmaticlabs/fastssz"
)

const (
	maxErrorLength       = 256
	bytesPerLengthOffset = 4
)

// SSZBytes is a bytes slice that satisfies the fast-ssz interface.
type SSZBytes []byte

// HashTreeRoot hashes the uint64 object following the SSZ standard.
func (b *SSZBytes) HashTreeRoot() ([32]byte, error) {
	return ssz.HashWithDefaultHasher(b)
}

// HashTreeRootWith hashes the uint64 object with the given hasher.
func (b *SSZBytes) HashTreeRootWith(hh *ssz.Hasher) error {
	indx := hh.Index()
	hh.PutBytes(*b)
	hh.Merkleize(indx)
	return nil
}

// BeaconBlockByRootsReq specifies the block by roots request type.
type BeaconBlockByRootsReq [][fieldparams.RootLength]byte

// MarshalSSZTo marshals the block by roots request with the provided byte slice.
func (r *BeaconBlockByRootsReq) MarshalSSZTo(dst []byte) ([]byte, error) {
	marshalledObj, err := r.MarshalSSZ()
	if err != nil {
		return nil, err
	}
	return append(dst, marshalledObj...), nil
}

// MarshalSSZ Marshals the block by roots request type into the serialized object.
func (r *BeaconBlockByRootsReq) MarshalSSZ() ([]byte, error) {
	if len(*r) > int(params.BeaconConfig().MaxRequestBlocks) {
		return nil, errors.Errorf("beacon block by roots request exceeds max size: %d > %d", len(*r), params.BeaconConfig().MaxRequestBlocks)
	}
	buf := make([]byte, 0, r.SizeSSZ())
	for _, r := range *r {
		buf = append(buf, r[:]...)
	}
	return buf, nil
}

// SizeSSZ returns the size of the serialized representation.
func (r *BeaconBlockByRootsReq) SizeSSZ() int {
	return len(*r) * fieldparams.RootLength
}

// UnmarshalSSZ unmarshals the provided bytes buffer into the
// block by roots request object.
func (r *BeaconBlockByRootsReq) UnmarshalSSZ(buf []byte) error {
	bufLen := len(buf)
	maxLength := int(params.BeaconConfig().MaxRequestBlocks * fieldparams.RootLength)
	if bufLen > maxLength {
		return errors.Errorf("expected buffer with length of up to %d but received length %d", maxLength, bufLen)
	}
	if bufLen%fieldparams.RootLength != 0 {
		return ssz.ErrIncorrectByteSize
	}
	numOfRoots := bufLen / fieldparams.RootLength
	roots := make([][fieldparams.RootLength]byte, 0, numOfRoots)
	for i := 0; i < numOfRoots; i++ {
		var rt [fieldparams.RootLength]byte
		copy(rt[:], buf[i*fieldparams.RootLength:(i+1)*fieldparams.RootLength])
		roots = append(roots, rt)
	}
	*r = roots
	return nil
}

// ErrorMessage describes the error message type.
type ErrorMessage []byte

// MarshalSSZTo marshals the error message with the provided byte slice.
func (m *ErrorMessage) MarshalSSZTo(dst []byte) ([]byte, error) {
	marshalledObj, err := m.MarshalSSZ()
	if err != nil {
		return nil, err
	}
	return append(dst, marshalledObj...), nil
}

// MarshalSSZ Marshals the error message into the serialized object.
func (m *ErrorMessage) MarshalSSZ() ([]byte, error) {
	if len(*m) > maxErrorLength {
		return nil, errors.Errorf("error message exceeds max size: %d > %d", len(*m), maxErrorLength)
	}
	buf := make([]byte, m.SizeSSZ())
	copy(buf, *m)
	return buf, nil
}

// SizeSSZ returns the size of the serialized representation.
func (m *ErrorMessage) SizeSSZ() int {
	return len(*m)
}

// UnmarshalSSZ unmarshals the provided bytes buffer into the
// error message object.
func (m *ErrorMessage) UnmarshalSSZ(buf []byte) error {
	bufLen := len(buf)
	maxLength := maxErrorLength
	if bufLen > maxLength {
		return errors.Errorf("expected buffer with length of upto %d but received length %d", maxLength, bufLen)
	}
	errMsg := make([]byte, bufLen)
	copy(errMsg, buf)
	*m = errMsg
	return nil
}

// BlobSidecarsByRootReq is used to specify a list of blob targets (root+index) in a BlobSidecarsByRoot RPC request.
type BlobSidecarsByRootReq []*eth.BlobIdentifier

// BlobIdentifier is a fixed size value, so we can compute its fixed size at start time (see init below)
var blobIdSize int

// SizeSSZ returns the size of the serialized representation.
func (b *BlobSidecarsByRootReq) SizeSSZ() int {
	return len(*b) * blobIdSize
}

// MarshalSSZTo appends the serialized BlobSidecarsByRootReq value to the provided byte slice.
func (b *BlobSidecarsByRootReq) MarshalSSZTo(dst []byte) ([]byte, error) {
	// A List without an enclosing container is marshaled exactly like a vector, no length offset required.
	marshalledObj, err := b.MarshalSSZ()
	if err != nil {
		return nil, err
	}
	return append(dst, marshalledObj...), nil
}

// MarshalSSZ serializes the BlobSidecarsByRootReq value to a byte slice.
func (b *BlobSidecarsByRootReq) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, len(*b)*blobIdSize)
	for i, id := range *b {
		by, err := id.MarshalSSZ()
		if err != nil {
			return nil, err
		}
		copy(buf[i*blobIdSize:(i+1)*blobIdSize], by)
	}
	return buf, nil
}

// UnmarshalSSZ unmarshals the provided bytes buffer into the
// BlobSidecarsByRootReq value.
func (b *BlobSidecarsByRootReq) UnmarshalSSZ(buf []byte) error {
	bufLen := len(buf)
	maxLength := int(params.BeaconConfig().MaxRequestBlobSidecarsElectra) * blobIdSize
	if bufLen > maxLength {
		return errors.Wrapf(ssz.ErrIncorrectListSize, "expected buffer with length of up to %d but received length %d", maxLength, bufLen)
	}
	if bufLen%blobIdSize != 0 {
		return errors.Wrapf(ssz.ErrIncorrectByteSize, "size=%d", bufLen)
	}
	count := bufLen / blobIdSize
	*b = make([]*eth.BlobIdentifier, count)
	for i := 0; i < count; i++ {
		id := &eth.BlobIdentifier{}
		err := id.UnmarshalSSZ(buf[i*blobIdSize : (i+1)*blobIdSize])
		if err != nil {
			return err
		}
		(*b)[i] = id
	}
	return nil
}

var _ sort.Interface = (*BlobSidecarsByRootReq)(nil)

// Less reports whether the element with index i must sort before the element with index j.
// BlobIdentifier will be sorted in lexicographic order by root, with Blob Index as tiebreaker for a given root.
func (s BlobSidecarsByRootReq) Less(i, j int) bool {
	rootCmp := bytes.Compare((s)[i].BlockRoot, (s)[j].BlockRoot)
	if rootCmp != 0 {
		// They aren't equal; return true if i < j, false if i > j.
		return rootCmp < 0
	}
	// They are equal; blob index is the tie breaker.
	return (s)[i].Index < (s)[j].Index
}

// Swap swaps the elements with indexes i and j.
func (s BlobSidecarsByRootReq) Swap(i, j int) {
	(s)[i], (s)[j] = (s)[j], (s)[i]
}

// Len is the number of elements in the collection.
func (s BlobSidecarsByRootReq) Len() int {
	return len(s)
}

// ====================================
// DataColumnsByRootIdentifiers section
// ====================================
var _ ssz.Marshaler = DataColumnsByRootIdentifiers{}
var _ ssz.Unmarshaler = (*DataColumnsByRootIdentifiers)(nil)

// DataColumnsByRootIdentifiers is used to specify a list of data column targets (root+index) in a DataColumnSidecarsByRoot RPC request.
type DataColumnsByRootIdentifiers []*eth.DataColumnsByRootIdentifier

// DataColumnIdentifier is a fixed size value, so we can compute its fixed size at start time (see init below)
var dataColumnIdSize int

// UnmarshalSSZ implements ssz.Unmarshaler. It unmarshals the provided bytes buffer into the DataColumnSidecarsByRootReq value.
func (d *DataColumnsByRootIdentifiers) UnmarshalSSZ(buf []byte) error {
	// Exit early if the buffer is too small.
	if len(buf) < bytesPerLengthOffset {
		return nil
	}

	// Get the size of the offsets.
	offsetEnd := binary.LittleEndian.Uint32(buf[:bytesPerLengthOffset])
	if offsetEnd%bytesPerLengthOffset != 0 {
		return errors.Errorf("expected offsets size to be a multiple of %d but got %d", bytesPerLengthOffset, offsetEnd)
	}

	count := offsetEnd / bytesPerLengthOffset
	if count < 1 {
		return nil
	}

	maxSize := params.BeaconConfig().MaxRequestBlocksDeneb
	if uint64(count) > maxSize {
		return errors.Errorf("data column identifiers list exceeds max size: %d > %d", count, maxSize)
	}

	if offsetEnd > uint32(len(buf)) {
		return errors.Errorf("offsets value %d larger than buffer %d", offsetEnd, len(buf))
	}
	valueStart := offsetEnd

	// Decode the identifers.
	*d = make([]*eth.DataColumnsByRootIdentifier, count)
	var start uint32
	end := uint32(len(buf))
	for i := count; i > 0; i-- {
		offsetEnd -= bytesPerLengthOffset
		start = binary.LittleEndian.Uint32(buf[offsetEnd : offsetEnd+bytesPerLengthOffset])
		if start > end {
			return errors.Errorf("expected offset[%d] %d to be less than %d", i-1, start, end)
		}
		if start < valueStart {
			return errors.Errorf("offset[%d] %d indexes before value section %d", i-1, start, valueStart)
		}
		// Decode the identifier.
		ident := &eth.DataColumnsByRootIdentifier{}
		if err := ident.UnmarshalSSZ(buf[start:end]); err != nil {
			return err
		}
		(*d)[i-1] = ident
		end = start
	}

	return nil
}

func (d DataColumnsByRootIdentifiers) MarshalSSZ() ([]byte, error) {
	var err error
	count := len(d)
	maxSize := params.BeaconConfig().MaxRequestBlocksDeneb
	if uint64(count) > maxSize {
		return nil, errors.Errorf("data column identifiers list exceeds max size: %d > %d", count, maxSize)
	}

	if len(d) == 0 {
		return []byte{}, nil
	}
	sizes := make([]uint32, count)
	valTotal := uint32(0)
	for i, elem := range d {
		if elem == nil {
			return nil, errors.New("nil item in DataColumnsByRootIdentifiers list")
		}
		sizes[i] = uint32(elem.SizeSSZ())
		valTotal += sizes[i]
	}
	offSize := uint32(4 * len(d))
	out := make([]byte, offSize, offSize+valTotal)
	for i := range sizes {
		binary.LittleEndian.PutUint32(out[i*4:i*4+4], offSize)
		offSize += sizes[i]
	}
	for _, elem := range d {
		out, err = elem.MarshalSSZTo(out)
		if err != nil {
			return nil, err
		}
	}

	return out, nil
}

// MarshalSSZTo implements ssz.Marshaler. It appends the serialized DataColumnSidecarsByRootReq value to the provided byte slice.
func (d DataColumnsByRootIdentifiers) MarshalSSZTo(dst []byte) ([]byte, error) {
	obj, err := d.MarshalSSZ()
	if err != nil {
		return nil, err
	}
	return append(dst, obj...), nil
}

// SizeSSZ implements ssz.Marshaler. It returns the size of the serialized representation.
func (d DataColumnsByRootIdentifiers) SizeSSZ() int {
	size := 0
	for i := 0; i < len(d); i++ {
		size += 4
		size += (d)[i].SizeSSZ()
	}
	return size
}

func init() {
	blobSizer := &eth.BlobIdentifier{}
	blobIdSize = blobSizer.SizeSSZ()

	dataColumnSizer := &eth.DataColumnSidecarsByRangeRequest{}
	dataColumnIdSize = dataColumnSizer.SizeSSZ()
}
