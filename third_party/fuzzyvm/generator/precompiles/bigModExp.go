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
	"encoding/binary"

	"github.com/MariusVanDerWijden/FuzzyVM/filler"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm/program"
	"github.com/holiman/uint256"
)

var bigModExpAddr = common.HexToAddress("0x5")

type bigModExpCaller struct{}

func (*bigModExpCaller) call(p *program.Program, f *filler.Filler) error {
	base := f.ByteSlice256()
	exp := f.ByteSlice256()
	mod := f.ByteSlice256()
	lBase := common.LeftPadBytes(int32ToByte(len(base)), 32)
	lExp := common.LeftPadBytes(int32ToByte(len(exp)), 32)
	lMod := common.LeftPadBytes(int32ToByte(len(mod)), 32)
	var data []byte
	data = append(data, lBase...)
	data = append(data, lExp...)
	data = append(data, lMod...)
	data = append(data, base...)
	data = append(data, exp...)
	data = append(data, mod...)
	p.Mstore(data, 0)
	c := CallObj{
		Gas:       uint256.MustFromBig(f.GasInt()),
		Address:   bigModExpAddr,
		InOffset:  0,
		InSize:    uint32(len(data)),
		OutOffset: 0,
		OutSize:   64,
		Value:     f.BigInt32(),
	}
	CallRandomizer(p, f, c)
	return nil
}

func int32ToByte(a int) []byte {
	res := make([]byte, 4)
	binary.BigEndian.PutUint32(res, uint32(a))
	return res
}
