package query

import (
	"errors"
)

type SSZIface interface {
	HashTreeRoot() ([32]byte, error)
	MarshalSSZ() ([]byte, error)
	UnmarshalSSZ([]byte) error
	// HashTreeRootWith(hh *ssz.Hasher) error
}

// HashTreeRoot calls the HashTreeRoot method on the stored value if it implements the Hashable interface.
// This leverages the SSZ-generated HashTreeRoot methods from Fastssz.
func (info *sszInfo) HashTreeRoot() ([32]byte, error) {
	if info == nil {
		return [32]byte{}, errors.New("sszInfo is nil")
	}

	if info.iface == nil {
		return [32]byte{}, errors.New("not stored iface")
	}

	// Check if the value implements the Hashable interface
	if hashable, ok := info.iface.(interface{ HashTreeRoot() ([32]byte, error) }); ok {
		return hashable.HashTreeRoot()
	}

	return [32]byte{}, errors.New("stored interface does not implement HashTreeRoot() method")
}

func (info *sszInfo) MarshalSSZ() ([]byte, error) {
	if info == nil {
		return nil, errors.New("sszInfo is nil")
	}

	if info.iface == nil {
		return nil, errors.New("not stored iface")
	}

	// Check if the value implements the SSZMarshaler interface
	if marshaler, ok := info.iface.(interface{ MarshalSSZ() ([]byte, error) }); ok {
		return marshaler.MarshalSSZ()
	}

	return nil, errors.New("stored interface does not implement MarshalSSZ() method")
}

func (info *sszInfo) UnmarshalSSZ(data []byte) error {
	if info == nil {
		return errors.New("sszInfo is nil")
	}

	if info.iface == nil {
		return errors.New("not stored iface")
	}

	// Check if the value implements the SSZUnmarshaler interface
	if unmarshaler, ok := info.iface.(interface{ UnmarshalSSZ([]byte) error }); ok {
		return unmarshaler.UnmarshalSSZ(data)
	}

	return errors.New("stored interface does not implement UnmarshalSSZ() method")
}
