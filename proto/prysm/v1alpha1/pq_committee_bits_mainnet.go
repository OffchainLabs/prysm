//go:build !minimal

package eth

import "github.com/OffchainLabs/go-bitfield"

func NewCommitteeBitsPQ() bitfield.Bitvector16 {
	return bitfield.NewBitvector16()
}
