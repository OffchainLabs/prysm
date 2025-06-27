// Package types contains all the respective p2p types that are required for sync
// but cannot be represented as a protobuf schema. This package also contains those
// types associated fast ssz methods.
package types

import (
	"bytes"
	"sort"

	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/encoding/ssz"
	eth "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
	fastssz "github.com/prysmaticlabs/fastssz"
)

const (
	maxErrorLength = 256
)

// SSZBytes is a bytes slice that satisfies the fast-ssz interface.
type SSZBytes []byte

// HashTreeRoot hashes the uint64 object following the SSZ standard.
func (b *SSZBytes) HashTreeRoot() ([32]byte, error) {
	return fastssz.HashWithDefaultHasher(b)
}

// HashTreeRootWith hashes the uint64 object with the given hasher.
func (b *SSZBytes) HashTreeRootWith(hh *fastssz.Hasher) error {
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
		return fastssz.ErrIncorrectByteSize
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

var BlobSidecarsByRootReqSerdes = ssz.NewListFixedElementSerdes[*eth.BlobIdentifier](func() *eth.BlobIdentifier {
	return &eth.BlobIdentifier{}
})

// BlobSidecarsByRootReq is used to specify a list of blob targets (root+index) in a BlobSidecarsByRoot RPC request.
type BlobSidecarsByRootReq []*eth.BlobIdentifier

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
	return BlobSidecarsByRootReqSerdes.Marshal(*b)
}

// UnmarshalSSZ unmarshals the provided bytes buffer into the
// BlobSidecarsByRootReq value.
func (b *BlobSidecarsByRootReq) UnmarshalSSZ(buf []byte) error {
	v, err := BlobSidecarsByRootReqSerdes.Unmarshal(buf)
	if err != nil {
		return errors.Wrapf(err, "failed to unmarshal BlobSidecarsByRootReq")
	}
	if len(v) > int(params.BeaconConfig().MaxRequestBlobSidecarsElectra) {
		return ErrMaxBlobReqExceeded
	}
	*b = v
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
var _ fastssz.Marshaler = DataColumnsByRootIdentifiers{}
var _ fastssz.Unmarshaler = &DataColumnsByRootIdentifiers{}

// DataColumnsByRootIdentifiers is used to specify a list of data column targets (root+index) in a DataColumnSidecarsByRoot RPC request.
type DataColumnsByRootIdentifiers []*eth.DataColumnsByRootIdentifier

var DataColumnsByRootIdentifiersSerdes = ssz.NewListVariableElementSerdes[*eth.DataColumnsByRootIdentifier](func() *eth.DataColumnsByRootIdentifier {
	return &eth.DataColumnsByRootIdentifier{}
})

func (d *DataColumnsByRootIdentifiers) UnmarshalSSZ(buf []byte) error {
	//v, err := DataColumnsByRootIdentifiersSerdes.Unmarshal(buf)
	v, err := DataColumnsByRootIdentifiersSerdes.Unmarshal(buf)
	if err != nil {
		return errors.Wrapf(err, "failed to unmarshal DataColumnsByRootIdentifiers")
	}
	*d = v
	return nil
}

func (d DataColumnsByRootIdentifiers) MarshalSSZ() ([]byte, error) {
	return DataColumnsByRootIdentifiersSerdes.Marshal(d)
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
