package primitives

import (
	"fmt"

	fssz "github.com/prysmaticlabs/fastssz"
)

var _ fssz.HashRoot = (ExecutionProofId)(0)
var _ fssz.Marshaler = (*ExecutionProofId)(nil)
var _ fssz.Unmarshaler = (*ExecutionProofId)(nil)

// Number of execution proofs
// Each proof represents a different zkVM+EL combination
//
// TODO(zkproofs): The number 8 is a parameter that we will want to configure in the future
const EXECUTION_PROOF_TYPE_COUNT = 8

// ExecutionProofId identifies which zkVM/proof system a proof belongs to.
type ExecutionProofId uint8

func (id *ExecutionProofId) IsValid() bool {
	return uint8(*id) < EXECUTION_PROOF_TYPE_COUNT
}

// HashTreeRoot --
func (id ExecutionProofId) HashTreeRoot() ([32]byte, error) {
	return fssz.HashWithDefaultHasher(id)
}

// HashTreeRootWith --
func (id ExecutionProofId) HashTreeRootWith(hh *fssz.Hasher) error {
	hh.PutUint8(uint8(id))
	return nil
}

// UnmarshalSSZ --
func (id *ExecutionProofId) UnmarshalSSZ(buf []byte) error {
	if len(buf) != id.SizeSSZ() {
		return fmt.Errorf("expected buffer of length %d received %d", id.SizeSSZ(), len(buf))
	}
	*id = ExecutionProofId(fssz.UnmarshallUint8(buf))
	return nil
}

// MarshalSSZTo --
func (id *ExecutionProofId) MarshalSSZTo(buf []byte) ([]byte, error) {
	marshalled, err := id.MarshalSSZ()
	if err != nil {
		return nil, err
	}
	return append(buf, marshalled...), nil
}

// MarshalSSZ --
func (id *ExecutionProofId) MarshalSSZ() ([]byte, error) {
	marshalled := fssz.MarshalUint8([]byte{}, uint8(*id))
	return marshalled, nil
}

// SizeSSZ --
func (id *ExecutionProofId) SizeSSZ() int {
	return 1
}
