// Copyright 2020 Marius van der Wijden
// This file is part of the fuzzy-vm library.
//
// The fuzzy-vm library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The fuzzy-vm library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the fuzzy-vm library. If not, see <http://www.gnu.org/licenses/>.

package precompiles

import (
	"math/big"

	"github.com/MariusVanDerWijden/FuzzyVM/filler"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm/program"
	"github.com/holiman/uint256"
)

var (
	precompiles = []precompile{
		new(ecdsaCaller),
		new(sha256Caller),
		new(ripemdCaller),
		new(identityCaller),
		new(bigModExpCaller),
		new(bn256Caller),
		new(bn256MulCaller),
		new(bn256PairingCaller),
		new(blake2fCaller),
	}
)

type precompile interface {
	call(p *program.Program, f *filler.Filler) error
}

// CallObj encompasses everything needed to make a call.
type CallObj struct {
	Gas       *uint256.Int
	Address   common.Address
	Value     *big.Int
	InOffset  uint32
	InSize    uint32
	OutOffset uint32
	OutSize   uint32
}

// CallRandomizer calls an address either with the CALL, CALLCODE or STATICCALL opcode.
func CallRandomizer(p *program.Program, f *filler.Filler, c CallObj) {
	// modify call object
	switch f.Byte() % 25 {
	case 0:
		c.InOffset = uint32(f.MemInt().Uint64())
	case 1:
		c.InSize = uint32(f.MemInt().Uint64())
	case 2:
		c.OutOffset = uint32(f.MemInt().Uint64())
	case 3:
		c.OutSize = uint32(f.MemInt().Uint64())
	}

	switch f.Byte() % 3 {
	case 0:
		p.Call(c.Gas, c.Address, c.Value, c.InOffset, c.InSize, c.OutOffset, c.OutSize)
	case 1:
		p.CallCode(c.Gas, c.Address, c.Value, c.InOffset, c.InSize, c.OutOffset, c.OutSize)
	case 2:
		p.StaticCall(c.Gas, c.Address, c.InOffset, c.InSize, c.OutOffset, c.OutSize)
	}
}

// CallPrecompile randomly calls one of the available precompiles.
func CallPrecompile(p *program.Program, f *filler.Filler) {
	// call a precompile
	var (
		idx  = int(f.Byte()) % len(precompiles)
		prec = precompiles[idx]
	)
	if err := prec.call(p, f); err != nil {
		panic(err)
	}
}
