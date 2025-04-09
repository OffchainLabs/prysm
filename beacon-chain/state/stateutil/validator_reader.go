package stateutil

import (
	multi_value_slice "github.com/prysmaticlabs/prysm/v5/container/multi-value-slice"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
)

// ValReader specifies an interface through which we can access the validator registry.
type ValReader interface {
	Len() int
	At(i int) (*ethpb.Validator, error)
}

// ValSliceReader describes a struct that conforms to the ValReader interface
type ValSliceReader struct {
	Validators []*ethpb.Validator
}

// NewValSliceReader constructs a ValSliceReader object.
func NewValSliceReader(vals []*ethpb.Validator) ValSliceReader {
	return ValSliceReader{Validators: vals}
}

// Len is the length of the validator registry.
func (v ValSliceReader) Len() int {
	return len(v.Validators)
}

// At returns the validator at the provided index.
func (v ValSliceReader) At(i int) (*ethpb.Validator, error) {
	return v.Validators[i], nil
}

// ValMultiValueSliceReader describes a struct that conforms to the ValReader interface.
// This struct is specifically designed for accessing validator data from a
// multivalue slice.
type ValMultiValueSliceReader struct {
	ValMVSlice *multi_value_slice.Slice[*ethpb.Validator]
	Identifier multi_value_slice.Identifiable
}

// NewValMultiValueSliceReader constructs a new val reader object.
func NewValMultiValueSliceReader(valSlice *multi_value_slice.Slice[*ethpb.Validator],
	identifier multi_value_slice.Identifiable) ValMultiValueSliceReader {
	return ValMultiValueSliceReader{
		ValMVSlice: valSlice,
		Identifier: identifier,
	}
}

// Len is the length of the validator registry.
func (v ValMultiValueSliceReader) Len() int {
	return v.ValMVSlice.Len(v.Identifier)
}

// At returns the validator at the provided index.
func (v ValMultiValueSliceReader) At(i int) (*ethpb.Validator, error) {
	return v.ValMVSlice.At(v.Identifier, uint64(i))
}
