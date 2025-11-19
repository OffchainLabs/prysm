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
	"github.com/MariusVanDerWijden/FuzzyVM/filler"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm/program"
	"github.com/holiman/uint256"
)

var sha256Addr = common.HexToAddress("0x2")

type sha256Caller struct{}

func (*sha256Caller) call(p *program.Program, f *filler.Filler) error {
	data := f.ByteSlice256()
	c := CallObj{
		Gas:       uint256.MustFromBig(f.GasInt()),
		Address:   sha256Addr,
		InOffset:  0,
		InSize:    uint32(len(data)),
		OutOffset: 0,
		OutSize:   32,
		Value:     f.BigInt32(),
	}
	p.Mstore(data, 0)
	CallRandomizer(p, f, c)
	return nil
}
