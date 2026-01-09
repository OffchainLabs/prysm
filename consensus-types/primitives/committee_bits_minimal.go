//go:build minimal

package primitives

import "github.com/OffchainLabs/go-bitfield"

func NewAttestationCommitteeBits() bitfield.Bitvector4 {
	return bitfield.NewBitvector4()
}
