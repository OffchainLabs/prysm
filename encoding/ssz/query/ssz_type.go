package query

import "fmt"

// SSZType represents the type supported by SSZ.
// https://github.com/ethereum/consensus-specs/blob/master/ssz/simple-serialize.md#typing
type SSZType int

// SSZ type constants.
const (
	// Basic types
	Uint8 SSZType = iota
	Uint16
	Uint32
	Uint64
	Boolean

	// Composite types
	Container
	Vector
	List
	Bitvector
	Bitlist

	// Added in EIP-7916
	ProgressiveList
	Union
)

func (t SSZType) String() string {
	switch t {
	case Uint8:
		return "Uint8"
	case Uint16:
		return "Uint16"
	case Uint32:
		return "Uint32"
	case Uint64:
		return "Uint64"
	case Boolean:
		return "Boolean"
	case Container:
		return "Container"
	case Vector:
		return "Vector"
	case List:
		return "List"
	case Bitvector:
		return "Bitvector"
	case Bitlist:
		return "Bitlist"
	case ProgressiveList:
		return "ProgressiveList"
	case Union:
		return "Union"
	default:
		return fmt.Sprintf("Unknown(%d)", t)
	}
}
