//go:build minimal || e2e

package primitives

import "github.com/prysmaticlabs/go-bitfield"

func NewAttestationCommitteeBits() bitfield.Bitvector4 {
	return bitfield.NewBitvector4()
}
