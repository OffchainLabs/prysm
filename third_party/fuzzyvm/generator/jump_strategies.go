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

import "math/big"

var jumpStrategies = []Strategy{
	new(jumpdestGenerator),
	new(jumpGenerator),
}

type jumpdestGenerator struct{}

func (*jumpdestGenerator) Execute(env Environment) {
	switch env.f.Byte() % 10 {
	case 0:
		// Set a jumpdest label
		env.jumptable.Push(env.p.Label(), env.p.Label())
	case 1:
		// Set the jumpdest randomly
		env.jumptable.Push(uint64(env.f.Uint16()), env.p.Label())
	default:
		// Set a jumpdest
		env.jumptable.Push(env.p.Label(), env.p.Label())
	}
}

func (*jumpdestGenerator) Importance() int {
	return 5
}

func (*jumpdestGenerator) String() string {
	return "jumpdestGenerator"
}

type jumpGenerator struct{}

func (*jumpGenerator) Execute(env Environment) {
	if env.f.Bool() {
		// Jump to a label
		jumpdest := env.jumptable.Pop(env.p.Label())
		env.p.Jump(jumpdest)
	} else {
		// Jumpi to a label
		var (
			jumpdest   = env.jumptable.Pop(env.p.Label())
			shouldJump = env.f.Bool()
			condition  = big.NewInt(0)
		)
		if shouldJump {
			condition = env.f.BigInt32()
		}
		// jumps if condition != 0
		env.p.JumpIf(jumpdest, condition)
	}
}

func (*jumpGenerator) Importance() int {
	return 7
}

func (*jumpGenerator) String() string {
	return "jumpGenerator"
}
