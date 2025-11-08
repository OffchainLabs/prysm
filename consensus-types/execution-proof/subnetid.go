package executionproof

import (
	"encoding/json"
	"fmt"
	"strconv"
	ssz "github.com/prysmaticlabs/fastssz"
)

// EXECUTION_PROOF_TYPE_COUNT is the number of execution proof subnets.
// Each subnet represents a different zkVM+EL combination.
const EXECUTION_PROOF_TYPE_COUNT uint8 = 8

// ExecutionProofId identifies which zkVM/proof system subnet a proof belongs to.
// Note: There is a 1-1 mapping between subnet ID and a unique proof.
type ExecutionProofId uint8


// NewExecutionProofId creates a new ExecutionProofId if the value is valid.
func NewExecutionProofId(id uint8) (ExecutionProofId, error) {
	if id < EXECUTION_PROOF_TYPE_COUNT {
		return ExecutionProofId(id), nil
	}
	return 0, fmt.Errorf(
		"Invalid ExecutionProofId: %d, must be < %d",
		id, EXECUTION_PROOF_TYPE_COUNT,
	)
}

// AsU8 returns the inner u8 value.
func (e ExecutionProofId) AsU8() uint8 {
	return uint8(e)
}

// AsUsize returns the subnet ID as a usize (int in Go).
func (e ExecutionProofId) AsUsize() int {
	return int(e)
}

// String implements the fmt.Stringer interface.
func (e ExecutionProofId) String() string {
	return strconv.FormatUint(uint64(e), 10)
}

// All returns all valid subnet IDs.
func All() []ExecutionProofId {
	subnets := make([]ExecutionProofId, EXECUTION_PROOF_TYPE_COUNT)
	for i := range EXECUTION_PROOF_TYPE_COUNT {
		// We can safely ignore the error here as we are iterating within bounds.
		subnets[i], _ = NewExecutionProofId(i)
	}
	return subnets
}

func (e *ExecutionProofId) MarshalSSZ() ([]byte, error) {
	return ssz.MarshalUint8(make([]byte, 0), uint8(*e)), nil
}

// UnmarshalSSZ implements the ssz.Unmarshaler interface.
func (e *ExecutionProofId) UnmarshalSSZ(buf []byte) error {
	val := ssz.UnmarshallUint8(buf)

	// Validate the value after unmarshaling
	if val >= EXECUTION_PROOF_TYPE_COUNT {
		return fmt.Errorf(
			"Invalid ExecutionProofId: %d, must be < %d",
			val, EXECUTION_PROOF_TYPE_COUNT,
		)
	}
	*e = ExecutionProofId(val)
	return nil
}

// IsSSZFixedLen returns true as uint8 is fixed length.
func (e *ExecutionProofId) IsSSZFixedLen() bool {
	return true
}

// SszFixedLen returns the fixed length of the underlying type.
func (e *ExecutionProofId) SszFixedLen() int {
	return 1 // uint8
}

// SszSize returns the size of the value.
func (e *ExecutionProofId) SszSize() int {
	return 1
}

// HashTreeRoot ssz hashes the BuilderBidCapella object
func (e *ExecutionProofId) HashTreeRoot() ([32]byte, error) {
	return ssz.HashWithDefaultHasher(e)
}

func (s *ExecutionProofId) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	return s.HashTreeRootWith(hh)
}

// MarshalJSON implements the json.Marshaler interface.
func (e ExecutionProofId) MarshalJSON() ([]byte, error) {
	return json.Marshal(uint8(e))
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (e *ExecutionProofId) UnmarshalJSON(data []byte) error {
	var id uint8
	if err := json.Unmarshal(data, &id); err != nil {
		return err
	}
	
	// Validate after unmarshaling
	newId, err := NewExecutionProofId(id)
	if err != nil {
		return err
	}
	*e = newId
	return nil
}