//go:build minimal

package eth

import "github.com/OffchainLabs/go-bitfield"

func NewCommitteeBitsPQ() bitfield.Bitvector4 {
	return bitfield.NewBitvector4()
}
