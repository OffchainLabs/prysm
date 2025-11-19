// Copyright 2021 Marius van der Wijden
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

package generator

import (
	"math"

	"github.com/MariusVanDerWijden/FuzzyVM/filler"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/core/vm/program"
)

type Environment struct {
	f         *filler.Filler
	p         *program.Program
	jumptable *Jumptable
}

type Strategy interface {
	// Execute executes the strategy.
	// adds the resulting opcodes to the program.
	Execute(env Environment)
	// Importance returns the importance of this strategy.
	// This is needed to calculate the probability of this strategy.
	// Should be on a scale of 1-100.
	Importance() int

	String() string
}

// Probability returns the probability of this strategy,
// given the sum of all strategies on scale 1-255.
func Probability(strat Strategy) byte {
	imp := strat.Importance()
	return max(byte(math.Round((float64(imp)/float64(100))*float64(255))), 1)
}

func makeMap(strats []Strategy) map[byte]Strategy {
	m := make(map[byte]Strategy)
	sum := byte(0)
	for _, strat := range strats {
		for i := byte(0); i < Probability(strat); i++ {
			m[sum] = strat
			sum++
		}
	}
	for i := sum - 1; i < 255; i++ {
		m[i+1] = new(validOpcodeGenerator)
	}
	return m
}

func (env Environment) CreateAndCall(code []byte, isCreate2 bool, callOp vm.OpCode) {
	var (
		value    = 0
		offset   = 0
		size     = len(code)
		salt     = 0
		createOp = vm.CREATE
	)
	// Load the code into mem
	env.p.Mstore(code, 0)
	// Create it
	if isCreate2 {
		env.p.Push(salt)
		createOp = vm.CREATE2
	}
	env.p.Push(size).Push(offset).Push(value).Op(createOp)
	// If there happen to be a zero on the stack, it doesn't matter, we're
	// not sending any value anyway
	env.p.Push(0).Push(0) // mem out
	env.p.Push(0).Push(0) // mem in
	addrOffset := vm.OpCode(vm.DUP5)
	if callOp == vm.CALL || callOp == vm.CALLCODE {
		env.p.Push(0) // value
		addrOffset = vm.DUP6
	}
	env.p.Op(addrOffset) // address (from create-op above)
	env.p.Op(vm.GAS, callOp)
}
