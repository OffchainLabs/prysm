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

var blake2fAddr = common.HexToAddress("0x9")

type blake2fCaller struct{}

func (*blake2fCaller) call(p *program.Program, f *filler.Filler) error {
	var (
		input  = make([]byte, 213)
		offset = 0
	)
	// Rounds
	binary.BigEndian.PutUint32(input, f.Uint32())
	offset += 4
	// h
	for i := 0; i < 8; i++ {
		binary.BigEndian.PutUint64(input[offset:], f.Uint64())
		offset += 8
	}
	// m
	for i := 0; i < 16; i++ {
		binary.BigEndian.PutUint64(input[offset:], f.Uint64())
		offset += 8
	}
	// t
	for i := 0; i < 2; i++ {
		binary.BigEndian.PutUint64(input[offset:], f.Uint64())
		offset += 8
	}
	// Valid or invalid inputs
	if f.Bool() {
		input[212] = f.Byte()
	} else {
		if f.Bool() {
			input[212] = 0
		} else {
			input[212] = 1
		}
	}

	c := CallObj{
		Gas:       uint256.MustFromBig(f.GasInt()),
		Address:   blake2fAddr,
		InOffset:  0,
		InSize:    213,
		OutOffset: 0,
		OutSize:   64,
		Value:     f.BigInt32(),
	}
	p.Mstore(input, 0)
	CallRandomizer(p, f, c)
	return nil
}
