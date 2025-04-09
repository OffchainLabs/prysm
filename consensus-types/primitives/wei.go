package primitives

import (
	"fmt"
	"math/big"
	"slices"

	fssz "github.com/prysmaticlabs/fastssz"
)

// ZW returns a non-nil zero value for primitives.Wei
func ZeroWei() Wei {
	return big.NewInt(0)
}

// Wei is the smallest unit of Ether, represented as a pointer to a bigInt.
type Wei *big.Int

// Gwei is a denomination of 1e9 Wei represented as an uint64.
type Gwei uint64

var _ fssz.HashRoot = (Gwei)(0)
var _ fssz.Marshaler = (*Gwei)(nil)
var _ fssz.Unmarshaler = (*Gwei)(nil)

// HashTreeRoot --
func (g Gwei) HashTreeRoot() ([32]byte, error) {
	return fssz.HashWithDefaultHasher(g)
}

// HashTreeRootWith --
func (g Gwei) HashTreeRootWith(hh *fssz.Hasher) error {
	hh.PutUint64(uint64(g))
	return nil
}

// UnmarshalSSZ --
func (g *Gwei) UnmarshalSSZ(buf []byte) error {
	if len(buf) != g.SizeSSZ() {
		return fmt.Errorf("expected buffer of length %d received %d", g.SizeSSZ(), len(buf))
	}
	*g = Gwei(fssz.UnmarshallUint64(buf))
	return nil
}

// MarshalSSZTo --
func (g *Gwei) MarshalSSZTo(dst []byte) ([]byte, error) {
	marshalled, err := g.MarshalSSZ()
	if err != nil {
		return nil, err
	}
	return append(dst, marshalled...), nil
}

// MarshalSSZ --
func (g *Gwei) MarshalSSZ() ([]byte, error) {
	marshalled := fssz.MarshalUint64([]byte{}, uint64(*g))
	return marshalled, nil
}

// SizeSSZ --
func (g *Gwei) SizeSSZ() int {
	return 8
}

// WeiToBigInt is a convenience method to cast a wei back to a big int
func WeiToBigInt(w Wei) *big.Int {
	return w
}

// Uint64ToWei creates a new Wei (aka big.Int) representing the given uint64 value.
func Uint64ToWei(v uint64) Wei {
	return big.NewInt(0).SetUint64(v)
}

// LittleEndianBytesToWei returns a Wei value given a little-endian binary representation.
// The only places we use this representation are in protobuf types that hold either the
// local execution payload bid or the builder bid. Going forward we should avoid that representation
// so this function being used in new places should be considered a code smell.
func LittleEndianBytesToWei(value []byte) Wei {
	if len(value) == 0 {
		return big.NewInt(0)
	}
	v := make([]byte, len(value))
	copy(v, value)
	// SetBytes expects a big-endian representation of the value, so we reverse the byte slice.
	slices.Reverse(v)
	return big.NewInt(0).SetBytes(v)
}

// WeiToGwei converts big int wei to uint64 gwei.
// The input `v` is copied before being modified.
func WeiToGwei(v Wei) Gwei {
	if v == nil {
		return 0
	}
	gweiPerEth := big.NewInt(1e9)
	copied := big.NewInt(0).Set(v)
	copied.Div(copied, gweiPerEth)
	return Gwei(copied.Uint64())
}
