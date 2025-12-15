//go:build !minimal

package primitives

import "github.com/OffchainLabs/go-bitfield"

func NewAttestationCommitteeBits() bitfield.Bitvector64 {
	return bitfield.NewBitvector64()
}
