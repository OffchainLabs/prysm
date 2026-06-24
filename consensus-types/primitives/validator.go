package primitives

import (
	"fmt"

	fssz "github.com/prysmaticlabs/fastssz"
)

var _ fssz.HashRoot = (ValidatorIndex)(0)
var _ fssz.Marshaler = (*ValidatorIndex)(nil)
var _ fssz.Unmarshaler = (*ValidatorIndex)(nil)

// BuilderIndexFlag marks a ValidatorIndex as a BuilderIndex when the bit is set.
//
// Spec v1.6.1: BUILDER_INDEX_FLAG.
const BuilderIndexFlag uint64 = 1 << 40

// ValidatorIndex in eth2.
type ValidatorIndex uint64

// IsBuilderIndex returns true when the BuilderIndex flag is set on the validator index.
//
//	<spec fn="is_builder_index" fork="gloas" hash="2fbd46e9">
//	def is_builder_index(validator_index: ValidatorIndex) -> bool:
//	    return (validator_index & BUILDER_INDEX_FLAG) != 0
//	</spec>
func (v ValidatorIndex) IsBuilderIndex() bool {
	return uint64(v)&BuilderIndexFlag != 0
}

// ToBuilderIndex strips the builder flag from a validator index.
//
//	<spec fn="convert_validator_index_to_builder_index" fork="gloas" hash="2fea5b47">
//	def convert_validator_index_to_builder_index(validator_index: ValidatorIndex) -> BuilderIndex:
//	    return BuilderIndex(validator_index & ~BUILDER_INDEX_FLAG)
//	</spec>
func (v ValidatorIndex) ToBuilderIndex() BuilderIndex {
	return BuilderIndex(uint64(v) & ^BuilderIndexFlag)
}

// Div divides validator index by x.
// This method panics if dividing by zero!
func (v ValidatorIndex) Div(x uint64) ValidatorIndex {
	if x == 0 {
		panic("divbyzero") // lint:nopanic -- Panic is communicated in the godoc commentary.
	}
	return ValidatorIndex(uint64(v) / x)
}

// Add increases validator index by x.
func (v ValidatorIndex) Add(x uint64) ValidatorIndex {
	return ValidatorIndex(uint64(v) + x)
}

// Sub subtracts x from the validator index.
// This method panics if causing an underflow!
func (v ValidatorIndex) Sub(x uint64) ValidatorIndex {
	if uint64(v) < x {
		panic("underflow") // lint:nopanic -- Panic is communicated in the godoc commentary.
	}
	return ValidatorIndex(uint64(v) - x)
}

// Mod returns result of `validator index % x`.
func (v ValidatorIndex) Mod(x uint64) ValidatorIndex {
	return ValidatorIndex(uint64(v) % x)
}

// HashTreeRoot --
func (v ValidatorIndex) HashTreeRoot() ([32]byte, error) {
	return fssz.HashWithDefaultHasher(v)
}

// HashTreeRootWith --
func (v ValidatorIndex) HashTreeRootWith(hh *fssz.Hasher) error {
	hh.PutUint64(uint64(v))
	return nil
}

// UnmarshalSSZ --
func (v *ValidatorIndex) UnmarshalSSZ(buf []byte) error {
	if len(buf) != v.SizeSSZ() {
		return fmt.Errorf("expected buffer of length %d received %d", v.SizeSSZ(), len(buf))
	}
	*v = ValidatorIndex(fssz.UnmarshallUint64(buf))
	return nil
}

// MarshalSSZTo --
func (v *ValidatorIndex) MarshalSSZTo(dst []byte) ([]byte, error) {
	marshalled, err := v.MarshalSSZ()
	if err != nil {
		return nil, err
	}
	return append(dst, marshalled...), nil
}

// MarshalSSZ --
func (v *ValidatorIndex) MarshalSSZ() ([]byte, error) {
	marshalled := fssz.MarshalUint64([]byte{}, uint64(*v))
	return marshalled, nil
}

// SizeSSZ --
func (v *ValidatorIndex) SizeSSZ() int {
	return 8
}
